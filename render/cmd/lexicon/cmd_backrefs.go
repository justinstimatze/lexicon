package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/justinstimatze/lexicon/render/internal/loader"
)

// cmdBackrefs prints the set of atoms that reference the target ID in
// their `related:` lists, with name + status badge per entry.
//
// Because the reciprocation gate already enforces that `related:` is
// bidirectional, the result is equivalent to the target atom's own
// `related:` list — but a dedicated subcommand is the discovery
// affordance: downstream consumers (inkling et al.) want to ask
// "which atoms point AT lex-NNNN" without knowing or reading
// lex-NNNN's outbound edges first.
//
// Per ROADMAP item #16.2 — surfaced by inkling sibling-project
// feedback (V103 2026-06-05). See ROADMAP.md.
//
// Output format (one entry per line):
//
//	lex-NNNN  [status]  name
//
// Status badge is colorless for portability; consumers can pipe to
// awk / column / fzf as needed.
func cmdBackrefs(renderDir string, args []string) {
	fl := flag.NewFlagSet("backrefs", flag.ExitOnError)
	statusFilter := fl.String("status", "", "only show entries with this status (e.g. active, under-review)")
	idsOnly := fl.Bool("ids", false, "print only lex-IDs, one per line (suitable for piping)")
	fl.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lexicon backrefs <lex-NNNN> [--status=STATUS] [--ids]")
		fmt.Fprintln(os.Stderr, "  Print atoms that reference the target ID in their related: list.")
		fl.PrintDefaults()
	}
	_ = fl.Parse(args)

	if fl.NArg() != 1 {
		fl.Usage()
		os.Exit(2)
	}
	target := fl.Arg(0)

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("%v", err)
	}

	if _, ok := pool[target]; !ok {
		fatal("backrefs: %s not in elements", target)
	}

	var hits []string
	for id, e := range pool {
		if id == target {
			continue
		}
		for _, r := range e.Related {
			if r == target {
				hits = append(hits, id)
				break
			}
		}
	}
	sort.Strings(hits)

	if *idsOnly {
		for _, id := range hits {
			fmt.Println(id)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "backrefs: %d atom(s) reference %s\n", len(hits), target)
	for _, id := range hits {
		e := pool[id]
		if *statusFilter != "" && e.Status != *statusFilter {
			continue
		}
		status := e.Status
		if status == "" {
			status = "?"
		}
		fmt.Printf("%-9s  [%-12s]  %s\n", id, status, e.Name)
	}
}
