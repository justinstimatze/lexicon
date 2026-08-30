package main

// `lexicon tarot` — explicit oblique-provocation mode (V13 TASK 8).
//
// The hook auto-fires this same shape inline when the lens detects a
// stuck-signal in the prompt; this subcommand is for when the user
// wants to draw cards explicitly, regardless of signal detection.
//
// Pure shuffle — no LLM, no relevance scoring, no filtering by current
// context. Eno's whole point: random IS the feature; the user's
// interpretive work against an arbitrary constraint is the engine.
// Mirrors Tarot deck/draw/interpret — elements/ is the deck, shuffle
// is the draw, user is the reader.

import (
	"flag"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func cmdTarot(renderDir string, args []string) {
	fl := flag.NewFlagSet("tarot", flag.ExitOnError)
	n := fl.Int("n", 3, "number of cards to draw")
	seed := fl.Int64("seed", 0, "RNG seed for reproducibility (0 = time-based)")
	includeUnderReview := fl.Bool("under-review", false, "draw from under-review entries too (default: active only)")
	_ = fl.Parse(args)

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("%v", err)
	}

	candidates := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		if e.Tier == "sub-atomic" || e.Status == "deprecated" {
			continue
		}
		if !*includeUnderReview && e.Status != "active" {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		fmt.Println("(no eligible primitives in elements)")
		return
	}

	// Sort by ID before shuffling. Go map iteration order is randomized per
	// execution, so without this normalization `--seed N` would still produce
	// different draws on back-to-back runs (the shuffle is deterministic, but
	// the slice being shuffled isn't).
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ID < candidates[j].ID
	})

	// Use a local seeded RNG. In Go 1.20+ the package-level `rand.Seed` is a
	// no-op against the global source, so calling it does NOT make the
	// subsequent `rand.Shuffle` reproducible. A local *rand.Rand with an
	// explicit source is the supported path.
	seedVal := *seed
	if seedVal == 0 {
		seedVal = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seedVal))
	rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	draws := *n
	if draws > len(candidates) {
		draws = len(candidates)
	}
	picks := candidates[:draws]

	fmt.Println("Tarot draw (oblique provocation; treat as Eno cards):")
	fmt.Println()
	for _, e := range picks {
		fmt.Printf("- %s (%s, %s → %s, %s)\n", e.Name, e.ID, e.TypeIn, e.TypeOut, e.Tier)
		if len(e.CanonicalInstances) > 0 {
			ex := strings.ReplaceAll(e.CanonicalInstances[0], "\n", " ")
			if len(ex) > 220 {
				ex = ex[:217] + "..."
			}
			fmt.Printf("  %s\n", ex)
		}
		fmt.Println()
	}
	fmt.Println("Worth asking, for each:")
	fmt.Println("  - how would this apply if you took it literally?")
	fmt.Println("  - what does it ignore that you've been treating as essential?")
	fmt.Println("  - what's the inverse, and is the inverse what you actually want?")
}
