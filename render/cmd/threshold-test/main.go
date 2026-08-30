// threshold-test is a one-off empirical study tool for the V6 session
// (2026-05-03). Runs the gate against a curated set of representative
// prompts at multiple thresholds and prints fires/prompt at each
// threshold to support evidence-based threshold-tuning. Mirrors
// cmd_hook.go's vocab extraction and gate.Run wiring; does NOT modify
// any production code.
//
// Usage: from render/, run `go run ./cmd/threshold-test`
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/gate"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// curatedPrompts is the empirical study sample: representative of typical
// Claude Code interactions. Mix of: Polya-likely (debugging via
// work-backwards), Fermi-likely, design (frame), debugging (concrete),
// concept-explanation, vague-not-substantive. Selected to span the
// likely deployment surface, not to game any particular entry's hot-fire
// rate.
var curatedPrompts = []string{
	// Polya-likely
	"how do i work backwards from this bug to find the root cause",
	"i'm stuck on this algorithm problem can you help me think through it",
	"find related problems to this graph traversal puzzle",
	// Fermi-likely
	"generate a fermi estimate of how many lines of code we have across all our repos",
	"roughly how much memory does this data structure use at scale",
	// Concept-explanation
	"explain how bayesian updating works in plain language",
	"what does coarse-graining mean in physics simulations",
	// Design / frame
	"whats the best architecture for a microservices setup with these constraints",
	"compare react vs vue for our internal tooling use case",
	// Concrete debugging / task
	"help me debug this go test failure in the loader package",
	"what does this error message about nil pointer dereference mean",
	"refactor this function to be more readable",
	"write a sql query to join these three tables on user_id",
	"set up a ci pipeline for this repo with go test and golangci-lint",
	// Vague / not-substantive
	"hi",
	"thanks",
	"can you help",
	"yes please continue",
}

// thresholds matches the discrete cluster points observed in V5 hook.log
// analysis (0.70 / 0.84 / 1.12) plus the current default (0.50) and a
// stricter ceiling (1.00).
var thresholds = []float64{0.50, 0.70, 0.84, 1.00, 1.12}

// extractPromptVocab is a verbatim copy of the same-named function in
// cmd_hook.go (which is package main and not importable). Same length-≥3
// + stop-word filter; same regex split; same dedupe.
func extractPromptVocab(prompt string) []string {
	splitter := regexp.MustCompile(`[^a-zA-Z\-]+`)
	tokens := splitter.Split(strings.ToLower(prompt), -1)
	out := make([]string, 0, len(tokens))
	seen := map[string]bool{}
	for _, t := range tokens {
		if len(t) < 3 || hookStopWord(t) || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// MUST stay in sync with cmd_hook.go::hookStopWords. V7 expanded class-2
// generic content tokens to suppress lex-bpr6b name-token dominance.
var hookStopWords = map[string]bool{
	// class 1: English function words
	"the": true, "and": true, "for": true, "you": true, "are": true,
	"but": true, "not": true, "this": true, "that": true, "with": true,
	"have": true, "what": true, "all": true, "can": true, "use": true,
	"how": true, "from": true, "your": true, "any": true, "out": true,
	"its": true, "one": true, "more": true, "should": true, "want": true,
	"need": true, "into": true, "now": true, "they": true, "them": true,
	"who": true, "why": true, "when": true, "where": true, "which": true,
	"will": true, "would": true, "could": true, "may": true, "might": true,
	"about": true, "just": true, "like": true, "than": true, "then": true,
	// class 2: generic content tokens (V7 expansion)
	"work": true, "related": true, "problem": true, "method": true,
	"pattern": true, "anchor": true, "strategy": true, "plan": true,
	"via": true, "find": true, "make": true, "way": true, "thing": true,
	"here": true, "there": true, "also": true, "well": true, "very": true,
	"some": true, "such": true, "only": true, "much": true, "many": true,
	"each": true, "every": true, "other": true, "another": true,
}

func hookStopWord(t string) bool { return hookStopWords[t] }

func main() {
	cwd, _ := os.Getwd()
	// Try repo-root layout (elements/ as sibling of render/), then
	// the render/ cwd layout (one level above).
	elementsDir := filepath.Join(cwd, "elements")
	if _, err := os.Stat(elementsDir); err != nil {
		elementsDir = filepath.Join(cwd, "..", "elements")
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loader: %v\n", err)
		os.Exit(1)
	}
	entries := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		entries = append(entries, e)
	}

	fmt.Printf("# threshold-tuning empirical study\n\n")
	fmt.Printf("Prompts: %d  ·  elements: %d entries  ·  thresholds: %v\n\n",
		len(curatedPrompts), len(entries), thresholds)

	// Per-prompt: collect ALL gate results above 0 (no threshold filter)
	// then bucket by which thresholds they would clear.
	type promptOutcome struct {
		prompt     string
		topK       []types.GateResult
		vocabSize  int
	}
	outcomes := make([]promptOutcome, 0, len(curatedPrompts))
	for _, p := range curatedPrompts {
		vocab := extractPromptVocab(p)
		results := gate.Run(gate.Input{
			Pool:         entries,
			Context:      p,
			WorkingVocab: vocab,
			TopK:         3,
		})
		outcomes = append(outcomes, promptOutcome{p, results, len(vocab)})
	}

	// === Section 1: per-prompt top-3 ===
	fmt.Printf("## Per-prompt top-3 results (threshold-blind)\n\n")
	for _, o := range outcomes {
		fmt.Printf("### `%s`\n", truncate(o.prompt, 80))
		fmt.Printf("vocab tokens: %d\n", o.vocabSize)
		if len(o.topK) == 0 {
			fmt.Printf("  (no entries scored)\n\n")
			continue
		}
		for _, r := range o.topK {
			entry := pool[r.PrimitiveID]
			name := r.PrimitiveID
			if entry != nil {
				name = entry.Name
			}
			fmt.Printf("  %.2f  %s  (%s)\n", r.Score, r.PrimitiveID, name)
		}
		fmt.Println()
	}

	// === Section 2: trade-off curve ===
	fmt.Printf("## Trade-off curve: fires per prompt at each threshold\n\n")
	fmt.Printf("threshold | prompts-with-≥1-fire | total-fires | unique-entries | silent-prompts\n")
	fmt.Printf("---|---|---|---|---\n")
	for _, t := range thresholds {
		promptsWithFire := 0
		totalFires := 0
		entrySet := map[string]bool{}
		for _, o := range outcomes {
			fired := false
			for _, r := range o.topK {
				if r.Score >= t {
					totalFires++
					entrySet[r.PrimitiveID] = true
					fired = true
				}
			}
			if fired {
				promptsWithFire++
			}
		}
		silent := len(outcomes) - promptsWithFire
		fmt.Printf("%.2f | %d/%d (%.0f%%) | %d | %d | %d\n",
			t, promptsWithFire, len(outcomes),
			100*float64(promptsWithFire)/float64(len(outcomes)),
			totalFires, len(entrySet), silent)
	}
	fmt.Println()

	// === Section 3: per-entry fire frequency at threshold 0.50 ===
	fmt.Printf("## Per-entry fire frequency at threshold 0.50 (current default)\n\n")
	freq := map[string]int{}
	for _, o := range outcomes {
		for _, r := range o.topK {
			if r.Score >= 0.50 {
				freq[r.PrimitiveID]++
			}
		}
	}
	type kv struct {
		id    string
		count int
	}
	sorted := make([]kv, 0, len(freq))
	for k, v := range freq {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].id < sorted[j].id
	})
	for _, e := range sorted {
		entry := pool[e.id]
		name := e.id
		if entry != nil {
			name = entry.Name
		}
		fmt.Printf("  %d/%d (%.0f%%)  %s  (%s)\n",
			e.count, len(outcomes),
			100*float64(e.count)/float64(len(outcomes)),
			e.id, name)
	}
	fmt.Println()

	// === Section 4: prompts that go silent at higher thresholds ===
	fmt.Printf("## Prompts that go SILENT (no fires) at each threshold\n\n")
	for _, t := range thresholds {
		fmt.Printf("### threshold %.2f\n", t)
		anySilent := false
		for _, o := range outcomes {
			fired := false
			for _, r := range o.topK {
				if r.Score >= t {
					fired = true
					break
				}
			}
			if !fired {
				fmt.Printf("  · `%s`\n", truncate(o.prompt, 70))
				anySilent = true
			}
		}
		if !anySilent {
			fmt.Printf("  (none — every prompt has at least one match)\n")
		}
		fmt.Println()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
