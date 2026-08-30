package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/extrapolate"
	"github.com/justinstimatze/lexicon/render/internal/loader"
)

// cmdExtrapolate: given a constellation of atom IDs, emit the adjacency
// frontier — atoms pointed at by N of the constellation but not present
// in it, ranked by N. Pure elements-graph operation; no LLM, no
// embedding pass; output is JSON by default for agent consumption.
//
// Usage:
//
//	lexicon extrapolate lex-0001 lex-75r77 lex-4yhqs
//	echo "lex-0001 lex-75r77" | lexicon extrapolate
//	lexicon extrapolate --text lex-0001 lex-75r77
//	lexicon extrapolate --top-k 5 lex-0001 lex-75r77
//
// Motivated by sluice's forecasting back-test (V114): feeding a
// constellation of fired atoms back to lexicon_read as prose is one
// way to read the adjacency frontier; this is the deterministic,
// model-confound-free baseline.
func cmdExtrapolate(renderDir string, args []string) {
	fl := flag.NewFlagSet("extrapolate", flag.ExitOnError)
	asText := fl.Bool("text", false, "human-readable text output (default: JSON)")
	topK := fl.Int("top-k", 0, "limit candidates returned (0 = all)")

	// Interleave flag-parsing with positional collection so that flag
	// order is not constrained ("lex-0001 --top-k 5" works the same as
	// "--top-k 5 lex-0001"). Go's flag.Parse stops at the first non-
	// flag, so we loop and consume one positional at a time.
	var positional []string
	remaining := args
	for len(remaining) > 0 {
		_ = fl.Parse(remaining)
		rest := fl.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		remaining = rest[1:]
	}
	ids := parseConstellation(positional)
	if len(ids) == 0 {
		data, _ := io.ReadAll(os.Stdin)
		ids = parseConstellation(strings.Fields(string(data)))
	}
	if len(ids) == 0 {
		fatal("extrapolate: no atom IDs provided (pass as args or stdin)")
	}

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, "..", "elements")
		if _, err := os.Stat(elementsDir); err != nil {
			elementsDir = filepath.Join(renderDir, "elements")
		}
	}
	atoms, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("extrapolate: load elements: %v", err)
	}
	result := extrapolate.Frontier(ids, atoms)
	if *topK > 0 && len(result.Candidates) > *topK {
		result.Candidates = result.Candidates[:*topK]
	}

	if *asText {
		printExtrapolateText(os.Stdout, result)
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fatal("extrapolate: encode: %v", err)
	}
}

// parseConstellation accepts a slice of tokens (CLI args or stdin
// fields) and splits each on whitespace + commas, dedups while
// preserving order. Returns empty slice if no IDs found.
func parseConstellation(tokens []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, tok := range tokens {
		for _, id := range strings.FieldsFunc(tok, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}) {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

func printExtrapolateText(w io.Writer, r extrapolate.Result) {
	fmt.Fprintf(w, "constellation: %d atoms\n", len(r.Constellation))
	if len(r.Missing) > 0 {
		fmt.Fprintf(w, "missing (not in elements): %s\n", strings.Join(r.Missing, ", "))
	}
	fmt.Fprintf(w, "adjacency frontier: %d candidates\n\n", len(r.Candidates))
	for _, c := range r.Candidates {
		name := c.Name
		if name == "" {
			name = "(unknown — not in elements)"
		}
		fmt.Fprintf(w, "%s  ×%d  %s\n", c.ID, c.AdjacencyCount, name)
		fmt.Fprintf(w, "  pointed-at-by: %s\n", strings.Join(c.PointedAtBy, ", "))
		if c.Tier != "" || c.Status != "" {
			fmt.Fprintf(w, "  tier=%s status=%s\n", c.Tier, c.Status)
		}
		fmt.Fprintln(w)
	}
}
