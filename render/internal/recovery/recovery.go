// Package recovery implements the composition-recovery /
// per-entry-discriminativity / pairwise-redundancy metric trio that
// replaced the Information Bottleneck framing in round-2 review.
//
// Inputs:
//   - a rediscovery set: named models, each with a should_fire truth
//     set of elements entries plus labeled excerpts.
//   - the loaded elements pool (via internal/loader).
//   - per-excerpt firing labels, ideally hand-coded so the metric
//     isn't downstream of an uncalibrated scorer.
//
// Outputs:
//   - composition-recovery rate per named model and aggregate.
//   - per-entry discriminativity (χ²) against each named model label.
//   - pairwise redundancy (Jaccard of firing-pattern vectors) across
//     all entries that fired anywhere.
//
// All three metrics use simple contingency-table math; no info
// theory, no MI estimation. See round-2 IB pressure-test for the
// reasoning behind this choice.
package recovery

import (
	"math"
	"sort"
)

// NamedModel is one row of the rediscovery set.
type NamedModel struct {
	ID          string
	Name        string
	Source      string
	ShouldFire  []string  // elements entry IDs that should fire to recover this model
	Excerpts    []Excerpt
}

// Excerpt is one labeled passage. Fired is the binary firing vector
// in entry-id space (typically populated by hand-coding for the v0
// experiment; LLM-coded once the surfacing function is calibrated).
type Excerpt struct {
	ID    string
	Text  string
	Fired map[string]bool // entry ID → did it fire on this excerpt
}

// Result aggregates the three metric outputs.
type Result struct {
	PerModelRecovery     []ModelRecovery
	OverallRecoveryRate  float64
	EntryDiscriminativity []EntryDiscrim
	PairwiseRedundancy   []PairRedundancy
}

// ModelRecovery is the recovery rate for one named model:
// fraction of excerpts in which the should_fire set is fully
// covered by the actual firings.
type ModelRecovery struct {
	ModelID         string
	ModelName       string
	ExcerptCount    int
	FullyRecovered  int
	PartiallyRecovered int
	NotRecovered    int
	Rate            float64 // FullyRecovered / ExcerptCount
	MeanCoverage    float64 // mean fraction of should_fire entries actually fired
}

// EntryDiscrim is one (entry, model) χ² discriminativity score.
// High χ² with high should-fire-membership = good discriminator;
// high χ² without should-fire-membership = surprising firing pattern
// worth investigating; low χ² = entry doesn't distinguish this model.
type EntryDiscrim struct {
	EntryID    string
	ModelID    string
	ChiSquared float64
	IsExpected bool // entry is in model.ShouldFire
}

// PairRedundancy is the Jaccard similarity of two entries' firing
// vectors across the full excerpt corpus. Entries with identical
// firing patterns (Jaccard = 1) are merge candidates; entries that
// never fire together (Jaccard = 0) carve different signal.
type PairRedundancy struct {
	EntryA  string
	EntryB  string
	Jaccard float64
}

// Compute runs all three metrics. excerptsAll is the concatenation
// of every model's excerpts; entriesAll is the universe of entry
// IDs to consider for discriminativity and redundancy (typically
// every entry that fired in at least one excerpt).
func Compute(models []NamedModel) Result {
	res := Result{}
	res.PerModelRecovery = perModelRecovery(models)
	res.OverallRecoveryRate = aggregateRate(res.PerModelRecovery)

	entries, excerpts := flattenExcerpts(models)
	res.EntryDiscriminativity = discriminativity(models, entries, excerpts)
	res.PairwiseRedundancy = pairwiseRedundancy(entries, excerpts)
	return res
}

func perModelRecovery(models []NamedModel) []ModelRecovery {
	out := make([]ModelRecovery, 0, len(models))
	for _, m := range models {
		mr := ModelRecovery{ModelID: m.ID, ModelName: m.Name, ExcerptCount: len(m.Excerpts)}
		if len(m.Excerpts) == 0 || len(m.ShouldFire) == 0 {
			out = append(out, mr)
			continue
		}
		shouldSet := stringSet(m.ShouldFire)
		var coverageSum float64
		for _, ex := range m.Excerpts {
			covered := 0
			for sf := range shouldSet {
				if ex.Fired[sf] {
					covered++
				}
			}
			frac := float64(covered) / float64(len(shouldSet))
			coverageSum += frac
			switch {
			case covered == len(shouldSet):
				mr.FullyRecovered++
			case covered > 0:
				mr.PartiallyRecovered++
			default:
				mr.NotRecovered++
			}
		}
		mr.Rate = float64(mr.FullyRecovered) / float64(mr.ExcerptCount)
		mr.MeanCoverage = coverageSum / float64(mr.ExcerptCount)
		out = append(out, mr)
	}
	return out
}

func aggregateRate(per []ModelRecovery) float64 {
	totalEx := 0
	totalRecov := 0
	for _, r := range per {
		totalEx += r.ExcerptCount
		totalRecov += r.FullyRecovered
	}
	if totalEx == 0 {
		return 0
	}
	return float64(totalRecov) / float64(totalEx)
}

// flattenExcerpts returns the universe of entry IDs that fired
// anywhere, plus all excerpts in a single slice tagged with their
// source model id (for χ² conditional rows).
type taggedExcerpt struct {
	ModelID string
	Excerpt
}

func flattenExcerpts(models []NamedModel) ([]string, []taggedExcerpt) {
	entrySet := map[string]bool{}
	var all []taggedExcerpt
	for _, m := range models {
		for _, ex := range m.Excerpts {
			all = append(all, taggedExcerpt{ModelID: m.ID, Excerpt: ex})
			for id := range ex.Fired {
				entrySet[id] = true
			}
		}
	}
	var entries []string
	for id := range entrySet {
		entries = append(entries, id)
	}
	sort.Strings(entries)
	return entries, all
}

// discriminativity returns one EntryDiscrim per (entry, model) pair.
// χ² is computed on a 2×2 contingency table:
//
//	                   model=this  model=other
//	entry fired           a            b
//	entry didn't fire     c            d
//
// Yates' continuity correction applied (so small N doesn't blow up).
func discriminativity(models []NamedModel, entries []string, all []taggedExcerpt) []EntryDiscrim {
	var out []EntryDiscrim
	for _, m := range models {
		shouldSet := stringSet(m.ShouldFire)
		for _, eid := range entries {
			var a, b, c, d float64
			for _, te := range all {
				fired := te.Fired[eid]
				isThis := te.ModelID == m.ID
				switch {
				case fired && isThis:
					a++
				case fired && !isThis:
					b++
				case !fired && isThis:
					c++
				case !fired && !isThis:
					d++
				}
			}
			chi := chiSquaredYates(a, b, c, d)
			out = append(out, EntryDiscrim{
				EntryID:    eid,
				ModelID:    m.ID,
				ChiSquared: chi,
				IsExpected: shouldSet[eid],
			})
		}
	}
	return out
}

func chiSquaredYates(a, b, c, d float64) float64 {
	n := a + b + c + d
	if n == 0 {
		return 0
	}
	rowA := a + b
	rowC := c + d
	colA := a + c
	colB := b + d
	if rowA == 0 || rowC == 0 || colA == 0 || colB == 0 {
		return 0
	}
	num := math.Abs(a*d-b*c) - n/2
	if num <= 0 {
		return 0
	}
	return n * num * num / (rowA * rowC * colA * colB)
}

// pairwiseRedundancy computes Jaccard similarity of firing-vectors
// across all excerpts. Returns one PairRedundancy per unordered pair;
// ordered as (A < B) lexicographically. Only emits pairs with
// Jaccard > 0 (most pairs in a sparse elements are 0; emitting them
// would dominate output noise).
func pairwiseRedundancy(entries []string, all []taggedExcerpt) []PairRedundancy {
	// Build per-entry firing vector
	fireSet := map[string]map[string]bool{}
	for _, eid := range entries {
		fireSet[eid] = map[string]bool{}
	}
	for _, te := range all {
		for eid := range te.Fired {
			if te.Fired[eid] {
				fireSet[eid][te.ID] = true
			}
		}
	}
	var out []PairRedundancy
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			a := fireSet[entries[i]]
			b := fireSet[entries[j]]
			inter := 0
			for x := range a {
				if b[x] {
					inter++
				}
			}
			union := len(a) + len(b) - inter
			if union == 0 || inter == 0 {
				continue
			}
			out = append(out, PairRedundancy{
				EntryA:  entries[i],
				EntryB:  entries[j],
				Jaccard: float64(inter) / float64(union),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Jaccard > out[j].Jaccard
	})
	return out
}

func stringSet(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}
