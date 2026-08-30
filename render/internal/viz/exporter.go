package viz

import (
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Graph is the on-disk shape emitted to the HTML template. Nodes carry
// elements metadata; edges carry their type so the UI can style
// related-edges vs decomposes-into-edges differently.
type Graph struct {
	Nodes    []Node                         `json:"nodes"`
	Edges    []Edge                         `json:"edges"`
	Clusters []ClusterMeta                  `json:"clusters"`
	Layouts  map[string]map[string]Position `json:"layouts,omitempty"`
}

// Node is one elements entry in the graph. InDegree is precomputed
// here so the HTML/JS layer doesn't need to recount; size of the node
// in the rendered graph scales with InDegree.
type Node struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	TypeIn             string    `json:"type_in"`
	TypeOut            string    `json:"type_out"`
	Tier               string    `json:"tier"`
	Status             string    `json:"status"`
	Cluster            string    `json:"cluster"`
	Evokes             []string  `json:"evokes,omitempty"`
	CanonicalInstances []string  `json:"canonical_instances,omitempty"`
	// AgentInstruction, CriticalQuestions, and Lineage are text-heavy,
	// atom-detail-only fields — same trimmed-by-default treatment as
	// CanonicalInstances (see cmd_export_graph.go's -full flag).
	AgentInstruction  string    `json:"agent_instruction,omitempty"`
	CriticalQuestions []string  `json:"critical_questions,omitempty"`
	Lineage           []Lineage `json:"lineage,omitempty"`
	InDegree         int       `json:"in_degree"`
	IsMolecule       bool      `json:"is_molecule"`
}

// Lineage is one provenance citation, trimmed to what a reader (human
// or agent) actually wants to see: the claim's quality tier, the
// citation, and the verbatim quote it's grounded in. Mirrors
// types.LineageEntry minus Text (an internal ref-file slug, not
// reader-facing) — see toLineage.
type Lineage struct {
	Source    string `json:"source"`
	Tradition string `json:"tradition,omitempty"`
	Citation  string `json:"citation,omitempty"`
	Quote     string `json:"quote,omitempty"`
}

// Edge is one directed relationship between elements entries.
//
// Type is one of: "related" (the symmetric atom-to-atom edge) or
// "decomposes-into" (molecule-to-constituent edge). Both flavors are
// emitted; the HTML/JS layer styles them differently.
type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// ToGraph projects an elements pool into the graph shape. Order of the
// returned Nodes/Edges is stable (sorted by id) for deterministic
// output across runs. clusterContinuityPath is passed straight to
// ComputeClusters — see its doc comment.
func ToGraph(pool map[string]*types.LexEntry, clusterContinuityPath string) Graph {
	nodes := make([]Node, 0, len(pool))
	edges := make([]Edge, 0, len(pool)*4)

	// First pass — compute in-degrees from the related edges of every
	// entry in the pool. Molecules' decomposes-into edges count
	// separately (toward the constituent atom's in-degree, since the
	// molecule "points at" the atom by including it).
	inDegree := map[string]int{}
	for _, e := range pool {
		for _, rel := range e.Related {
			if _, ok := pool[rel]; ok {
				inDegree[rel]++
			}
		}
		for _, dec := range e.DecomposesInto {
			if _, ok := pool[dec]; ok {
				inDegree[dec]++
			}
		}
	}

	// Compute communities once from the related[]+decomposes-into
	// edge set. The result replaces the previously hand-curated
	// cluster map; nodes get their community label inline.
	clusterByID, clusters := ComputeClusters(pool, clusterContinuityPath)

	// Second pass — emit nodes. Iterate the pool's keys in stable order
	// for reproducibility; relying on map-iteration would scramble the
	// JSON between runs and noise up diffs.
	for _, e := range sortedKeys(pool) {
		entry := pool[e]
		nodes = append(nodes, Node{
			ID:                 entry.ID,
			Name:               entry.Name,
			TypeIn:             entry.TypeIn,
			TypeOut:            entry.TypeOut,
			Tier:               entry.Tier,
			Status:             entry.Status,
			Cluster:            string(clusterByID[entry.ID]),
			Evokes:             entry.Evokes,
			CanonicalInstances: entry.CanonicalInstances,
			AgentInstruction:   entry.AgentInstruction,
			CriticalQuestions:  entry.CriticalQuestions,
			Lineage:            toLineage(entry.Lineage),
			InDegree:           inDegree[entry.ID],
			IsMolecule:         entry.Tier == "molecule",
		})
	}

	// Third pass — emit edges. Skip dangling edges (target not in pool)
	// to avoid orphan-arrows in the visualization.
	for _, src := range sortedKeys(pool) {
		entry := pool[src]
		for _, rel := range entry.Related {
			if _, ok := pool[rel]; !ok {
				continue
			}
			edges = append(edges, Edge{Source: entry.ID, Target: rel, Type: "related"})
		}
		for _, dec := range entry.DecomposesInto {
			if _, ok := pool[dec]; !ok {
				continue
			}
			edges = append(edges, Edge{Source: entry.ID, Target: dec, Type: "decomposes-into"})
		}
	}

	return Graph{Nodes: nodes, Edges: edges, Clusters: clusters}
}

// toLineage strips types.LineageEntry down to its reader-facing fields
// and renames them to the snake_case the rest of the export uses.
func toLineage(in []types.LineageEntry) []Lineage {
	if len(in) == 0 {
		return nil
	}
	out := make([]Lineage, len(in))
	for i, l := range in {
		out[i] = Lineage{Source: l.Source, Tradition: l.Tradition, Citation: l.Citation, Quote: l.Quote}
	}
	return out
}

// sortedKeys returns the pool's keys in lexicographic order — keeps
// JSON output deterministic between runs.
func sortedKeys(pool map[string]*types.LexEntry) []string {
	out := make([]string, 0, len(pool))
	for k := range pool {
		out = append(out, k)
	}
	// Inline insertion sort to avoid pulling in sort just for short keys.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
