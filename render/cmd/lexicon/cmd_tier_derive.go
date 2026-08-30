package main

// `lexicon tier-derive` — compute the derived encounter-tier (1-5)
// for every atom from existing fields (lineage tradition, source-text
// signal, in-degree). Per the V118 at panel refinement, encounter-tier
// is NOT a stored field; it is a derived view. Atoms may carry an
// optional `encounter-tier-override` for the rare Hofstadter-translation
// case (tier-5 content compressed into tier-2 prose); divergence from
// the derived value emits info via lint.
//
// Tiers (mirrors SCHEMA.md):
//   1 — proverbial    lands without framing; could say to a stranger
//   2 — plain         lands after one sentence of framing
//   3 — structural    needs a paragraph or domain knowledge to land
//   4 — counter-intuitive   requires surrendering a strong prior
//   5 — esoteric      needs sustained training before it lands
//
// Output: one line per atom, sorted by id. Default emits TSV
// (id, derived, override, divergence, primary-tradition, reasoning).
// `-json` switches to JSONL. `-skew` summarizes the tier histogram.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func cmdTierDerive(renderDir string, args []string) {
	fl := flag.NewFlagSet("tier-derive", flag.ExitOnError)
	jsonOut := fl.Bool("json", false, "emit JSONL (one atom per line)")
	skewOnly := fl.Bool("skew", false, "emit only the tier histogram, not per-atom rows")
	if err := fl.Parse(args); err != nil {
		fatal("parse flags: %s", err)
	}

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("load elements: %s", err)
	}

	inDeg := map[string]int{}
	for _, e := range pool {
		for _, rel := range e.Related {
			inDeg[rel]++
		}
		for _, dec := range e.DecomposesInto {
			inDeg[dec]++
		}
	}

	ids := make([]string, 0, len(pool))
	for id := range pool {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	hist := [6]int{}
	overrideCount := 0
	divergeCount := 0
	for _, id := range ids {
		e := pool[id]
		derived, reasoning := deriveTier(e, inDeg[id])
		override := e.EncounterTierOverride
		divergence := 0
		if override > 0 {
			overrideCount++
			divergence = override - derived
		}
		if divergence < -1 || divergence > 1 {
			divergeCount++
		}
		hist[derived]++
		if *skewOnly {
			continue
		}
		if *jsonOut {
			fmt.Printf("{\"id\":%q,\"derived\":%d,\"override\":%d,\"divergence\":%d,\"tradition\":%q,\"reasoning\":%q}\n",
				id, derived, override, divergence, primaryTradition(e), reasoning)
		} else {
			fmt.Printf("%s\t%d\t%d\t%+d\t%s\t%s\n", id, derived, override, divergence, primaryTradition(e), reasoning)
		}
	}

	fmt.Fprintf(os.Stderr, "\ntier-derive: %d atoms\n", len(ids))
	for t := 1; t <= 5; t++ {
		bar := strings.Repeat("█", hist[t]/10)
		fmt.Fprintf(os.Stderr, "  tier %d %4d %s\n", t, hist[t], bar)
	}
	fmt.Fprintf(os.Stderr, "  overrides: %d set, %d diverge >1 from derived\n", overrideCount, divergeCount)
}

// deriveTier maps an atom's existing signals to a 1-5 encounter-tier.
// Heuristic, not measurement; the override field exists for the cases
// the heuristic gets wrong.
func deriveTier(e *types.LexEntry, inDegree int) (int, string) {
	var reasons []string
	tier := 3 // default to the elements-primitive zone

	tradition := strings.ToLower(primaryTradition(e))
	source := ""
	if len(e.Lineage) > 0 {
		source = strings.ToLower(e.Lineage[0].Text)
	}

	switch {
	case containsAny(tradition, "folk-wisdom", "proverb", "kotowaza", "vernacular", "saying"):
		tier = 1
		reasons = append(reasons, "tradition=folk")
	case containsAny(tradition, "buddhist", "zen", "mahayana", "taoist", "dao", "advaita", "tantric", "kabbalist", "sufi"):
		tier = 5
		reasons = append(reasons, "tradition=esoteric")
	case containsAny(tradition, "phenomenology", "continental", "post-structural", "deconstruction", "hegel", "heidegger"):
		tier = 4
		reasons = append(reasons, "tradition=continental")
	case containsAny(tradition, "formal", "godel", "tarski", "logic-formal", "category-theory"):
		tier = 5
		reasons = append(reasons, "tradition=formal")
	case containsAny(tradition, "pop-science", "trade-nonfiction", "gladwell", "kahneman", "popularizer"):
		tier = 2
		reasons = append(reasons, "tradition=pop-science")
	case containsAny(source, "tao-te-ching", "zhuangzi", "wumenguan", "wu-men-kuan", "upanisads", "upanishads", "bhagavad", "advaita", "dogen", "nagarjuna"):
		tier = 5
		reasons = append(reasons, "source=esoteric-primary")
	case containsAny(source, "hegel", "heidegger", "derrida", "husserl", "nietzsche-beyond"):
		tier = 4
		reasons = append(reasons, "source=continental-primary")
	case containsAny(source, "godel", "tarski", "wittgenstein-tractatus", "russell-pm"):
		tier = 5
		reasons = append(reasons, "source=formal-primary")
	case containsAny(source, "galef", "mieder", "owomoyela", "burckhardt", "dahl-russian", "lin-yutang", "plopper", "speake"):
		tier = 1
		reasons = append(reasons, "source=proverb-collection")
	case containsAny(source, "aristotle-nicomachean", "aristotle-rhetoric", "hume-treatise", "hume-enquiry", "mill-system-of-logic", "kant-groundwork", "schopenhauer"):
		tier = 3
		reasons = append(reasons, "source=academic-primary")
	default:
		reasons = append(reasons, "default=structural-3")
	}

	if inDegree >= 12 {
		if tier > 3 {
			tier--
			reasons = append(reasons, fmt.Sprintf("hub-pull (in-degree %d)", inDegree))
		}
	}
	if inDegree == 0 && tier < 5 {
		// Leaf atoms drift outward — but only if not already at the floor.
		if tier > 1 {
			reasons = append(reasons, "leaf (in-degree 0)")
		}
	}

	if tier < 1 {
		tier = 1
	}
	if tier > 5 {
		tier = 5
	}
	return tier, strings.Join(reasons, "; ")
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(s, n) {
			return true
		}
	}
	return false
}
