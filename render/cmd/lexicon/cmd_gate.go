package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/framestatus"
	"github.com/justinstimatze/lexicon/render/internal/gate"
	"github.com/justinstimatze/lexicon/render/internal/lens"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func cmdGate(renderDir string, args []string) {
	fl := flag.NewFlagSet("gate", flag.ExitOnError)
	contextStr := fl.String("context", "", "current elements-shape context")
	vocab := fl.String("vocab", "", "comma-separated working vocabulary")
	topK := fl.Int("top-k", gate.DefaultTopK, "max results")
	noLens := fl.Bool("no-lens", false, "skip the LLM-backed semantic lens (lexical-only re-rank on full pool)")
	_ = fl.Parse(args)

	// The lens default timeout (8s) is tuned for the UserPromptSubmit hook
	// hot path. Interactive/batch CLI use can afford to wait, and dense
	// contexts otherwise time out into a degenerate lexical fallback. Give
	// the CLI a generous default; an explicit env value still wins.
	if os.Getenv("LEXICON_LENS_TIMEOUT_MS") == "" {
		_ = os.Setenv("LEXICON_LENS_TIMEOUT_MS", "30000")
	}

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("%v", err)
	}
	entries := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		entries = append(entries, e)
	}

	var workingVocab []string
	if *vocab != "" {
		for _, w := range strings.Split(*vocab, ",") {
			w = strings.TrimSpace(w)
			if w != "" {
				workingVocab = append(workingVocab, w)
			}
		}
	}

	// V13: mirror the hook's lens-then-gate flow. Lens is skipped when
	// --no-lens is set, when the lens module is disabled (no API key /
	// LEXICON_LENS_DISABLED=1), or when --context is empty (nothing to
	// filter against — fall back to pure lexical scoring on the full
	// pool, the pre-V13 behavior).
	candidatePool := entries
	var lensConfidences map[string]float64
	lensSkipped := *noLens || lens.Disabled() || strings.TrimSpace(*contextStr) == ""
	if !lensSkipped {
		c, err := client.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "lens: client init: %v (falling back to lexical full-pool)\n", err)
		} else {
			lensRes, lensErr := lens.Filter(context.Background(), *contextStr, entries, c, false)
			cacheNote := ""
			if lensRes.Usage.CacheReadTokens > 0 {
				cacheNote = fmt.Sprintf(" [cache-hit %d tok]", lensRes.Usage.CacheReadTokens)
			} else if lensRes.Usage.CacheCreationTokens > 0 {
				cacheNote = fmt.Sprintf(" [cache-write %d tok]", lensRes.Usage.CacheCreationTokens)
			}
			if lensErr != nil {
				fmt.Fprintf(os.Stderr, "lens: %v (falling back to lexical full-pool)%s\n", lensErr, cacheNote)
			} else if len(lensRes.Entries) == 0 {
				fmt.Fprintf(os.Stderr, "lens: no semantically relevant primitives%s\n", cacheNote)
				if lensRes.StuckSignal {
					fmt.Fprintln(os.Stderr, "  (stuck-signal detected — try `lexicon tarot`)")
				}
				if lensRes.ContradictionSignal {
					fmt.Fprintf(os.Stderr, "  (contradiction-signal: %s)\n", lensRes.ContradictionPhrasing)
				}
				return
			} else {
				topConf := lensRes.Confidences[lensRes.Entries[0].ID]
				fmt.Fprintf(os.Stderr, "lens: filtered %d -> %d candidates (top confidence %.2f)%s\n",
					len(entries), len(lensRes.Entries), topConf, cacheNote)
				if lensRes.StuckSignal {
					fmt.Fprintln(os.Stderr, "  signal: stuck (`lexicon tarot` for oblique alternative)")
				}
				if lensRes.ContradictionSignal {
					fmt.Fprintf(os.Stderr, "  signal: contradiction — %s\n", lensRes.ContradictionPhrasing)
				}
				candidatePool = lensRes.Entries
				lensConfidences = lensRes.Confidences
			}
		}
	}

	fsMap, fsErr := framestatus.Load(renderDir)
	if fsErr != nil {
		fmt.Fprintf(os.Stderr, "frame-status: %v (running without frame down-weighting)\n", fsErr)
	}

	results := gate.Run(gate.Input{
		Pool:         candidatePool,
		Context:      *contextStr,
		WorkingVocab: workingVocab,
		TopK:         *topK,
		Confidences:  lensConfidences,
		FrameStatus:  fsMap,
	})
	for _, r := range results {
		fmt.Println(gate.FormatResult(r))
	}
}
