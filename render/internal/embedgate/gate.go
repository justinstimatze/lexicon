// Package embedgate is lexicon's cheap-first semantic gate. It embeds the user
// prompt with a local ollama model and returns the top-K atoms by cosine
// similarity to per-atom PROTOTYPE vectors. Prototypes are computed once
// (cache-on-disk; invalidated by model+elements hash) and held in memory for
// the process lifetime. Tuned RECALL-FIRST: a false trip costs one Haiku-lens
// call which then rejects, so the gate errs toward letting smoke through.
//
// Pattern lifted from sibling project cupel (github.com/justinstimatze/cupel,
// cmd/cupel/gate.go). cupel uses one prototype per ~10 engines; lexicon
// uses one prototype per ~250+ atoms. Same shape, different N.
package embedgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// DefaultThreshold is the cosine cutoff below which the gate emits silence
// (no candidates pass to the lens). 0.58 confirmed by V56 (e) held-out probe
// re-calibration after extending the corpus with V53.2 atoms (lex-ymckb/0294/
// 0295). See render/internal/embedgate/probe.go:
//
//	POS (32 prompts × 17 gold atoms): top-1 sim 0.581–0.810, p50=0.674
//	NEG (7 ordinary prompts):         top-1 sim 0.501–0.608, p50=0.547
//	POS min (0.581) brackets the threshold from above; NEG max (0.608) brackets
//	from below — they OVERLAP, so the gate cannot be both 100%-POS-recall and
//	100%-NEG-silence on this corpus. Corpus extension maintained the same
//	overlap region, validating the V54.1 threshold against a wider sample.
//
// V208: below-threshold no longer hard-silences (cmd_hook.go falls through to
// the full-pool lens instead, matching every other fail-soft path in the
// hook). A "false silence" now just means the fallback path runs instead of
// the cheap narrowed path — never a lost atom. That changes the calibration
// tradeoff: the old comment here reasoned T=0.61 was "wrong" because it lost
// 4 POS outright; with the safety net in place, T=0.61 just routes those 4
// POS through the (cache-eligible, still cheap at real call volume) full-pool
// lens instead, while fully silencing the NEG corpus at the narrow-path
// entry. Re-run of V208 c against the current 1024-atom elements (up from
// ~250-300 at the original V56 calibration) found the POS/NEG overlap region
// essentially unchanged (POS floor 0.581, NEG ceiling 0.608) despite the ~4x
// elements growth — narrowing the gate's raw recall window is a separate,
// still-open question (calib's own `gold in top-20: 29/32` finding), not
// something this threshold controls.
//
// To re-calibrate against a richer corpus: set LEXICON_CALIB_CORPUS=/path.json
// (see probe.go for shape) and re-run `lexicon calib -v`.
const DefaultThreshold = 0.61

// DefaultTopK is the number of candidate atoms passed to the lens on a trip.
// Cupel passes 1 (its lens classifies among ~10 engines). Lexicon classifies
// among the whole elements — three orders of magnitude more — and the lens
// needs to discriminate; 20 gives it enough context to rank without inflating
// prompt size.
const DefaultTopK = 20

// Result is one ranked atom from the embedding gate.
type Result struct {
	AtomID string
	Score  float64 // cosine similarity in [-1, 1]; for normalized vectors typically [0, 1]
}

// Threshold reads LEXICON_GATE_THRESHOLD or returns DefaultThreshold.
func Threshold() float64 {
	if v := os.Getenv("LEXICON_GATE_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return DefaultThreshold
}

// TopK reads LEXICON_GATE_TOP_K or returns DefaultTopK.
func TopK() int {
	if v := os.Getenv("LEXICON_GATE_TOP_K"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return DefaultTopK
}

// maxPrototypeChars caps prototype length. Some lexicon canonical-instances run
// 1000+ chars (heavy cross-domain entries like lex-s2pjj). nomic-embed-text has
// an 8k-token context but very long inputs slow batch embedding and bloat the
// on-disk cache without proportional semantic gain. ~1200 chars covers
// name + most canonical-instance + all evokes for the vast majority of atoms.
const maxPrototypeChars = 1200

// PrototypeText is the single-instance embedding input for an atom: name +
// first canonical-instance + evokes (per V53 user-confirmed shape), truncated
// to maxPrototypeChars on a word boundary. Kept for callers that only need
// one representative text; the embedding path itself uses PrototypeTexts,
// which scores against ALL of an atom's instances rather than just the
// first — an atom whose first instance happens to be its most archaic or
// niche example (e.g. lex-33w6z leading with a Hávamál myth ahead of its own
// far more transferable "negotiation concessions" / "microservices vs
// monolith" instances) shouldn't have its overall matchability capped by
// that ordering. Root-caused via lexicon calib -v (2026-08-25): of 7 POS
// probe misses, only 2 were close-to-cutoff; the rest ranged to rank 1022,
// and at least one (lex-33w6z) missed on BOTH its calibration prompts while
// carrying multiple well-anchored later instances.
func PrototypeText(e *types.LexEntry) string {
	if len(e.CanonicalInstances) == 0 {
		return buildPrototypeText(e, "")
	}
	return buildPrototypeText(e, e.CanonicalInstances[0])
}

// PrototypeTexts returns one embeddable text per canonical-instance (name +
// that instance + evokes, each independently truncated). Atoms with zero
// canonical-instances (shouldn't happen per schema — required, ≥1 — but
// handled) get a single name(+evokes)-only fallback text so they still get a
// usable vector rather than silently dropping out of the gate.
func PrototypeTexts(e *types.LexEntry) []string {
	if len(e.CanonicalInstances) == 0 {
		return []string{buildPrototypeText(e, "")}
	}
	out := make([]string, len(e.CanonicalInstances))
	for i, inst := range e.CanonicalInstances {
		out[i] = buildPrototypeText(e, inst)
	}
	return out
}

func buildPrototypeText(e *types.LexEntry, instance string) string {
	var b strings.Builder
	b.WriteString(e.Name)
	if instance != "" {
		b.WriteByte('\n')
		b.WriteString(instance)
	}
	if len(e.Evokes) > 0 {
		b.WriteByte('\n')
		b.WriteString(strings.Join(e.Evokes, ", "))
	}
	s := b.String()
	if len(s) <= maxPrototypeChars {
		return s
	}
	// Truncate on the last whitespace before the cap so we don't split a word.
	cut := maxPrototypeChars
	for i := maxPrototypeChars - 1; i > maxPrototypeChars-100 && i > 0; i-- {
		if s[i] == ' ' || s[i] == '\n' {
			cut = i
			break
		}
	}
	return s[:cut]
}

// protoEntry is one atom's cached prototype set: the content-hash it was
// embedded from (so a per-atom edit invalidates only that atom, not the
// whole cache) plus one L2-normalized vector per canonical-instance
// (PrototypeTexts). Score takes the atom's BEST (max-cosine) vector against
// a query — see BestCosine — rather than a single vector, so the atom's
// overall matchability isn't capped by whichever instance happens to be
// listed first.
type protoEntry struct {
	Hash    string      `json:"hash"`
	Vectors [][]float64 `json:"vectors"`
}

// protoTextsHash combines an atom's PrototypeTexts into one content
// fingerprint. Any instance changing (added, edited, removed) invalidates
// the whole atom's cached vector set and it's fully re-embedded — coarser
// than per-instance invalidation, but instances rarely change independently
// of each other within one editing pass, and this keeps the cache format and
// Score's scoring loop simple.
func protoTextsHash(texts []string) string {
	return protoHash(strings.Join(texts, "\x00"))
}

// protoCache is the on-disk JSON shape. Entries are keyed by atom ID, each
// carrying its own content hash. A per-atom key (rather than a whole-elements
// hash) means adding or editing N atoms in a 1000+-atom elements invalidates
// only those N entries. The whole-elements-hash design this replaced meant
// every mining-pass commit invalidated the ENTIRE cache: LoadOrBuildPrototypes
// would then need several minutes to re-embed everything, and a mining session
// commits new atoms faster than that rebuild completes -- so the hook was
// chronically stuck on the slow full-pool-lens fallback (diagnosed via
// dispatch from a sibling session, msg-0d5dd49e, 2026-07-14). Only a Model
// mismatch invalidates wholesale, since embeddings from different models
// aren't comparable.
type protoCache struct {
	Model   string                `json:"model"`
	Entries map[string]protoEntry `json:"entries"`
}

// CachePath returns the on-disk cache location. ~/.claude/lexicon/prototypes.json
// by default; LEXICON_CACHE_DIR overrides the parent.
func CachePath() string {
	dir := os.Getenv("LEXICON_CACHE_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".claude", "lexicon")
	}
	_ = os.MkdirAll(dir, 0o700)
	return filepath.Join(dir, "prototypes.json")
}

// protoHash returns the content fingerprint for one atom's prototype text. A
// cached entry stays valid only while this hash matches what it was embedded
// from.
func protoHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

// readCache loads the on-disk cache, or a zero-value cache if the file is
// absent, corrupt, or was built under a different embed model.
func readCache() protoCache {
	empty := protoCache{Model: EmbedModel(), Entries: map[string]protoEntry{}}
	b, err := os.ReadFile(CachePath())
	if err != nil {
		return empty
	}
	var pc protoCache
	if json.Unmarshal(b, &pc) != nil || pc.Model != EmbedModel() {
		return empty
	}
	if pc.Entries == nil {
		pc.Entries = map[string]protoEntry{}
	}
	return pc
}

// LoadOrBuildPrototypes returns the per-atom prototype vectors. Atoms whose
// cached entry already matches their current content hash cost zero ollama
// round trips; only new or edited atoms are embedded.
//
// This is the OFF-PATH builder, invoked by `lexicon build-prototypes` (and
// calib/replay) — never by the hook. The hook reads the cache via Score, which
// is warm-only and never builds (building 700+ prototypes is a minutes-long
// ollama job that must not touch the blocking UserPromptSubmit path). The
// per-batch persist makes that minutes-long build RESUMABLE: each completed
// ollama batch is written immediately, so an interrupted build (Ctrl-C, a
// dropped ollama connection, or a context deadline) keeps the work already done
// and the next invocation fills in only what's missing, rather than restarting
// from zero. Callers that pass a deadline-free context build everything in one
// pass; the incremental persist is purely insurance against interruption.
//
// Best-effort contract: the returned map is whatever has been embedded so far
// (possibly a subset if the build was cut short). An error is returned only
// when nothing at all is available (empty cache + a build that couldn't run),
// which `build-prototypes` surfaces with an ollama-down hint.
func LoadOrBuildPrototypes(ctx context.Context, atoms []*types.LexEntry) (map[string][][]float64, error) {
	pc := readCache()

	// Seed from whatever entries already match their atom's current content
	// hash; a stale or absent entry just means that one atom needs embedding,
	// not that the whole cache is discarded.
	cached := make(map[string][][]float64, len(atoms))
	type missingAtom struct {
		id    string
		texts []string
	}
	missing := make([]missingAtom, 0)
	for _, a := range atoms {
		texts := PrototypeTexts(a)
		if len(texts) == 0 {
			continue
		}
		h := protoTextsHash(texts)
		if e, ok := pc.Entries[a.ID]; ok && e.Hash == h {
			cached[a.ID] = e.Vectors
			continue
		}
		missing = append(missing, missingAtom{id: a.ID, texts: texts})
	}
	if len(missing) == 0 {
		return cached, nil // warm + complete
	}

	// Flatten to a single (id, text) stream so batching is purely a function
	// of total text count, not atom count -- an atom with many instances
	// doesn't get special-cased, it just contributes that many items to the
	// same batch stream everything else uses.
	flatIDs := make([]string, 0)
	flatTexts := make([]string, 0)
	for _, m := range missing {
		for _, t := range m.texts {
			flatIDs = append(flatIDs, m.id)
			flatTexts = append(flatTexts, t)
		}
	}

	// Embed the missing prototypes one ollama batch at a time, persisting after
	// each so a deadline (or an ollama dropout) mid-build keeps the work already
	// done. Chunking at batchSize matches EmbedTexts's own internal batch, so
	// each EmbedTexts call here is a single round trip and a natural save point.
	// An atom's vectors only land in the persisted cache once ALL of its texts
	// have arrived (checked below) — a deadline that cuts off mid-atom must not
	// persist a partial, hash-matching-but-incomplete entry that Score would
	// silently under-score against.
	perAtomVecs := make(map[string][][]float64, len(missing))
	var buildErr error
	for i := 0; i < len(flatTexts); i += batchSize {
		end := i + batchSize
		if end > len(flatTexts) {
			end = len(flatTexts)
		}
		vecs, err := EmbedTexts(ctx, flatTexts[i:end])
		if err != nil {
			buildErr = err
			break
		}
		for j, v := range vecs {
			id := flatIDs[i+j]
			perAtomVecs[id] = append(perAtomVecs[id], v)
		}
		for _, m := range missing {
			if _, done := cached[m.id]; done {
				continue
			}
			if len(perAtomVecs[m.id]) == len(m.texts) {
				h := protoTextsHash(m.texts)
				pc.Entries[m.id] = protoEntry{Hash: h, Vectors: perAtomVecs[m.id]}
				cached[m.id] = perAtomVecs[m.id]
			}
		}
		persistCache(pc)
	}

	if len(cached) == 0 {
		if buildErr != nil {
			return nil, fmt.Errorf("embedgate: build prototypes: %w", buildErr)
		}
		return nil, nil
	}
	return cached, nil
}

// persistCache writes the prototype cache atomically (temp file + rename) so
// a deadline-cut or a concurrent hook process can never observe a torn JSON
// file. Best-effort: any IO error is swallowed — the cache is a latency
// optimization, not a source of truth, and the in-memory vectors are already
// usable for this call.
func persistCache(pc protoCache) {
	b, err := json.Marshal(pc)
	if err != nil {
		return
	}
	dir := filepath.Dir(CachePath())
	tmp, err := os.CreateTemp(dir, "prototypes-*.json.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, CachePath()); err != nil {
		_ = os.Remove(tmpName)
	}
}

// ErrColdCache means NO atom in the pool has a valid cached prototype (empty
// cache, wrong model, or every entry stale) — so the gate has nothing at all
// to score against. It is NOT an error condition in the usual sense: the
// caller (the hook) treats it as "skip the gate, hand the full pool to the
// lens." A cache that's merely PARTIAL (most atoms valid, a handful new or
// edited) does not trigger this — Score narrows to the valid subset instead.
// The cache is (re)built OFF the blocking path with `lexicon build-prototypes`;
// see Score.
var ErrColdCache = errors.New("embedgate: prototype cache cold (run: lexicon build-prototypes)")

// loadCachedPrototypes reads the on-disk prototype cache WITHOUT embedding
// anything, and returns only the entries whose stored hash still matches each
// atom's current content. Atoms that are new, edited, or never embedded are
// simply absent from the returned map rather than invalidating the rest —
// Score narrows to whatever subset IS valid. Returns nil only when NOTHING is
// valid (empty cache, wrong model, or every entry stale), which is the
// wholesale-cold signal callers still need.
func loadCachedPrototypes(atoms []*types.LexEntry) map[string][][]float64 {
	pc := readCache()
	if len(pc.Entries) == 0 {
		return nil
	}
	out := make(map[string][][]float64, len(atoms))
	for _, a := range atoms {
		e, ok := pc.Entries[a.ID]
		if !ok || e.Hash != protoTextsHash(PrototypeTexts(a)) {
			continue
		}
		out[a.ID] = e.Vectors
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Score embeds the prompt and returns the top-K atoms by cosine similarity to
// their prototypes.
//
// WARM-ONLY by design. Score reads the prototype cache but NEVER builds it.
// Building is a multi-batch, minutes-long ollama job (one embed per atom; ~5min
// for 700+ atoms on a single-runner box), and it must never run on the blocking
// UserPromptSubmit hook path — doing so is what produced the pathological
// multi-minute hook stalls (the harness kills the hook at 30s and discards its
// output). On a cold cache Score returns ErrColdCache and the caller falls back
// to the full-pool lens; the cache is (re)built off-path by
// `lexicon build-prototypes`. The only embedding Score does is the single user
// prompt — a fast one-shot that honors ctx's deadline.
func Score(ctx context.Context, prompt string, atoms []*types.LexEntry, k int) ([]Result, error) {
	if k <= 0 {
		k = DefaultTopK
	}
	protos := loadCachedPrototypes(atoms)
	if len(protos) == 0 {
		return nil, ErrColdCache
	}
	pv, err := EmbedTexts(ctx, []string{prompt})
	if err != nil {
		return nil, fmt.Errorf("embedgate: embed prompt: %w", err)
	}
	scores := make([]Result, 0, len(atoms))
	for _, a := range atoms {
		vecs, ok := protos[a.ID]
		if !ok {
			continue
		}
		scores = append(scores, Result{AtomID: a.ID, Score: BestCosine(pv[0], vecs)})
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].Score > scores[j].Score })
	if len(scores) > k {
		scores = scores[:k]
	}
	return scores, nil
}
