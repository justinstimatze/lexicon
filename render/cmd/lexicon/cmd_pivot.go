package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/viz"
)

// cmdPivot emits the static elements pivot-table page (type-in rows
// × type-out columns). Companion to `lexicon matrix`; the two pages
// share the elements JSON and link to each other.
// Default output path: render/viz/pivot.html.
func cmdPivot(renderDir string, args []string) {
	fl := flag.NewFlagSet("pivot", flag.ExitOnError)
	out := fl.String("out", "", "output path for the pivot HTML page (default: render/viz/pivot.html)")
	if err := fl.Parse(args); err != nil {
		fatal("parse flags: %s", err)
	}

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("loader: %s", err)
	}

	graph := viz.ToGraph(pool, clusterContinuityPath(renderDir))
	html, err := viz.RenderPivot(graph)
	if err != nil {
		fatal("render pivot: %s", err)
	}

	target := *out
	if target == "" {
		target = filepath.Join(renderDir, "viz", "pivot.html")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fatal("mkdir: %s", err)
	}
	if err := os.WriteFile(target, html, 0o644); err != nil {
		fatal("write: %s", err)
	}

	fmt.Printf("wrote %s (%d atoms)\n", target, len(graph.Nodes))
	fmt.Printf("open:  xdg-open %s\n", target)
}
