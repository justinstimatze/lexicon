package main

// `lexicon anti-patterns` — enumerate (n−1)-subsets of every molecule's
// `decomposes-into:` and emit candidate anti-patterns. The host
// molecule minus one atom names a recognizable failure-mode when
// the missing piece is the diagnostic.
//
// Output: a markdown table to stdout, intended for the maintainer to
// fill in the failure-mode and cross-domain-attestation columns by
// hand. Code's job is the candidate list; cross-domain attestation
// is a discipline question, not an algorithm.
//
// Delivers what composition-operations.md §"Why bonds matter"
// promises but never ships.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/assembly"
	"github.com/justinstimatze/lexicon/render/internal/loader"
)

func cmdAntiPatterns(renderDir string, args []string) {
	fl := flag.NewFlagSet("anti-patterns", flag.ExitOnError)
	parent := fl.String("parent", "", "limit output to anti-patterns of this parent molecule (lex-NNNN)")
	fl.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lexicon anti-patterns [--parent <lex-id>]")
		fmt.Fprintln(os.Stderr, "  Emit a markdown table of candidate anti-patterns derived from")
		fmt.Fprintln(os.Stderr, "  every molecule's decomposes-into (n-1)-subsets.")
		fl.PrintDefaults()
	}
	_ = fl.Parse(args)

	elementsDir := filepath.Join(renderDir, "..", "elements")
	sub, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("load elements: %v", err)
	}

	all := assembly.EnumerateAntiPatterns(sub)
	if *parent != "" {
		var filtered []assembly.AntiPattern
		for _, ap := range all {
			if ap.ParentID == *parent {
				filtered = append(filtered, ap)
			}
		}
		all = filtered
	}

	emitAntiPatternMarkdown(all)

	// Stats to stderr
	parents := map[string]bool{}
	for _, ap := range all {
		parents[ap.ParentID] = true
	}
	fmt.Fprintf(os.Stderr, "\nanti-patterns: %d candidates across %d parent molecules\n", len(all), len(parents))
}

func emitAntiPatternMarkdown(aps []assembly.AntiPattern) {
	if len(aps) == 0 {
		fmt.Println("(no anti-patterns: no molecules with 2 ≤ |decomposes-into| ≤ 8 in elements)")
		return
	}
	fmt.Println("| anti-pattern name | parent | missing atom | remaining | failure-mode | cross-domain attestation |")
	fmt.Println("|---|---|---|---|---|---|")
	for _, ap := range aps {
		remaining := strings.Join(ap.RemainingNames, ", ")
		fmt.Printf("| %s | %s (%s) | %s (%s) | %s | %s | %s |\n",
			ap.Name,
			ap.ParentName, ap.ParentID,
			ap.MissingName, ap.MissingID,
			remaining,
			ap.FailureModeTODO,
			ap.AttestationTODO,
		)
	}
}
