package main

// `lexicon scaffolded-by <id>` — derived inverse traversal of the
// scaffolds-from edges. Returns atoms whose scaffolds-from list
// contains the target. Per the V118 at panel refinement, the
// `enables[]` / `scaffolded-by` direction is NEVER stored — it's
// always computed on demand, killing the eventual-consistency rot
// that two reciprocated fields would create.
//
// Output: one atom id + name per line (TSV). `-json` switches to JSONL.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/loader"
)

func cmdScaffoldedBy(renderDir string, args []string) {
	fl := flag.NewFlagSet("scaffolded-by", flag.ExitOnError)
	jsonOut := fl.Bool("json", false, "emit JSONL (one entry per line)")
	if err := fl.Parse(args); err != nil {
		fatal("parse flags: %s", err)
	}
	if fl.NArg() == 0 {
		fatal("usage: lexicon scaffolded-by <lex-id>")
	}
	target := fl.Arg(0)

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("load elements: %s", err)
	}
	if _, ok := pool[target]; !ok {
		fatal("no entry %s in elements", target)
	}

	var ids []string
	for id, e := range pool {
		for _, scaf := range e.ScaffoldsFrom {
			if strings.TrimSpace(scaf) == target {
				ids = append(ids, id)
				break
			}
		}
	}
	sort.Strings(ids)

	for _, id := range ids {
		e := pool[id]
		if *jsonOut {
			fmt.Printf("{\"id\":%q,\"name\":%q}\n", id, e.Name)
		} else {
			fmt.Printf("%s\t%s\n", id, e.Name)
		}
	}
	fmt.Fprintf(os.Stderr, "\nscaffolded-by %s: %d atoms\n", target, len(ids))
}
