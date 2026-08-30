// Package lexicon is the stable, directly-callable Go API for lexicon's
// pattern-match scoring path -- the same logic `lexicon read` runs, minus
// the CLI's flag parsing and stdout formatting. It exists so a sibling
// project (freshet and others) can call this in-process instead of
// shelling out to the built binary and parsing its JSON back out.
//
// The CLI (cmd/lexicon) is itself a thin wrapper over this package, not a
// separate implementation -- see cmd/lexicon/cmd_read.go and cmd_what_if.go.
package lexicon

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/embedgate"
	"github.com/justinstimatze/lexicon/render/internal/framestatus"
	"github.com/justinstimatze/lexicon/render/internal/gate"
	"github.com/justinstimatze/lexicon/render/internal/lens"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

const (
	maxAdjacencies = 6
	maxCQs         = 3
)

// ReadOptions configures a Read call. The zero value runs top_k=3 with
// the semantic lens enabled.
type ReadOptions struct {
	// TopK is how many patterns to surface. Defaults to 3 if <= 0.
	TopK int
	// NoLens skips the LLM-backed semantic lens and scores lexically over
	// the full pool -- faster, no API call, but prompt-vocab-dependent.
	NoLens bool
}

// Adjacency is a related atom, resolved to its own name and one-line gloss.
type Adjacency struct {
	ID    string
	Name  string
	Gloss string
}

// Pattern is one surfaced atom plus the fields a caller typically needs
// without a second lookup: its own score, whether the match had real
// lexical grounding, frame-status (Principle 10 honesty labels, if the
// oracle-risk register has an entry), a one-line gloss, the full
// agent-instruction, up to 3 critical-questions, and up to 6 adjacencies.
type Pattern struct {
	ID                string
	Name              string
	Tier              string
	Score             float64
	LexicalMatch      bool
	FrameStatus       string
	FrameHandle       string
	Gloss             string
	AgentInstruction  string
	CriticalQuestions []string
	Adjacencies       []Adjacency
}

// Result is what Read returns: the resolved patterns plus enough context
// to know how they were produced. Diagnostics carries the same
// progress/fallback lines the CLI prints to stderr (embed-gate narrowing,
// lens fallback, etc.) as plain strings, so a caller can log them without
// Read writing to stderr itself.
type Result struct {
	Context     string
	TopK        int
	LensUsed    bool
	Patterns    []Pattern
	Diagnostics []string
}

// Corpus is the elements pool and frame-status register, loaded once and
// reused across many Score calls -- the right choice for a caller
// scoring more than a handful of passages (a batch pipeline, a
// multi-partition sweep), since loading is the expensive part and
// scoring itself is cheap once it's in memory. Read is a one-shot
// convenience wrapper for occasional callers that don't want to manage
// a Corpus's lifetime themselves.
type Corpus struct {
	renderDir  string
	pool       map[string]*types.LexEntry
	allEntries []*types.LexEntry
	fsMap      framestatus.Map
}

// Pool is the loaded elements, keyed by atom id -- for a caller building
// its own output shape from ScoreRaw's raw *types.LexEntry results
// (resolving adjacencies, looking up a neighbor by id) rather than using
// Score's Pattern struct.
func (corp *Corpus) Pool() map[string]*types.LexEntry { return corp.pool }

// FrameStatus is the loaded oracle-risk register (Principle 10 honesty
// labels), for a caller building its own output shape.
func (corp *Corpus) FrameStatus() framestatus.Map { return corp.fsMap }

// LoadCorpus loads the elements pool rooted at
// filepath.Join(renderDir, "..", "elements") (loader.DefaultElementsDir)
// and the frame-status register, once. renderDir is the render/
// directory's own path (matching the CLI's own -C/cwd convention) so a
// caller running from anywhere can point this at a specific checkout.
func LoadCorpus(renderDir string) (*Corpus, error) {
	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		return nil, fmt.Errorf("lexicon: load elements: %w", err)
	}
	allEntries := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		allEntries = append(allEntries, e)
	}
	// A missing/unparseable register just drops the honesty labels
	// rather than failing the load -- fsMap stays its zero value, and
	// Lookup on it always misses.
	fsMap, _ := framestatus.Load(renderDir)
	return &Corpus{renderDir: renderDir, pool: pool, allEntries: allEntries, fsMap: fsMap}, nil
}

// Read is LoadCorpus followed by Score, for a caller scoring a single
// passage who doesn't need the Corpus afterward.
func Read(ctx context.Context, renderDir, contextStr string, opts ReadOptions) (*Result, error) {
	c, err := LoadCorpus(renderDir)
	if err != nil {
		return nil, err
	}
	return c.Score(ctx, contextStr, opts)
}

// Score scores contextStr against the loaded corpus and returns the
// top-K surfaced patterns.
//
// This does one real network call when the lens is enabled and reachable
// (an Anthropic-backed semantic filter, narrowed first by a local-embed
// pre-filter when its prototype cache is warm) -- pass NoLens: true for a
// fully local, lexical-only score with no network dependency.
func (corp *Corpus) Score(ctx context.Context, contextStr string, opts ReadOptions) (*Result, error) {
	contextStr = strings.TrimSpace(contextStr)
	if contextStr == "" {
		return nil, errors.New("lexicon: empty context")
	}
	topK := opts.TopK
	if topK <= 0 {
		topK = 3
	}

	picked, scores, lexMatch, lensUsed, diag := corp.ScoreRaw(ctx, contextStr, topK, opts.NoLens)

	patterns := make([]Pattern, 0, len(picked))
	for _, e := range picked {
		tier := e.Tier
		if tier == "" {
			tier = "atomic"
		}
		p := Pattern{
			ID:               e.ID,
			Name:             e.Name,
			Tier:             tier,
			Score:            scores[e.ID],
			LexicalMatch:     lexMatch[e.ID],
			Gloss:            gloss(e),
			AgentInstruction: e.AgentInstruction,
		}
		if fs, ok := corp.fsMap.Lookup(e.ID); ok {
			p.FrameStatus = string(fs.Status)
			p.FrameHandle = fs.Handle
		}
		if n := len(e.CriticalQuestions); n > 0 {
			if n > maxCQs {
				n = maxCQs
			}
			p.CriticalQuestions = e.CriticalQuestions[:n]
		}
		for _, nb := range adjacenciesOf(e, corp.pool, maxAdjacencies) {
			p.Adjacencies = append(p.Adjacencies, Adjacency{ID: nb.ID, Name: nb.Name, Gloss: gloss(nb)})
		}
		patterns = append(patterns, p)
	}

	return &Result{
		Context:     contextStr,
		TopK:        topK,
		LensUsed:    lensUsed,
		Patterns:    patterns,
		Diagnostics: diag,
	}, nil
}

// ScoreRaw is Score's underlying logic, returning the same
// entries-plus-maps shape the CLI's pre-package implementation used
// internally -- for a caller building its own output shape (lexicon's
// own `partitions`/`council` commands do exactly this: they need raw
// *types.LexEntry access per surfaced atom, not the Pattern struct).
// Prefer Score unless you specifically need this.
func (corp *Corpus) ScoreRaw(ctx context.Context, contextStr string, topK int, noLens bool) (picked []*types.LexEntry, scores map[string]float64, lexMatch map[string]bool, lensUsed bool, diag []string) {
	pool, allEntries, fsMap := corp.pool, corp.allEntries, corp.fsMap
	candidatePool := allEntries
	var lensConfidences map[string]float64
	if !noLens && !lens.Disabled() {
		if c, err := client.New(); err != nil {
			diag = append(diag, fmt.Sprintf("lens: client init: %v (falling back to lexical full-pool)", err))
		} else {
			lensInput := allEntries
			embedCtx, embedCancel := context.WithTimeout(ctx, ResolveEmbedGateBudget())
			gateRes, gateErr := embedgate.Score(embedCtx, contextStr, allEntries, embedgate.TopK())
			embedCancel()
			switch {
			case errors.Is(gateErr, embedgate.ErrColdCache):
				diag = append(diag, "embed gate: prototype cache cold (run: lexicon build-prototypes); lens sees full pool")
			case gateErr != nil:
				diag = append(diag, fmt.Sprintf("embed gate: %v (lens sees full pool)", gateErr))
			case len(gateRes) > 0:
				if narrowed, active, logMsg := DecideEmbedNarrowing(gateRes, pool, embedgate.Threshold(), len(allEntries)); active && len(narrowed) > 0 {
					diag = append(diag, logMsg)
					lensInput = narrowed
				}
			}

			lensRes, lensErr := lens.Filter(ctx, contextStr, lensInput, c, false)
			switch {
			case lensErr != nil:
				diag = append(diag, fmt.Sprintf("lens: %v (falling back to lexical full-pool)", lensErr))
			case len(lensRes.Entries) == 0:
				diag = append(diag, "lens: no semantically relevant primitives — falling back to lexical full-pool")
			default:
				diag = append(diag, fmt.Sprintf("lens: %d -> %d candidates", len(lensInput), len(lensRes.Entries)))
				candidatePool = lensRes.Entries
				lensConfidences = lensRes.Confidences
				lensUsed = true
			}
		}
	}

	results := gate.Run(gate.Input{
		Pool:         candidatePool,
		Context:      contextStr,
		TopK:         topK,
		WorkingVocab: ExtractPromptVocab(contextStr),
		Confidences:  lensConfidences,
		FrameStatus:  fsMap,
	})
	if len(results) == 0 {
		return nil, nil, nil, lensUsed, diag
	}

	picked = make([]*types.LexEntry, 0, len(results))
	scores = make(map[string]float64, len(results))
	lexMatch = make(map[string]bool, len(results))
	for _, r := range results {
		if e := pool[r.PrimitiveID]; e != nil {
			picked = append(picked, e)
			scores[r.PrimitiveID] = r.Score
			lexMatch[r.PrimitiveID] = strings.Contains(r.Reason, "token-in-vocab")
		}
	}
	return picked, scores, lexMatch, lensUsed, diag
}

func gloss(e *types.LexEntry) string {
	if e == nil {
		return ""
	}
	if len(e.CanonicalInstances) > 0 {
		return truncateGreedy(e.CanonicalInstances[0], 140)
	}
	if len(e.Evokes) > 0 {
		return truncateGreedy(e.Evokes[0], 140)
	}
	return ""
}

func truncateGreedy(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// adjacenciesOf resolves an atom's related-list to LexEntry pointers,
// skipping self-references and missing pool entries, capped at max.
func adjacenciesOf(e *types.LexEntry, pool map[string]*types.LexEntry, max int) []*types.LexEntry {
	out := make([]*types.LexEntry, 0, max)
	for _, id := range e.Related {
		if id == e.ID {
			continue
		}
		neighbor, ok := pool[id]
		if !ok || neighbor == nil {
			continue
		}
		out = append(out, neighbor)
		if len(out) >= max {
			break
		}
	}
	return out
}
