package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/modes"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func cmdRender(renderDir string, args []string) {
	if len(args) < 1 {
		fatal("missing lex-id positional")
	}
	id := args[0]
	fl := flag.NewFlagSet("render", flag.ExitOnError)
	mode := fl.String("mode", "meta-explanatory", "render mode")
	contextStr := fl.String("context", "", "user's situation (required for narrative)")
	why := fl.Bool("why", false, "append introspection-trace to output")
	// Skip positional (args[0]); stdlib flag stops at first non-flag,
	// so positional must be peeled off before parsing.
	_ = fl.Parse(args[1:])

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)

	entry, err := loader.Load(elementsDir, id)
	if err != nil {
		fatal("%v", err)
	}

	var output types.RenderOutput
	switch types.RenderMode(*mode) {
	case types.ModeAlgebraic:
		output = modes.Algebraic(entry)
	case types.ModeMetaExplanatory:
		output = modes.MetaExplanatory(entry)
	case types.ModeNarrative:
		if *contextStr == "" {
			fatal("narrative mode requires --context describing the user's situation")
		}
		c, err := client.New()
		if err != nil {
			fatal("%v", err)
		}
		out, err := modes.Narrative(context.Background(), c, entry, *contextStr, nil)
		if err != nil {
			fatal("%v", err)
		}
		output = out
	case types.ModeVisual:
		pool, err := loader.LoadAll(elementsDir)
		if err != nil {
			fatal("%v", err)
		}
		output = modes.Visual(entry, pool)
	case types.ModeIntrospection:
		pool, err := loader.LoadAll(elementsDir)
		if err != nil {
			fatal("%v", err)
		}
		output = modes.Introspection(entry, pool)
	default:
		fatal("unknown mode %q", *mode)
	}

	fmt.Println(output.Text)
	if *why {
		fmt.Println("")
		fmt.Println("# introspection-trace")
		fmt.Printf("primitive: %s\n", id)
		fmt.Printf("mode: %s\n", *mode)
		if *contextStr != "" {
			fmt.Printf("context: %s\n", *contextStr)
		}
		fmt.Printf("tier: %s\n", entry.Tier)
		fmt.Printf("status: %s\n", entry.Status)
		if output.IntrospectionTrace != "" {
			fmt.Printf("llm: %s\n", output.IntrospectionTrace)
		}
	}
}
