package assembly

import (
	"sort"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Distance is the breakdown for one molecule pair.
type Distance struct {
	Jaccard      float64 // atom-set Jaccard distance, 0..1
	TED          int     // Zhang-Shasha tree-edit distance on parsed assembly
	TypeMismatch int     // 0 if type-in and type-out agree, else 1
}

// Weights govern how the three components combine. Defaults are
// w_atom=1.0, w_assembly=1.0 (raw TED), w_type=0.5. Per the round-2
// plan, exposed as flags so the maintainer can run the metric under
// multiple weightings rather than picking one in v0.
type Weights struct {
	Atom     float64
	Assembly float64
	Type     float64
}

// DefaultWeights matches the round-2 implementation plan.
func DefaultWeights() Weights {
	return Weights{Atom: 1.0, Assembly: 1.0, Type: 0.5}
}

// Combined applies the weights to the breakdown.
func (d Distance) Combined(w Weights) float64 {
	return w.Atom*d.Jaccard + w.Assembly*float64(d.TED) + w.Type*float64(d.TypeMismatch)
}

// MolecularDistance computes the Distance between two molecules.
// When either side has no assembly, TED is 0 and only the atom-set /
// type-signature components contribute. When either side has no
// decomposes-into, Jaccard is 0.
func MolecularDistance(a, b *types.LexEntry) Distance {
	d := Distance{}
	d.Jaccard = jaccardDistance(cleanDecompose(a.DecomposesInto), cleanDecompose(b.DecomposesInto))
	d.TED = parsedAssemblyTED(a.Assembly, b.Assembly)
	if a.TypeIn != b.TypeIn || a.TypeOut != b.TypeOut {
		d.TypeMismatch = 1
	}
	return d
}

// Neighbor is one nearest-neighbor entry from RankNeighbors.
type Neighbor struct {
	ID       string
	Distance Distance
	Score    float64
}

// RankNeighbors returns the top-k nearest molecules to host (excluding
// host itself) under the given weights, sorted by combined score
// ascending. Only entries with at least one of (assembly, decomposes-into)
// are considered; pure atoms without composition fields can't be
// compared meaningfully.
func RankNeighbors(host *types.LexEntry, elements map[string]*types.LexEntry, w Weights, k int) []Neighbor {
	var ranked []Neighbor
	for id, e := range elements {
		if id == host.ID {
			continue
		}
		if e.Assembly == "" && len(e.DecomposesInto) == 0 {
			continue
		}
		d := MolecularDistance(host, e)
		ranked = append(ranked, Neighbor{ID: id, Distance: d, Score: d.Combined(w)})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score < ranked[j].Score
		}
		return ranked[i].ID < ranked[j].ID
	})
	if k > 0 && len(ranked) > k {
		ranked = ranked[:k]
	}
	return ranked
}

// --- atom-set Jaccard ---

func jaccardDistance(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	setA := map[string]bool{}
	for _, x := range a {
		setA[x] = true
	}
	setB := map[string]bool{}
	for _, x := range b {
		setB[x] = true
	}
	inter := 0
	for x := range setA {
		if setB[x] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return 1 - float64(inter)/float64(union)
}

// --- assembly tree-edit distance (Zhang-Shasha) ---

// parsedAssemblyTED parses both sides and computes Zhang-Shasha TED.
// Returns 0 when either side fails to parse or is empty (the
// distance is undefined; callers can interpret 0-with-empty as
// "incomparable" via the absence of an assembly field).
func parsedAssemblyTED(srcA, srcB string) int {
	if srcA == "" || srcB == "" {
		return 0
	}
	na, errA := Parse(srcA)
	nb, errB := Parse(srcB)
	if errA != nil || errB != nil {
		return 0
	}
	return TreeEditDistance(toTree(na), toTree(nb))
}

// TreeNode is a label-node tree used by the Zhang-Shasha algorithm.
// Decoupled from the parser AST so the algorithm has no dependency
// on assembly semantics.
type TreeNode struct {
	Label    string
	Children []*TreeNode
}

// toTree converts the parser AST into TreeNode form for TED.
// Labels carry node-kind so substitutions only happen between
// like-with-like (atom↔atom, op↔op, etc.). Predicate values are
// included with key (`named-arg:until=corrections-small`).
func toTree(n Node) *TreeNode {
	switch v := n.(type) {
	case *OpNode:
		t := &TreeNode{Label: "op:" + v.Op}
		for _, a := range v.Args {
			t.Children = append(t.Children, toTree(a))
		}
		for _, k := range v.NamedKeys {
			rhs := toTree(v.NamedArgs[k])
			named := &TreeNode{Label: "named-arg:" + k, Children: []*TreeNode{rhs}}
			t.Children = append(t.Children, named)
		}
		return t
	case *AtomLeaf:
		return &TreeNode{Label: "atom:" + v.ID}
	case *MissingLeaf:
		return &TreeNode{Label: "missing:" + v.Name}
	case *Predicate:
		return &TreeNode{Label: "predicate:" + v.Name}
	case *Ellipsis:
		return &TreeNode{Label: "ellipsis"}
	}
	return &TreeNode{Label: "?"}
}

// TreeEditDistance computes the Zhang-Shasha tree-edit distance
// between two label-node trees, with unit cost for insert / delete /
// label-substitution. O(n² × |keyroots(T1)| × |keyroots(T2)|) — for
// the elements' shallow trees (≤8 nodes each, ≤15 molecules with
// assembly) this is milliseconds total.
//
// Reference: Zhang & Shasha 1989 §3, "Simple fast algorithms for the
// editing distance between trees and related problems."
func TreeEditDistance(a, b *TreeNode) int {
	ta := buildTedTree(a)
	tb := buildTedTree(b)
	n := len(ta.nodes)
	m := len(tb.nodes)
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}
	treedist := make([][]int, n)
	for i := range treedist {
		treedist[i] = make([]int, m)
	}
	for _, ki := range ta.keyroots {
		for _, kj := range tb.keyroots {
			forestDist(ta, tb, ki, kj, treedist)
		}
	}
	return treedist[n-1][m-1]
}

type tedTree struct {
	nodes    []*TreeNode // postorder
	l        []int       // l[i] = postorder index of leftmost leaf in subtree rooted at i
	keyroots []int
}

func buildTedTree(root *TreeNode) *tedTree {
	t := &tedTree{}
	var visit func(n *TreeNode) int // returns leftmost-leaf index
	visit = func(n *TreeNode) int {
		var leftmost = -1
		for i, c := range n.Children {
			lm := visit(c)
			if i == 0 {
				leftmost = lm
			}
		}
		t.nodes = append(t.nodes, n)
		idx := len(t.nodes) - 1
		if leftmost == -1 {
			leftmost = idx
		}
		t.l = append(t.l, leftmost)
		return leftmost
	}
	if root != nil {
		visit(root)
	}
	// keyroots: largest postorder index for each unique l value
	largest := map[int]int{}
	for i := 0; i < len(t.l); i++ {
		largest[t.l[i]] = i
	}
	for _, idx := range largest {
		t.keyroots = append(t.keyroots, idx)
	}
	sort.Ints(t.keyroots)
	return t
}

// forestDist fills treedist for the keyroot pair (ki, kj). Standard
// Zhang-Shasha forest-distance DP.
func forestDist(ta, tb *tedTree, ki, kj int, treedist [][]int) {
	li := ta.l[ki]
	lj := tb.l[kj]
	p := ki - li + 1 // forest length on T1
	q := kj - lj + 1 // forest length on T2
	fd := make([][]int, p+1)
	for i := range fd {
		fd[i] = make([]int, q+1)
	}
	for x := 1; x <= p; x++ {
		fd[x][0] = fd[x-1][0] + 1
	}
	for y := 1; y <= q; y++ {
		fd[0][y] = fd[0][y-1] + 1
	}
	for x := 1; x <= p; x++ {
		for y := 1; y <= q; y++ {
			ai := li + x - 1
			bj := lj + y - 1
			if ta.l[ai] == li && tb.l[bj] == lj {
				cost := 1
				if ta.nodes[ai].Label == tb.nodes[bj].Label {
					cost = 0
				}
				fd[x][y] = min3(
					fd[x-1][y]+1,
					fd[x][y-1]+1,
					fd[x-1][y-1]+cost,
				)
				treedist[ai][bj] = fd[x][y]
			} else {
				lax := ta.l[ai] - li
				lby := tb.l[bj] - lj
				fd[x][y] = min3(
					fd[x-1][y]+1,
					fd[x][y-1]+1,
					fd[lax][lby]+treedist[ai][bj],
				)
			}
		}
	}
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
