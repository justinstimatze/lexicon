package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// cmdRead is a thin paste-into-shell alias for `what-if --mode pattern-id
// --explain`. The intended workflow: copy article / doc / transcript
// content, then `lexicon read <file>` or `pbpaste | lexicon read -` or
// `lexicon read` (defaults to stdin). Output is the plain-language
// pattern-id synthesis. Supersedes the browser-plugin sketch from
// project_browser_plugin_news_decomposition — paste-into-shell is
// source-agnostic and skips the extension/HTTP/permissions tax.
func cmdRead(renderDir string, args []string) {
	fl := flag.NewFlagSet("read", flag.ExitOnError)
	topK := fl.Int("top-k", 3, "patterns surfaced")
	noLens := fl.Bool("no-lens", false, "skip the LLM-backed semantic lens (lexical-only on full pool)")
	noExplain := fl.Bool("no-explain", false, "skip the plain-language translator (print structured markdown instead). DEPRECATED: prefer --format markdown")
	format := fl.String("format", "json", "output format: json (default, agent-consumable), markdown (structured), plain (markdown + LLM translator)")
	_ = fl.Parse(args)

	rest := fl.Args()
	var src io.Reader = os.Stdin
	srcName := "stdin"
	if len(rest) > 0 && rest[0] != "-" {
		f, err := os.Open(rest[0])
		if err != nil {
			fatal("open %s: %v", rest[0], err)
		}
		defer f.Close()
		src = f
		srcName = rest[0]
	}
	data, err := io.ReadAll(src)
	if err != nil {
		fatal("read %s: %v", srcName, err)
	}
	contextStr := strings.TrimSpace(string(data))
	if contextStr == "" {
		fatal("read: empty input (from %s)", srcName)
	}

	if os.Getenv("LEXICON_LENS_TIMEOUT_MS") == "" {
		_ = os.Setenv("LEXICON_LENS_TIMEOUT_MS", "30000")
	}

	// Legacy --no-explain forces markdown; explicit --format wins otherwise.
	if *noExplain && *format == "json" {
		*format = "markdown"
	}
	// "plain" historically means "markdown + LLM translator"; everything else
	// skips the translator.
	explain := *format == "plain"

	if explain {
		fmt.Fprintf(os.Stderr, "read: %d chars from %s; lens=%v format=%s\n",
			len(contextStr), srcName, !*noLens, *format)
	}

	runPatternID(renderDir, contextStr, *topK, *noLens, explain, *format)
}
