package main

// cmd_distinctness.go — `lexicon distinctness <text>`: mining-pass
// audit helper. Pass a candidate atom's name + brief description, get
// the elements atoms most likely to overlap with it, each carrying
// its own "operationally distinct from" entries so you can see what
// near-neighbors it has already distinguished itself against.
//
// Saves the manual rg + grep + read + compare cycle that distinctness
// audit otherwise requires.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/framestatus"
	"github.com/justinstimatze/lexicon/render/internal/gate"
	"github.com/justinstimatze/lexicon/render/internal/lens"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// distinctnessChunkMaxTokens bounds each chunk's estimated index size
// well under Haiku's 200K context (leaves headroom for the fixed system
// prompt, the candidate/user text, output tokens, and estimation slop —
// see lens.EstimateTokens's doc comment on why the estimate itself
// deliberately overshoots).
//
// This exists because a single-call full-pool index overflowed Haiku's
// context once the corpus passed ~3500 atoms (confirmed via `lexicon
// distinctness` stderr: "prompt is too long: 200151 tokens > 200000
// maximum"). An earlier fix here narrowed the pool with embedgate's
// cosine-similarity pre-filter before the lens ever ran it -- cheap, but
// it silently drops whatever the embedding step ranks outside its top-K
// from ever being reasoned about by the LLM at all, which is the wrong
// tradeoff for a tool whose entire job is catching near-duplicates: a
// missed duplicate here isn't a missed UX nicety (the interactive hook's
// failure mode, where embed-gating is the right call), it's a shipped
// duplicate atom.
//
// Instead: lens.ChunkPool splits the FULL pool into token-budgeted
// chunks and every chunk gets scored by the lens (cmdDistinctness runs
// them concurrently below) -- every atom in the corpus gets a real LLM
// look, full stop, and the chunk count grows on its own as the corpus
// grows rather than silently overflowing again at some future size.
const distinctnessChunkMaxTokens = 100_000

func cmdDistinctness(renderDir string, args []string) {
	fl := flag.NewFlagSet("distinctness", flag.ExitOnError)
	topK := fl.Int("top-k", 5, "candidates surfaced (default 5)")
	noLens := fl.Bool("no-lens", false, "skip the LLM-backed semantic lens (lexical-only on full pool)")
	detail := fl.Bool("detail", false, "include each match's critical_questions (default: omitted -- several hundred chars per molecule-tier match, multiplied across every result)")
	args = reorderFlagsFirst(args)
	_ = fl.Parse(args)

	var src io.Reader = os.Stdin
	srcName := "stdin"
	if rest := fl.Args(); len(rest) > 0 && rest[0] != "-" {
		f, err := os.Open(rest[0])
		if err != nil {
			fatal("open %s: %v", rest[0], err)
		}
		defer f.Close()
		src = f
		srcName = rest[0]
	}
	data, err := io.ReadAll(src)
	if err != nil {
		fatal("read %s: %v", srcName, err)
	}
	candidate := strings.TrimSpace(string(data))
	if candidate == "" {
		fatal("distinctness: empty input (from %s)", srcName)
	}

	if os.Getenv("LEXICON_LENS_TIMEOUT_MS") == "" {
		_ = os.Setenv("LEXICON_LENS_TIMEOUT_MS", "30000")
	}

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("load elements: %v", err)
	}
	all := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		all = append(all, e)
	}

	candidatePool := all
	var lensConfidences map[string]float64
	lensUsed := false
	var warning string
	if !*noLens && !lens.Disabled() {
		if c, err := client.New(); err == nil {
			chunks := lens.ChunkPool(all, distinctnessChunkMaxTokens)
			merged, confidences, chunksOK, chunksFailed := scanChunksConcurrently(candidate, chunks, c)
			if chunksOK > 0 {
				candidatePool = merged
				lensConfidences = confidences
				lensUsed = true
				fmt.Fprintf(os.Stderr, "lens: %d -> %d candidates across %d/%d chunks\n", len(all), len(candidatePool), chunksOK, len(chunks))
			}
			if chunksFailed > 0 {
				warning = fmt.Sprintf(
					"%d of %d elements chunks failed even after a retry -- the atoms in those "+
						"chunks were NOT seen by the semantic lens at all this call. Matches below "+
						"are real for the chunks that succeeded, but coverage is incomplete; rerun "+
						"before trusting a clean result.", chunksFailed, len(chunks))
			}
		}
	}
	if !lensUsed && warning == "" {
		warning = "no semantic lens ran (skipped via -no-lens, missing ANTHROPIC_API_KEY, or every " +
			"chunk failed even after a retry) -- these matches are tier/status-only, carry no content " +
			"signal, and are identical for every query at a given top_k. Do not treat them as real " +
			"semantic neighbors."
	}

	fsMap, _ := framestatus.Load(renderDir)

	results := gate.Run(gate.Input{
		Pool:        candidatePool,
		Context:     candidate,
		TopK:        *topK,
		Confidences: lensConfidences,
		FrameStatus: fsMap,
	})

	fmt.Println(formatDistinctnessJSON(candidate, results, pool, fsMap, lensUsed, warning, *detail))
}

// scanChunksConcurrently runs the LLM lens over every chunk in parallel
// (each chunk is an independent, disjoint slice of the corpus — see
// lens.ChunkPool — so there's no shared state to race on beyond the
// result slots each goroutine owns exclusively) and merges the results:
// deduped entries in first-seen order, confidences unioned. A chunk gets
// one retry (mirrors the pre-chunking single-call retry) before counting
// as failed. Returns how many chunks succeeded/failed so the caller can
// warn on partial coverage rather than silently reporting a clean result
// built from less than the full corpus.
func scanChunksConcurrently(candidate string, chunks [][]*types.LexEntry, c client.Client) (merged []*types.LexEntry, confidences map[string]float64, chunksOK, chunksFailed int) {
	type chunkOutcome struct {
		entries     []*types.LexEntry
		confidences map[string]float64
		err         error
	}
	outcomes := make([]chunkOutcome, len(chunks))
	var wg sync.WaitGroup
	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk []*types.LexEntry) {
			defer wg.Done()
			res, err := lens.Filter(context.Background(), candidate, chunk, c, false)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lens: chunk %d/%d failed (%v), retrying once\n", i+1, len(chunks), err)
				res, err = lens.Filter(context.Background(), candidate, chunk, c, false)
			}
			outcomes[i] = chunkOutcome{entries: res.Entries, confidences: res.Confidences, err: err}
		}(i, chunk)
	}
	wg.Wait()

	seen := make(map[string]bool)
	confidences = make(map[string]float64)
	for i, o := range outcomes {
		if o.err != nil {
			chunksFailed++
			fmt.Fprintf(os.Stderr, "lens: chunk %d/%d retry also failed (%v), its atoms were not considered\n", i+1, len(chunks), o.err)
			continue
		}
		chunksOK++
		for _, e := range o.entries {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			merged = append(merged, e)
		}
		for id, conf := range o.confidences {
			confidences[id] = conf
		}
	}
	return merged, confidences, chunksOK, chunksFailed
}

func formatDistinctnessJSON(candidate string, results []types.GateResult, pool map[string]*types.LexEntry, fsMap framestatus.Map, lensUsed bool, warning string, detail bool) string {
	type match struct {
		ID                        string   `json:"id"`
		Name                      string   `json:"name"`
		Tier                      string   `json:"tier"`
		Score                     float64  `json:"score"`
		Status                    string   `json:"status,omitempty"`
		FrameStatus               string   `json:"frame_status,omitempty"`
		Gloss                     string   `json:"gloss,omitempty"`
		AgentInstruction          string   `json:"agent_instruction,omitempty"`
		TypeIn                    string   `json:"type_in,omitempty"`
		TypeOut                   string   `json:"type_out,omitempty"`
		CriticalQuestions         []string `json:"critical_questions,omitempty"`
		OperationallyDistinctFrom []string `json:"operationally_distinct_from,omitempty"`
	}
	type out struct {
		Candidate string  `json:"candidate"`
		LensUsed  bool    `json:"lens_used"`
		Warning   string  `json:"warning,omitempty"`
		Matches   []match `json:"matches"`
	}

	matches := make([]match, 0, len(results))
	for _, r := range results {
		e := pool[r.PrimitiveID]
		if e == nil {
			continue
		}
		m := match{
			ID:               e.ID,
			Name:             e.Name,
			Tier:             e.Tier,
			Score:            r.Score,
			Status:           e.Status,
			Gloss:            patternGloss(e),
			AgentInstruction: e.AgentInstruction,
			TypeIn:           e.TypeIn,
			TypeOut:          e.TypeOut,
		}
		if fs, ok := fsMap.Lookup(e.ID); ok {
			m.FrameStatus = string(fs.Status)
		}
		if detail && len(e.CriticalQuestions) > 0 {
			m.CriticalQuestions = e.CriticalQuestions
		}
		for _, ci := range e.CanonicalInstances {
			if strings.Contains(ci, "Operationally distinct from") {
				m.OperationallyDistinctFrom = append(m.OperationallyDistinctFrom, ci)
			}
		}
		matches = append(matches, m)
	}

	doc := out{
		Candidate: strings.TrimSpace(candidate),
		LensUsed:  lensUsed,
		Warning:   warning,
		Matches:   matches,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}
