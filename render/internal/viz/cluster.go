// Package viz generates the interactive elements views (graph,
// pivot, matrix). Clusters here are computed at runtime by label
// propagation over the related[] + decomposes-into edge set, so the
// cluster vocabulary stays in sync with elements structure instead
// of drifting from a hand-curated map. Cluster *identity* (which id a
// given community keeps across runs) is separately persisted via
// docs/cluster-continuity.json — see ComputeClusters and
// cluster_continuity.go.
package viz

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// ClusterID is an opaque label assigned by community detection —
// "c" plus a 4-char code from the same hand-typing-safe alphabet
// cmd_renumber.go uses for atom ids (see drawClusterCode), persisted
// across runs by cluster_continuity.go rather than reassigned by
// current size-rank. Display order in Graph.Clusters is still
// size-sorted; the id string itself carries no rank or creation-order
// information to read into.
type ClusterID string

const (
	// ClusterUnclustered is the bucket for atoms with no edges. Greyed
	// out in the UI and listed last in the legend.
	ClusterUnclustered ClusterID = "c00"
)

// ClusterMeta is display info for one cluster — id, color, and a
// human-readable label of the form "c8h3q (54) — <hub atom name>".
type ClusterMeta struct {
	ID    ClusterID `json:"id"`
	Name  string    `json:"name"`
	Label string    `json:"label"`
	Color string    `json:"color"`
}

// Tufte-friendly pastel palette — limited contrast on purpose so
// cluster blocks read as gentle bands rather than competing for
// attention. Cycles when an elements snapshot has more communities
// than colors; in practice 12 distinct colors covers the common case.
// Tuned to read distinctly against the dark sepia bg (#16130f). Each
// is around ~65–70% lightness with ~40–60% saturation — saturated
// enough to differentiate by hue against the warm-black bg, light
// enough that dark text on the chip (color: var(--bg)) stays legible.
// Warm-leaning so the chips sit inside the same family as the accent
// gold rather than fighting it.
var clusterPalette = []string{
	"#e89090", // coral
	"#e8b56b", // amber gold (sibling of --accent)
	"#a8c878", // sage
	"#7dcab1", // mint-teal
	"#88b8e0", // sky blue
	"#b6a3e0", // lavender
	"#e0a3cf", // rose pink
	"#e8a373", // peach
	"#d4c878", // dijon
	"#80c0c8", // aqua
	"#c890b8", // mauve
	"#b8c878", // olive
}

// ComputeClusters runs greedy modularity optimization (Louvain
// phase 1) over the elements' related[] + decomposes-into edge set
// and returns (1) a cluster-id per atom, and (2) the ClusterMeta list
// to feed Graph.Clusters, ordered by community size descending (so a
// legend/pivot iterating Graph.Clusters still sees biggest-first).
//
// continuityPath, if non-empty, is a docs/cluster-continuity.json-style
// file this reads before assigning ids and rewrites after: a newly
// computed community keeps the id of whichever previous-run community
// it overlaps most (Jaccard, see matchStableIDs), so a cluster's id
// and color survive a re-run as long as its membership didn't shift
// much — traded for the id no longer being a readable size-rank (the
// label's own "(N)" count still says how big a cluster currently is).
// Pass "" to skip persistence entirely — every community is then
// treated as new and draws a fresh opaque id every call, e.g. for
// tests that don't care about cross-run stability.
//
// Determinism: per-pass node order is a Fisher-Yates shuffle with a
// pass-derived seed; neighbor sets and candidate communities are
// sorted before iteration; same elements produces the same partition
// across runs.
//
// Algorithm: greedy modularity. Each node starts in its own
// community. Per pass, each node considers moving to each of its
// neighbor communities (and staying); modularity gain for moving v
// to c is ΔQ ∝ k_{v,c} − Σtot[c] · k_v / 2m where k_{v,c} is edges
// from v to c, Σtot[c] is the sum of degrees in c, k_v is v's degree,
// and 2m is twice the edge count. Move v to the community maximizing
// gain. Repeat passes until no node moves. This replaces the previous
// LPA pass, which on dense elements collapsed into a single giant
// community (the (Σtot · k_v)/2m penalty here is what prevents that:
// joining a large community costs more than joining a small one with
// the same edge count).
//
// Isolated atoms (no edges) stay in their initial singleton community
// and are collected into ClusterUnclustered at emit time.
func ComputeClusters(pool map[string]*types.LexEntry, continuityPath string) (map[string]ClusterID, []ClusterMeta) {
	ids := sortedKeys(pool)
	if len(ids) == 0 {
		return map[string]ClusterID{}, nil
	}

	// Build undirected adjacency. Both related[] and decomposes-into
	// contribute — a molecule and its constituent are clearly in the
	// same community for clustering purposes.
	adj := make(map[string]map[string]bool, len(ids))
	for _, id := range ids {
		adj[id] = map[string]bool{}
	}
	for _, id := range ids {
		e := pool[id]
		for _, rel := range e.Related {
			if _, ok := pool[rel]; !ok {
				continue
			}
			adj[id][rel] = true
			adj[rel][id] = true
		}
		for _, dec := range e.DecomposesInto {
			if _, ok := pool[dec]; !ok {
				continue
			}
			adj[id][dec] = true
			adj[dec][id] = true
		}
	}

	// twoM = 2m, the sum of all degrees (each undirected edge
	// contributes 2). Used as the modularity normalizer.
	var twoM float64
	for _, id := range ids {
		twoM += float64(len(adj[id]))
	}
	if twoM == 0 {
		// All atoms isolated — return all-unclustered.
		out := map[string]ClusterID{}
		for _, id := range ids {
			out[id] = ClusterUnclustered
		}
		return out, []ClusterMeta{{
			ID:    ClusterUnclustered,
			Name:  string(ClusterUnclustered),
			Label: fmt.Sprintf("%s (%d) — isolated", ClusterUnclustered, len(ids)),
			Color: "#eeeeee",
		}}
	}

	// Each atom starts as its own community. label[v] = integer id.
	label := make(map[string]int, len(ids))
	for i, id := range ids {
		label[id] = i
	}

	// Σtot per community = sum of degrees of its nodes. Initialized
	// to each node's own degree (each in its singleton community).
	sigmaTot := make(map[int]float64, len(ids))
	for i, id := range ids {
		sigmaTot[i] = float64(len(adj[id]))
	}

	const maxPasses = 40
	order := make([]string, len(ids))
	copy(order, ids)
	for pass := 0; pass < maxPasses; pass++ {
		seed := uint32(pass*2654435761 + 1)
		for i := len(order) - 1; i > 0; i-- {
			seed = seed*1664525 + 1013904223
			j := int(seed % uint32(i+1))
			order[i], order[j] = order[j], order[i]
		}
		moved := false
		for _, id := range order {
			kv := float64(len(adj[id]))
			if kv == 0 {
				continue
			}
			currentC := label[id]

			// Tabulate edges from v to each neighbor community.
			// Sort neighbors for deterministic iteration over the map.
			neighbors := make([]string, 0, len(adj[id]))
			for n := range adj[id] {
				neighbors = append(neighbors, n)
			}
			sort.Strings(neighbors)
			kvc := make(map[int]float64)
			for _, n := range neighbors {
				kvc[label[n]]++
			}

			// Remove v from its current community so the gain
			// calculation for every candidate (including currentC)
			// uses the same removed-state baseline.
			sigmaTot[currentC] -= kv

			// Ensure currentC is in the candidate set even if v has
			// no neighbors in it (k_{v,currentC} == 0 in that case).
			if _, ok := kvc[currentC]; !ok {
				kvc[currentC] = 0
			}

			cands := make([]int, 0, len(kvc))
			for c := range kvc {
				cands = append(cands, c)
			}
			sort.Ints(cands)
			bestC := currentC
			bestGain := math.Inf(-1)
			for _, c := range cands {
				gain := kvc[c] - sigmaTot[c]*kv/twoM
				if gain > bestGain {
					bestGain = gain
					bestC = c
				}
			}
			label[id] = bestC
			sigmaTot[bestC] += kv
			if bestC != currentC {
				moved = true
			}
		}
		if !moved {
			break
		}
	}

	// Group nodes by community label; isolated nodes go to the
	// unclustered bucket regardless of their initial singleton label.
	groups := map[int][]string{}
	for id, l := range label {
		if len(adj[id]) == 0 {
			groups[-1] = append(groups[-1], id)
			continue
		}
		groups[l] = append(groups[l], id)
	}

	type sized struct {
		labelInt int
		members  []string
	}
	ordered := []sized{}
	for l, m := range groups {
		if l == -1 {
			continue
		}
		sort.Strings(m)
		ordered = append(ordered, sized{l, m})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if len(ordered[i].members) != len(ordered[j].members) {
			return len(ordered[i].members) > len(ordered[j].members)
		}
		return ordered[i].members[0] < ordered[j].members[0]
	})

	communities := make([][]string, len(ordered))
	for i, grp := range ordered {
		communities[i] = grp.members
	}
	prev := loadClusterContinuity(continuityPath)
	stableIDs, next := matchStableIDs(communities, prev)
	if err := saveClusterContinuity(continuityPath, next); err != nil {
		// Best-effort: losing continuity just means the next run treats
		// every cluster as new, not a correctness failure worth aborting
		// the whole export over.
		fmt.Fprintf(os.Stderr, "cluster continuity: %v\n", err)
	}

	out := map[string]ClusterID{}
	meta := []ClusterMeta{}
	for i, grp := range ordered {
		cid := stableIDs[i]
		hub := grp.members[0]
		hubDeg := len(adj[hub])
		for _, m := range grp.members {
			if d := len(adj[m]); d > hubDeg {
				hub = m
				hubDeg = d
			}
		}
		// Full hub name, untruncated -- truncation is a display concern for
		// whichever UI has limited width (a table cell), not something to
		// bake into the data every consumer inherits, including the ones
		// with room to show it in full (e.g. a detail modal).
		hubName := pool[hub].Name
		label := fmt.Sprintf("%s (%d) — %s", cid, len(grp.members), hubName)
		meta = append(meta, ClusterMeta{
			ID:    cid,
			Name:  string(cid),
			Label: label,
			Color: clusterPalette[i%len(clusterPalette)],
		})
		for _, m := range grp.members {
			out[m] = cid
		}
	}
	if iso := groups[-1]; len(iso) > 0 {
		sort.Strings(iso)
		for _, m := range iso {
			out[m] = ClusterUnclustered
		}
		meta = append(meta, ClusterMeta{
			ID:    ClusterUnclustered,
			Name:  string(ClusterUnclustered),
			Label: fmt.Sprintf("%s (%d) — isolated", ClusterUnclustered, len(iso)),
			Color: "#eeeeee",
		})
	}
	return out, meta
}
