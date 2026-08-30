package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/viz"
)

// cmdMatrix emits the cluster-sorted adjacency-matrix page. Companion
// to `lexicon pivot` (type pivot) and the web/ SPA's Graph tab — the
// only one of the three with no SPA equivalent yet. Pure Canvas2D
// rendering; no CDN dependency.
func cmdMatrix(renderDir string, args []string) {
	fl := flag.NewFlagSet("matrix", flag.ExitOnError)
	out := fl.String("out", "", "output path for the matrix HTML page (default: render/viz/matrix.html)")
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
	html, err := viz.RenderMatrix(graph)
	if err != nil {
		fatal("render matrix: %s", err)
	}

	target := *out
	if target == "" {
		target = filepath.Join(renderDir, "viz", "matrix.html")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fatal("mkdir: %s", err)
	}
	if err := os.WriteFile(target, html, 0o644); err != nil {
		fatal("write: %s", err)
	}

	fmt.Printf("wrote %s (%d atoms · %d edges)\n", target, len(graph.Nodes), len(graph.Edges))
	fmt.Printf("open:  xdg-open %s\n", target)
}
