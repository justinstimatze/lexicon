package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/viz"
)

// cmdShell emits the unified app shell composing the matrix and pivot
// panels with a persistent detail pane. This is the modern landing
// page; the standalone `matrix` and `pivot` commands remain as legacy
// direct-access pages.
func cmdShell(renderDir string, args []string) {
	fl := flag.NewFlagSet("shell", flag.ExitOnError)
	out := fl.String("out", "", "output path for the shell HTML page (default: render/viz/index.html)")
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
	html, err := viz.RenderShell(graph)
	if err != nil {
		fatal("render shell: %s", err)
	}

	target := *out
	if target == "" {
		target = filepath.Join(renderDir, "viz", "index.html")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fatal("mkdir: %s", err)
	}
	if err := os.WriteFile(target, html, 0o644); err != nil {
		fatal("write: %s", err)
	}

	fmt.Printf("wrote %s (%d atoms · %d edges · %d clusters)\n", target, len(graph.Nodes), len(graph.Edges), len(graph.Clusters))
	fmt.Printf("open:  xdg-open %s\n", target)
}
