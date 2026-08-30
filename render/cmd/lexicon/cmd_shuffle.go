package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/modes"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func cmdShuffle(renderDir string, args []string) {
	fl := flag.NewFlagSet("shuffle", flag.ExitOnError)
	n := fl.Int("n", 3, "number of entries to draw")
	mode := fl.String("mode", "meta-explanatory", "render mode (algebraic|meta-explanatory|narrative|visual|introspection)")
	contextStr := fl.String("context", "", "user's situation (required for narrative mode)")
	filter := fl.String("filter", "", "filter expression (e.g. tier=atomic, status=active, tier=molecule)")
	seed := fl.Int64("seed", 0, "random seed (0 = time-based, nonzero = deterministic)")
	_ = fl.Parse(args)

	if *n < 1 {
		fatal("--n must be >= 1")
	}

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("%v", err)
	}

	candidates := applyShuffleFilter(pool, *filter)
	if len(candidates) == 0 {
		fatal("no entries match filter %q", *filter)
	}

	// Stable initial ordering so a fixed seed produces deterministic output.
	sort.Strings(candidates)

	rngSeed := *seed
	if rngSeed == 0 {
		rngSeed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(rngSeed))
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	take := *n
	if take > len(candidates) {
		take = len(candidates)
	}
	picks := candidates[:take]

	for i, id := range picks {
		entry := pool[id]
		output, err := renderEntryByMode(entry, pool, *mode, *contextStr)
		if err != nil {
			fatal("rendering %s: %v", id, err)
		}
		if i > 0 {
			fmt.Println()
			fmt.Println("---")
			fmt.Println()
		}
		fmt.Printf("# %s (%s)\n", id, entry.Name)
		fmt.Println(output.Text)
	}
}

// applyShuffleFilter returns the IDs of pool entries matching the filter
// expression. Empty filter matches all. Filter format: key=value.
// Supported keys: tier, status.
func applyShuffleFilter(pool map[string]*types.LexEntry, filter string) []string {
	ids := make([]string, 0, len(pool))
	if filter == "" {
		for id := range pool {
			ids = append(ids, id)
		}
		return ids
	}

	parts := strings.SplitN(filter, "=", 2)
	if len(parts) != 2 {
		fatal("filter must be key=value, got %q", filter)
	}
	key, want := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

	for id, entry := range pool {
		var got string
		switch key {
		case "tier":
			got = entry.Tier
		case "status":
			got = entry.Status
		default:
			fatal("unknown filter key %q (supported: tier, status)", key)
		}
		if got == want {
			ids = append(ids, id)
		}
	}
	return ids
}

// renderEntryByMode is the shared per-entry renderer used by shuffle.
// Mirrors cmdRender's mode dispatch but returns the output rather than
// printing directly, so the caller can paginate across multiple entries.
func renderEntryByMode(entry *types.LexEntry, pool map[string]*types.LexEntry, mode, contextStr string) (types.RenderOutput, error) {
	switch types.RenderMode(mode) {
	case types.ModeAlgebraic:
		return modes.Algebraic(entry), nil
	case types.ModeMetaExplanatory:
		return modes.MetaExplanatory(entry), nil
	case types.ModeNarrative:
		if contextStr == "" {
			return types.RenderOutput{}, fmt.Errorf("narrative mode requires --context")
		}
		c, err := client.New()
		if err != nil {
			return types.RenderOutput{}, err
		}
		return modes.Narrative(context.Background(), c, entry, contextStr, nil)
	case types.ModeVisual:
		return modes.Visual(entry, pool), nil
	case types.ModeIntrospection:
		return modes.Introspection(entry, pool), nil
	default:
		return types.RenderOutput{}, fmt.Errorf("unknown mode %q", mode)
	}
}
