package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdPartitions: cross-domain firing aggregator. Run pattern-id
// independently over N passages (one per domain/partition) and surface the
// atoms that fired across at least --min-k of them. A pattern firing in
// isolation on one domain is often just that domain's local vocabulary; a
// pattern firing across several unrelated domains is more likely a real
// slow-variable or scale-separated primitive rather than a domain-specific
// coincidence. ROADMAP.md's "Elements-graph navigation" / cross-domain
// firing aggregator item.
//
// Usage:
//
//	lexicon partitions domain1.txt domain2.txt domain3.txt
//	lexicon partitions --min-k 3 --top-k 5 a.txt b.txt c.txt d.txt
func cmdPartitions(renderDir string, args []string) {
	fl := flag.NewFlagSet("partitions", flag.ExitOnError)
	topK := fl.Int("top-k", 3, "patterns surfaced per partition before cross-firing filter")
	minK := fl.Int("min-k", 2, "minimum number of partitions an atom must fire in to be surfaced")
	noLens := fl.Bool("no-lens", false, "skip the LLM-backed semantic lens (lexical-only on full pool)")

	// Interleave flag-parsing with positional collection so flag order isn't
	// constrained relative to the file list (matches cmd_extrapolate.go's
	// convention — Go's flag.Parse stops at the first non-flag argument).
	var files []string
	remaining := args
	for len(remaining) > 0 {
		_ = fl.Parse(remaining)
		rest := fl.Args()
		if len(rest) == 0 {
			break
		}
		files = append(files, rest[0])
		remaining = rest[1:]
	}
	if len(files) < 2 {
		fatal("partitions: need at least 2 passage files, one per domain (got %d)", len(files))
	}
	if *minK < 2 {
		fatal("partitions: --min-k must be >= 2 (an atom firing in 1 partition isn't cross-domain)")
	}
	if *minK > len(files) {
		fatal("partitions: --min-k (%d) exceeds the number of partitions (%d)", *minK, len(files))
	}

	corp := loadCorpusOrFatal(renderDir)
	pool := corp.Pool()

	type partitionHit struct {
		label string
		score float64
	}
	firedIn := map[string][]partitionHit{} // atom ID -> partitions it fired in
	labels := make([]string, 0, len(files))

	for _, path := range files {
		label := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		labels = append(labels, label)
		data, err := os.ReadFile(path)
		if err != nil {
			fatal("partitions: read %s: %v", path, err)
		}
		contextStr := strings.TrimSpace(string(data))
		if contextStr == "" {
			fatal("partitions: %s is empty", path)
		}
		picked, scores, _, _, _ := corp.ScoreRaw(context.Background(), contextStr, *topK, *noLens)
		for _, e := range picked {
			firedIn[e.ID] = append(firedIn[e.ID], partitionHit{label: label, score: scores[e.ID]})
		}
	}

	type crossFired struct {
		ID                string             `json:"id"`
		Name              string             `json:"name"`
		Tier              string             `json:"tier"`
		FiredIn           []string           `json:"fired_in"`
		Scores            map[string]float64 `json:"scores"`
		Gloss             string             `json:"gloss,omitempty"`
		AgentInstruction  string             `json:"agent_instruction,omitempty"`
		CriticalQuestions []string           `json:"critical_questions,omitempty"`
	}
	const maxCQs = 3

	var results []crossFired
	for id, hits := range firedIn {
		if len(hits) < *minK {
			continue
		}
		e := pool[id]
		if e == nil {
			continue
		}
		tier := e.Tier
		if tier == "" {
			tier = "atomic"
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].label < hits[j].label })
		firedLabels := make([]string, 0, len(hits))
		scores := make(map[string]float64, len(hits))
		for _, h := range hits {
			firedLabels = append(firedLabels, h.label)
			scores[h.label] = h.score
		}
		cf := crossFired{
			ID:               e.ID,
			Name:             e.Name,
			Tier:             tier,
			FiredIn:          firedLabels,
			Scores:           scores,
			Gloss:            patternGloss(e),
			AgentInstruction: e.AgentInstruction,
		}
		if len(e.CriticalQuestions) > 0 {
			limit := maxCQs
			if len(e.CriticalQuestions) < limit {
				limit = len(e.CriticalQuestions)
			}
			cf.CriticalQuestions = e.CriticalQuestions[:limit]
		}
		results = append(results, cf)
	}
	sort.Slice(results, func(i, j int) bool {
		if len(results[i].FiredIn) != len(results[j].FiredIn) {
			return len(results[i].FiredIn) > len(results[j].FiredIn)
		}
		return results[i].ID < results[j].ID
	})

	type out struct {
		Partitions []string     `json:"partitions"`
		MinK       int          `json:"min_k"`
		TopK       int          `json:"top_k"`
		CrossFired []crossFired `json:"cross_fired"`
	}
	doc := out{Partitions: labels, MinK: *minK, TopK: *topK, CrossFired: results}
	if results == nil {
		doc.CrossFired = []crossFired{}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		fatal("partitions: encode: %v", err)
	}
}
