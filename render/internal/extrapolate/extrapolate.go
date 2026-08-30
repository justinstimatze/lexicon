// Package extrapolate implements the adjacency-frontier read on a
// constellation of atom IDs: given N input atoms, returns atoms NOT in
// the input set that are pointed at by one or more of them, ranked by
// adjacency-count.
//
// This is the elements-graph primitive sluice was hand-composing
// through lexicon_read's prose-through-lens path. Pure walk of the
// `related` field — deterministic, no LLM call, no embedding pass, no
// model confound. The semantic-lens variant remains available via
// lexicon read on a constellation-description prose input; this
// package is the structural baseline that variant should be compared
// against.
//
// Operational read: a constellation says "this gestalt is firing"; the
// frontier names the atoms that gestalt invokes-but-doesn't-contain,
// i.e. the ontological negative space of the chosen frame. See the
// V114 sluice forecasting-back-test rundown for the back-test that
// motivated this primitive.
package extrapolate

import (
	"sort"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Candidate is one atom on the adjacency frontier of a constellation.
type Candidate struct {
	ID             string   `json:"id"`
	Name           string   `json:"name,omitempty"`
	Tier           string   `json:"tier,omitempty"`
	Status         string   `json:"status,omitempty"`
	AdjacencyCount int      `json:"adjacency_count"`
	PointedAtBy    []string `json:"pointed_at_by"`
}

// Result wraps the frontier with provenance about the input.
type Result struct {
	Constellation []string    `json:"constellation"`
	Missing       []string    `json:"missing,omitempty"`
	Candidates    []Candidate `json:"candidates"`
}

// Frontier computes the adjacency frontier of constellation against
// atoms. Sort order: AdjacencyCount desc, then ID asc for stability.
// Missing IDs (in constellation but not in atoms) are surfaced
// separately rather than silently dropped — callers may want to know
// they passed a typo or a not-yet-minted candidate.
func Frontier(constellation []string, atoms map[string]*types.LexEntry) Result {
	constSet := make(map[string]struct{}, len(constellation))
	for _, id := range constellation {
		constSet[id] = struct{}{}
	}
	var missing []string
	pointers := map[string][]string{}
	for _, id := range constellation {
		e, ok := atoms[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		for _, rel := range e.Related {
			if _, inInput := constSet[rel]; inInput {
				continue
			}
			pointers[rel] = append(pointers[rel], id)
		}
	}
	cands := make([]Candidate, 0, len(pointers))
	for relID, srcs := range pointers {
		c := Candidate{
			ID:             relID,
			AdjacencyCount: len(srcs),
			PointedAtBy:    srcs,
		}
		if e, ok := atoms[relID]; ok {
			c.Name = e.Name
			c.Tier = e.Tier
			c.Status = e.Status
		}
		// keep PointedAtBy sorted for deterministic JSON output
		sort.Strings(c.PointedAtBy)
		cands = append(cands, c)
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].AdjacencyCount != cands[j].AdjacencyCount {
			return cands[i].AdjacencyCount > cands[j].AdjacencyCount
		}
		return cands[i].ID < cands[j].ID
	})
	return Result{
		Constellation: constellation,
		Missing:       missing,
		Candidates:    cands,
	}
}
