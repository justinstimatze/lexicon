package assembly

import (
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

func TestTreeEditDistanceIdentity(t *testing.T) {
	src := "sequential(lex-0001, lex-0002)"
	a, _ := Parse(src)
	b, _ := Parse(src)
	if d := TreeEditDistance(toTree(a), toTree(b)); d != 0 {
		t.Errorf("identity TED expected 0, got %d", d)
	}
}

func TestTreeEditDistanceSingleAtomSwap(t *testing.T) {
	a, _ := Parse("sequential(lex-0001, lex-0002)")
	b, _ := Parse("sequential(lex-0001, lex-0003)")
	d := TreeEditDistance(toTree(a), toTree(b))
	if d != 1 {
		t.Errorf("single atom swap should be TED=1, got %d", d)
	}
}

func TestTreeEditDistanceOpSwap(t *testing.T) {
	a, _ := Parse("sequential(lex-0001, lex-0002)")
	b, _ := Parse("parallel(lex-0001, lex-0002)")
	d := TreeEditDistance(toTree(a), toTree(b))
	if d != 1 {
		t.Errorf("op swap (sequential→parallel) should be TED=1, got %d", d)
	}
}

func TestTreeEditDistanceInsertAtom(t *testing.T) {
	a, _ := Parse("sequential(lex-0001, lex-0002)")
	b, _ := Parse("sequential(lex-0001, lex-0002, lex-0003)")
	d := TreeEditDistance(toTree(a), toTree(b))
	if d != 1 {
		t.Errorf("inserting one atom should be TED=1, got %d", d)
	}
}

func TestTreeEditDistanceSymmetric(t *testing.T) {
	a, _ := Parse("sequential(lex-0001, lex-0002)")
	b, _ := Parse("sequential(lex-0001, parallel(lex-0002, lex-0003))")
	dAB := TreeEditDistance(toTree(a), toTree(b))
	dBA := TreeEditDistance(toTree(b), toTree(a))
	if dAB != dBA {
		t.Errorf("TED should be symmetric: a→b=%d, b→a=%d", dAB, dBA)
	}
}

func TestJaccardDistance(t *testing.T) {
	cases := []struct {
		a    []string
		b    []string
		want float64
	}{
		{[]string{}, []string{}, 0},
		{[]string{"x"}, []string{"x"}, 0},
		{[]string{"x"}, []string{"y"}, 1},
		{[]string{"x", "y"}, []string{"y", "z"}, 2.0 / 3.0}, // |∩|=1, |∪|=3, dist = 1 - 1/3 = 2/3
		{[]string{"x", "y", "z"}, []string{"x", "y", "z"}, 0},
	}
	for _, tc := range cases {
		got := jaccardDistance(tc.a, tc.b)
		if (got-tc.want) > 1e-9 || (tc.want-got) > 1e-9 {
			t.Errorf("jaccardDistance(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMolecularDistanceFullStack(t *testing.T) {
	a := &types.LexEntry{
		ID:             "lex-A",
		TypeIn:         "claim",
		TypeOut:        "posture",
		Assembly:       "sequential(lex-0001, lex-0002)",
		DecomposesInto: []string{"lex-0001", "lex-0002"},
	}
	// Identical structure but different atom in second slot.
	b := &types.LexEntry{
		ID:             "lex-B",
		TypeIn:         "claim",
		TypeOut:        "posture",
		Assembly:       "sequential(lex-0001, lex-0003)",
		DecomposesInto: []string{"lex-0001", "lex-0003"},
	}
	d := MolecularDistance(a, b)
	if d.TED != 1 {
		t.Errorf("TED should be 1 for single atom swap, got %d", d.TED)
	}
	if d.TypeMismatch != 0 {
		t.Errorf("type signatures match, expected TypeMismatch=0, got %d", d.TypeMismatch)
	}
	// Jaccard: |∩|=1 (lex-0001), |∪|=3 (lex-0001, lex-0002, lex-0003), dist = 2/3
	want := 2.0 / 3.0
	if (d.Jaccard - want) > 1e-9 {
		t.Errorf("Jaccard expected %v, got %v", want, d.Jaccard)
	}
}

func TestMolecularDistanceTypeMismatch(t *testing.T) {
	a := &types.LexEntry{ID: "lex-A", TypeIn: "claim", TypeOut: "posture"}
	b := &types.LexEntry{ID: "lex-B", TypeIn: "state", TypeOut: "posture"}
	d := MolecularDistance(a, b)
	if d.TypeMismatch != 1 {
		t.Errorf("differing type-in should set TypeMismatch=1, got %d", d.TypeMismatch)
	}
}

func TestRankNeighborsExcludesHost(t *testing.T) {
	host := &types.LexEntry{
		ID:             "lex-A",
		Assembly:       "sequential(lex-0001, lex-0002)",
		DecomposesInto: []string{"lex-0001", "lex-0002"},
	}
	sub := map[string]*types.LexEntry{
		"lex-A": host,
		"lex-B": {
			ID:             "lex-B",
			Assembly:       "sequential(lex-0001, lex-0002)",
			DecomposesInto: []string{"lex-0001", "lex-0002"},
		},
	}
	got := RankNeighbors(host, sub, DefaultWeights(), 5)
	for _, n := range got {
		if n.ID == "lex-A" {
			t.Errorf("RankNeighbors should exclude host, but included %s", n.ID)
		}
	}
	if len(got) != 1 {
		t.Errorf("expected 1 neighbor (the only non-host), got %d", len(got))
	}
}

func TestRankNeighborsSkipsAtomsWithoutComposition(t *testing.T) {
	host := &types.LexEntry{
		ID:             "lex-A",
		Assembly:       "sequential(lex-0001, lex-0002)",
		DecomposesInto: []string{"lex-0001", "lex-0002"},
	}
	sub := map[string]*types.LexEntry{
		"lex-A":     host,
		"lex-atom1": {ID: "lex-atom1", TypeIn: "claim"},   // no assembly, no decomposes-into
		"lex-mol1":  {ID: "lex-mol1", Assembly: "lex-0001"}, // has assembly
	}
	got := RankNeighbors(host, sub, DefaultWeights(), 5)
	for _, n := range got {
		if n.ID == "lex-atom1" {
			t.Errorf("expected lex-atom1 to be skipped (no composition fields), but it ranked")
		}
	}
}
