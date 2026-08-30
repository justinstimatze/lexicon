package main

// `lexicon recovery <set.yaml>` — run the composition-recovery /
// per-entry-discriminativity / pairwise-redundancy metric trio
// against a rediscovery set on disk.
//
// The rediscovery-set YAML format is in
// elements-recovery/rediscovery-set.yaml. Excerpts
// carry a `fired:` map keyed by entry id; populate by hand for
// the v0 experiment.

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/justinstimatze/lexicon/render/internal/recovery"
)

func cmdRecovery(renderDir string, args []string) {
	fl := flag.NewFlagSet("recovery", flag.ExitOnError)
	topPairs := fl.Int("top-pairs", 10, "max number of redundancy pairs to print")
	chiThreshold := fl.Float64("chi-threshold", 1.0, "discriminativity χ² threshold to include in output")
	fl.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lexicon recovery <rediscovery-set.yaml>")
		fmt.Fprintln(os.Stderr, "  Compute composition-recovery rate, per-entry discriminativity (χ²),")
		fmt.Fprintln(os.Stderr, "  and pairwise firing-pattern redundancy from a labeled rediscovery set.")
		fl.PrintDefaults()
	}
	_ = fl.Parse(args)

	if fl.NArg() != 1 {
		fl.Usage()
		os.Exit(2)
	}
	models, err := loadRediscoverySet(fl.Arg(0))
	if err != nil {
		fatal("load rediscovery set: %v", err)
	}
	if len(models) == 0 {
		fatal("rediscovery set is empty")
	}

	res := recovery.Compute(models)
	emitRecoveryReport(res, *topPairs, *chiThreshold)
}

type rediscoveryFile struct {
	NamedModels []rediscoveryModel `yaml:"named_models"`
}

type rediscoveryModel struct {
	ID         string                `yaml:"id"`
	Name       string                `yaml:"name"`
	Source     string                `yaml:"source"`
	ShouldFire []string              `yaml:"should_fire"`
	Excerpts   []rediscoveryExcerpt  `yaml:"excerpts"`
}

type rediscoveryExcerpt struct {
	ID                 string          `yaml:"id"`
	Text               string          `yaml:"text"`
	Fired              map[string]bool `yaml:"fired"`
	ShouldFireExtra    []string        `yaml:"should_fire_in_addition"`
}

func loadRediscoverySet(path string) ([]recovery.NamedModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f rediscoveryFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	out := make([]recovery.NamedModel, 0, len(f.NamedModels))
	for _, m := range f.NamedModels {
		// Skip TODO templates: rows where ShouldFire is empty or contains "TODO".
		if isTodoModel(m) {
			continue
		}
		nm := recovery.NamedModel{
			ID:         m.ID,
			Name:       m.Name,
			Source:     m.Source,
			ShouldFire: m.ShouldFire,
		}
		for _, e := range m.Excerpts {
			fired := e.Fired
			if fired == nil {
				fired = map[string]bool{}
			}
			nm.Excerpts = append(nm.Excerpts, recovery.Excerpt{
				ID:    e.ID,
				Text:  e.Text,
				Fired: fired,
			})
		}
		out = append(out, nm)
	}
	return out, nil
}

func isTodoModel(m rediscoveryModel) bool {
	if m.Name == "" || m.ID == "" {
		return true
	}
	if len(m.ShouldFire) == 0 {
		return true
	}
	for _, sf := range m.ShouldFire {
		if sf == "TODO" || sf == "" {
			return true
		}
	}
	return false
}

func emitRecoveryReport(res recovery.Result, topPairs int, chiThreshold float64) {
	fmt.Println("# composition-recovery report")
	fmt.Println()
	fmt.Println("## Per-model recovery")
	fmt.Println()
	fmt.Println("| model | excerpts | full | partial | none | rate | mean coverage |")
	fmt.Println("|---|---|---|---|---|---|---|")
	for _, mr := range res.PerModelRecovery {
		fmt.Printf("| %s (%s) | %d | %d | %d | %d | %.2f | %.2f |\n",
			mr.ModelName, mr.ModelID, mr.ExcerptCount,
			mr.FullyRecovered, mr.PartiallyRecovered, mr.NotRecovered,
			mr.Rate, mr.MeanCoverage)
	}
	fmt.Printf("\n**Aggregate full-recovery rate: %.2f**\n", res.OverallRecoveryRate)

	fmt.Println()
	fmt.Println("## Per-entry discriminativity (χ², Yates corrected)")
	fmt.Println()
	fmt.Println("Above threshold; **flagged** = entry not in model's should_fire (surprising co-fire).")
	fmt.Println()
	fmt.Println("| entry | model | χ² | expected? |")
	fmt.Println("|---|---|---|---|")
	// Sort by χ² descending
	disc := make([]recovery.EntryDiscrim, len(res.EntryDiscriminativity))
	copy(disc, res.EntryDiscriminativity)
	sort.Slice(disc, func(i, j int) bool { return disc[i].ChiSquared > disc[j].ChiSquared })
	printed := 0
	for _, d := range disc {
		if d.ChiSquared < chiThreshold {
			continue
		}
		mark := "expected"
		if !d.IsExpected {
			mark = "**surprising**"
		}
		fmt.Printf("| %s | %s | %.2f | %s |\n", d.EntryID, d.ModelID, d.ChiSquared, mark)
		printed++
		if printed >= 50 {
			fmt.Println("| ... | | | (truncated) |")
			break
		}
	}

	fmt.Println()
	fmt.Println("## Pairwise redundancy (top entries by Jaccard of firing-patterns)")
	fmt.Println()
	fmt.Println("Pairs where Jaccard ≥ 0.7 are merge/dedup candidates; Jaccard < 0.3 means the")
	fmt.Println("entries carve different signal.")
	fmt.Println()
	fmt.Println("| entry A | entry B | jaccard |")
	fmt.Println("|---|---|---|")
	for i, p := range res.PairwiseRedundancy {
		if i >= topPairs {
			fmt.Println("| ... | | (truncated; raise --top-pairs) |")
			break
		}
		fmt.Printf("| %s | %s | %.3f |\n", p.EntryA, p.EntryB, p.Jaccard)
	}
	fmt.Println()
}
