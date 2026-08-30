// benchmark runs the curated benchmark in benchmark-prompts.yaml against
// the production gate (current threshold + stopword config from
// cmd_hook.go) and reports precision / recall / F1 per category and
// aggregate.
//
// Purpose: replace V6's 18-prompt threshold-test with a labeled
// benchmark that supports rigorous gate evaluation. Pre-registered
// metrics (per V8 surfacing in lex-bpr6b-rename-analysis-pass-1.md):
//
//   - precision = |fires ∩ expected_fires| / |fires|
//   - recall    = |fires ∩ expected_fires| / |expected_fires|
//   - F1        = 2PR / (P+R)
//   - silent_success_rate = % of expected_silent prompts with 0 fires
//
// Vocab extraction and stopword list MUST stay in sync with
// cmd_hook.go::extractPromptVocab + hookStopWords. Any future stopword
// edit to the production hook should mirror here.
//
// Usage: from render/, run `go run ./cmd/benchmark`
//        --threshold 0.84    (override threshold; default = production)
//        --top-k 3           (override top-k; default = production)
//        --category polya    (filter to one category)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/gate"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
	"gopkg.in/yaml.v3"
)

const (
	defaultThreshold = 0.84
	defaultTopK      = 3
)

type benchmarkPrompt struct {
	Text           string   `yaml:"text"`
	Category       string   `yaml:"category"`
	ExpectedFires  []string `yaml:"expected_fires"`
	ExpectedSilent bool     `yaml:"expected_silent"`
	Notes          string   `yaml:"notes,omitempty"`
}

type benchmarkFile struct {
	Version int               `yaml:"version"`
	Created string            `yaml:"created"`
	Prompts []benchmarkPrompt `yaml:"prompts"`
}

type promptResult struct {
	prompt          benchmarkPrompt
	vocab           []string
	gateResults     []types.GateResult
	firesAtThresh   []string
	hits            []string
	missesExpected  []string
	spuriousFires   []string
}

// MUST stay in sync with cmd_hook.go::hookStopWords (V7 expanded list).
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

func extractPromptVocab(prompt string) []string {
	splitter := regexp.MustCompile(`[^a-zA-Z\-]+`)
	tokens := splitter.Split(strings.ToLower(prompt), -1)
	out := make([]string, 0, len(tokens))
	seen := map[string]bool{}
	for _, t := range tokens {
		if len(t) < 3 || hookStopWords[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func main() {
	threshold := flag.Float64("threshold", defaultThreshold, "score threshold for fire")
	topK := flag.Int("top-k", defaultTopK, "top-k results to consider")
	categoryFilter := flag.String("category", "", "filter to one category (empty = all)")
	verbose := flag.Bool("verbose", false, "print per-prompt detail")
	flag.Parse()

	cwd, _ := os.Getwd()
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

	// Load benchmark
	benchPath := filepath.Join(cwd, "cmd", "benchmark", "benchmark-prompts.yaml")
	if _, err := os.Stat(benchPath); err != nil {
		benchPath = filepath.Join(cwd, "render", "cmd", "benchmark", "benchmark-prompts.yaml")
	}
	data, err := os.ReadFile(benchPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read benchmark: %v\n", err)
		os.Exit(1)
	}
	var bench benchmarkFile
	if err := yaml.Unmarshal(data, &bench); err != nil {
		fmt.Fprintf(os.Stderr, "parse benchmark: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("# benchmark v%d (%s)\n", bench.Version, bench.Created)
	fmt.Printf("# elements: %d entries · prompts: %d · threshold: %.2f · top-k: %d\n\n",
		len(entries), len(bench.Prompts), *threshold, *topK)

	// Run each prompt
	results := make([]promptResult, 0, len(bench.Prompts))
	for _, p := range bench.Prompts {
		if *categoryFilter != "" && p.Category != *categoryFilter {
			continue
		}
		vocab := extractPromptVocab(p.Text)
		gr := gate.Run(gate.Input{
			Pool:         entries,
			Context:      p.Text,
			WorkingVocab: vocab,
			TopK:         *topK,
		})
		fires := []string{}
		for _, r := range gr {
			if r.Score >= *threshold {
				fires = append(fires, r.PrimitiveID)
			}
		}
		expectedSet := map[string]bool{}
		for _, e := range p.ExpectedFires {
			expectedSet[e] = true
		}
		hits := []string{}
		spurious := []string{}
		for _, f := range fires {
			if expectedSet[f] {
				hits = append(hits, f)
			} else if !p.ExpectedSilent {
				spurious = append(spurious, f)
			} else {
				spurious = append(spurious, f)
			}
		}
		fireSet := map[string]bool{}
		for _, f := range fires {
			fireSet[f] = true
		}
		misses := []string{}
		for _, e := range p.ExpectedFires {
			if !fireSet[e] {
				misses = append(misses, e)
			}
		}
		results = append(results, promptResult{
			prompt:         p,
			vocab:          vocab,
			gateResults:    gr,
			firesAtThresh:  fires,
			hits:           hits,
			missesExpected: misses,
			spuriousFires:  spurious,
		})
	}

	if *verbose {
		printVerbose(results, pool)
	}

	// Aggregate by category
	type catStats struct {
		category   string
		count      int
		// for non-silent prompts:
		nonSilent     int
		totalFires    int
		totalHits     int
		totalExpected int
		// for silent prompts:
		silentCount     int
		silentSuccesses int // 0 fires
	}
	catMap := map[string]*catStats{}
	order := []string{}
	for _, r := range results {
		c, ok := catMap[r.prompt.Category]
		if !ok {
			c = &catStats{category: r.prompt.Category}
			catMap[r.prompt.Category] = c
			order = append(order, r.prompt.Category)
		}
		c.count++
		if r.prompt.ExpectedSilent {
			c.silentCount++
			if len(r.firesAtThresh) == 0 {
				c.silentSuccesses++
			}
		} else {
			c.nonSilent++
			c.totalFires += len(r.firesAtThresh)
			c.totalHits += len(r.hits)
			c.totalExpected += len(r.prompt.ExpectedFires)
		}
	}

	fmt.Printf("## Per-category metrics\n\n")
	fmt.Printf("category | n | non-silent | fires | hits | expected | precision | recall | F1 | silent | silent-success\n")
	fmt.Printf("---|---|---|---|---|---|---|---|---|---|---\n")

	totalNonSilent := 0
	totalFires := 0
	totalHits := 0
	totalExpected := 0
	totalSilent := 0
	totalSilentSuccess := 0

	for _, k := range order {
		c := catMap[k]
		precision := 0.0
		if c.totalFires > 0 {
			precision = float64(c.totalHits) / float64(c.totalFires)
		}
		recall := 0.0
		if c.totalExpected > 0 {
			recall = float64(c.totalHits) / float64(c.totalExpected)
		}
		f1 := 0.0
		if precision+recall > 0 {
			f1 = 2 * precision * recall / (precision + recall)
		}
		silentSucc := 0.0
		if c.silentCount > 0 {
			silentSucc = float64(c.silentSuccesses) / float64(c.silentCount)
		}
		fmt.Printf("%s | %d | %d | %d | %d | %d | %.2f | %.2f | %.2f | %d | %.0f%%\n",
			c.category, c.count, c.nonSilent, c.totalFires, c.totalHits, c.totalExpected,
			precision, recall, f1, c.silentCount, silentSucc*100)
		totalNonSilent += c.nonSilent
		totalFires += c.totalFires
		totalHits += c.totalHits
		totalExpected += c.totalExpected
		totalSilent += c.silentCount
		totalSilentSuccess += c.silentSuccesses
	}

	// Aggregate
	aggP := 0.0
	if totalFires > 0 {
		aggP = float64(totalHits) / float64(totalFires)
	}
	aggR := 0.0
	if totalExpected > 0 {
		aggR = float64(totalHits) / float64(totalExpected)
	}
	aggF1 := 0.0
	if aggP+aggR > 0 {
		aggF1 = 2 * aggP * aggR / (aggP + aggR)
	}
	silentRate := 0.0
	if totalSilent > 0 {
		silentRate = float64(totalSilentSuccess) / float64(totalSilent)
	}
	fmt.Printf("---|---|---|---|---|---|---|---|---|---|---\n")
	fmt.Printf("**TOTAL** | %d | %d | %d | %d | %d | **%.2f** | **%.2f** | **%.2f** | %d | **%.0f%%**\n",
		len(results), totalNonSilent, totalFires, totalHits, totalExpected,
		aggP, aggR, aggF1, totalSilent, silentRate*100)

	// Per-entry fire frequency across whole benchmark
	fmt.Printf("\n## Per-entry fire frequency (top 20)\n\n")
	freq := map[string]int{}
	for _, r := range results {
		for _, f := range r.firesAtThresh {
			freq[f]++
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
	cap := 20
	if len(sorted) < cap {
		cap = len(sorted)
	}
	for _, e := range sorted[:cap] {
		entry := pool[e.id]
		name := e.id
		if entry != nil {
			name = entry.Name
		}
		fmt.Printf("  %2d/%d (%2.0f%%)  %s  (%s)\n",
			e.count, len(results),
			100*float64(e.count)/float64(len(results)),
			e.id, name)
	}

	// Per-category miss list (non-silent prompts that fired NOTHING expected)
	fmt.Printf("\n## Non-silent prompts where 0 expected entries fired\n\n")
	for _, r := range results {
		if r.prompt.ExpectedSilent {
			continue
		}
		if len(r.hits) > 0 {
			continue
		}
		gotIDs := strings.Join(r.firesAtThresh, ",")
		if gotIDs == "" {
			gotIDs = "(silent)"
		}
		fmt.Printf("  · [%s] `%s`\n    expected one of: %v · got: %s\n",
			r.prompt.Category, truncate(r.prompt.Text, 80),
			r.prompt.ExpectedFires, gotIDs)
	}

	// Silent prompts that had spurious fires
	fmt.Printf("\n## Silent prompts that had spurious fires\n\n")
	for _, r := range results {
		if !r.prompt.ExpectedSilent {
			continue
		}
		if len(r.firesAtThresh) == 0 {
			continue
		}
		fmt.Printf("  · [%s] `%s` → %v\n",
			r.prompt.Category, truncate(r.prompt.Text, 80),
			r.firesAtThresh)
	}
}

func printVerbose(results []promptResult, pool map[string]*types.LexEntry) {
	fmt.Printf("## Per-prompt detail\n\n")
	for _, r := range results {
		fmt.Printf("### [%s] `%s`\n", r.prompt.Category, r.prompt.Text)
		fmt.Printf("vocab: %v · expected_fires: %v · expected_silent: %v\n",
			r.vocab, r.prompt.ExpectedFires, r.prompt.ExpectedSilent)
		for _, gr := range r.gateResults {
			marker := " "
			contains := func(xs []string, x string) bool {
				for _, s := range xs {
					if s == x {
						return true
					}
				}
				return false
			}
			if contains(r.firesAtThresh, gr.PrimitiveID) {
				marker = "*"
				if contains(r.hits, gr.PrimitiveID) {
					marker = "+"
				}
			}
			name := gr.PrimitiveID
			if e, ok := pool[gr.PrimitiveID]; ok {
				name = e.Name
			}
			fmt.Printf("  %s %.2f  %s  (%s)\n", marker, gr.Score, gr.PrimitiveID, name)
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
