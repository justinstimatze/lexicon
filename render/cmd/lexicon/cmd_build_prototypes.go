package main

// cmd_build_prototypes.go — warm the embedgate prototype cache. One-shot
// subcommand; run after elements changes (or on first use) to force a fresh
// embedding pass. The cache lives at ~/.claude/lexicon/prototypes.json, keyed
// per-atom by content hash (so only new/edited atoms re-embed) and wholesale
// by model — explicit warm is for predictable first-call latency, not
// correctness.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/embedgate"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func cmdBuildPrototypes(renderDir string, args []string) {
	fs := flag.NewFlagSet("build-prototypes", flag.ExitOnError)
	force := fs.Bool("force", false, "delete cache before building (forces recompute even on hit)")
	_ = fs.Parse(args)

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-prototypes: loader: %s\n", err)
		os.Exit(1)
	}
	atoms := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		atoms = append(atoms, e)
	}

	if *force {
		_ = os.Remove(embedgate.CachePath())
	}

	fmt.Printf("model: %s\nollama: %s\natoms: %d\ncache: %s\n",
		embedgate.EmbedModel(), embedgate.OllamaURL(), len(atoms), embedgate.CachePath())

	t0 := time.Now()
	protos, err := embedgate.LoadOrBuildPrototypes(context.Background(), atoms)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-prototypes: %s\n", err)
		fmt.Fprintln(os.Stderr, "hint: is ollama running? (curl -fsS "+embedgate.OllamaURL()+"/api/tags)")
		os.Exit(1)
	}
	fmt.Printf("built %d prototypes in %s\n", len(protos), time.Since(t0).Round(time.Millisecond))
}
