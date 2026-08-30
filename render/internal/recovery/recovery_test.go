package recovery

import (
	"math"
	"testing"
)

func TestPerModelRecoveryFullCoverage(t *testing.T) {
	models := []NamedModel{{
		ID:         "rm-001",
		Name:       "expert-opinion",
		ShouldFire: []string{"lex-A", "lex-B"},
		Excerpts: []Excerpt{
			{ID: "e1", Fired: map[string]bool{"lex-A": true, "lex-B": true}},
			{ID: "e2", Fired: map[string]bool{"lex-A": true, "lex-B": true}},
		},
	}}
	res := Compute(models)
	if len(res.PerModelRecovery) != 1 {
		t.Fatalf("expected 1 model, got %d", len(res.PerModelRecovery))
	}
	mr := res.PerModelRecovery[0]
	if mr.FullyRecovered != 2 {
		t.Errorf("expected 2 fully-recovered, got %d", mr.FullyRecovered)
	}
	if mr.Rate != 1.0 {
		t.Errorf("expected rate 1.0, got %v", mr.Rate)
	}
}

func TestPerModelRecoveryPartial(t *testing.T) {
	models := []NamedModel{{
		ID:         "rm-002",
		Name:       "two-piece-model",
		ShouldFire: []string{"lex-A", "lex-B"},
		Excerpts: []Excerpt{
			{ID: "e1", Fired: map[string]bool{"lex-A": true}},  // partial
			{ID: "e2", Fired: map[string]bool{"lex-A": true, "lex-B": true}}, // full
			{ID: "e3", Fired: map[string]bool{"lex-C": true}},  // none of should_fire
		},
	}}
	res := Compute(models)
	mr := res.PerModelRecovery[0]
	if mr.FullyRecovered != 1 {
		t.Errorf("expected 1 full, got %d", mr.FullyRecovered)
	}
	if mr.PartiallyRecovered != 1 {
		t.Errorf("expected 1 partial, got %d", mr.PartiallyRecovered)
	}
	if mr.NotRecovered != 1 {
		t.Errorf("expected 1 none, got %d", mr.NotRecovered)
	}
	want := 1.0 / 3.0
	if math.Abs(mr.Rate-want) > 1e-9 {
		t.Errorf("expected rate %v, got %v", want, mr.Rate)
	}
}

func TestDiscriminativityNonZeroForExpectedAtoms(t *testing.T) {
	// Note: χ² magnitude is symmetric on the rm-A vs rm-B partition for
	// this small fixture (lex-1 fires iff model=rm-A, lex-2 fires iff
	// model=rm-B). The signal we test here is that the χ² value is
	// non-zero (the entry discriminates *something*), and that
	// IsExpected correctly tags entries listed in should_fire.
	models := []NamedModel{
		{
			ID:         "rm-A",
			ShouldFire: []string{"lex-1"},
			Excerpts: []Excerpt{
				{ID: "ea1", Fired: map[string]bool{"lex-1": true}},
				{ID: "ea2", Fired: map[string]bool{"lex-1": true}},
			},
		},
		{
			ID:         "rm-B",
			ShouldFire: []string{"lex-2"},
			Excerpts: []Excerpt{
				{ID: "eb1", Fired: map[string]bool{"lex-2": true}},
				{ID: "eb2", Fired: map[string]bool{"lex-2": true}},
			},
		},
	}
	res := Compute(models)
	var foundExpected, foundNonZero bool
	for _, d := range res.EntryDiscriminativity {
		if d.EntryID == "lex-1" && d.ModelID == "rm-A" {
			if d.ChiSquared > 0 {
				foundNonZero = true
			}
			if d.IsExpected {
				foundExpected = true
			}
		}
		if d.EntryID == "lex-1" && d.ModelID == "rm-B" && d.IsExpected {
			t.Errorf("lex-1 should not be IsExpected for rm-B (not in should_fire)")
		}
	}
	if !foundNonZero {
		t.Errorf("expected lex-1 / rm-A χ² > 0")
	}
	if !foundExpected {
		t.Errorf("expected lex-1 / rm-A to be marked IsExpected")
	}
}

func TestPairwiseRedundancyIdenticalFirings(t *testing.T) {
	// Two entries that fire on exactly the same excerpts should have
	// Jaccard = 1.
	models := []NamedModel{{
		ID: "rm-1",
		Excerpts: []Excerpt{
			{ID: "e1", Fired: map[string]bool{"lex-A": true, "lex-B": true}},
			{ID: "e2", Fired: map[string]bool{"lex-A": true, "lex-B": true}},
			{ID: "e3", Fired: map[string]bool{}},
		},
	}}
	res := Compute(models)
	for _, p := range res.PairwiseRedundancy {
		if (p.EntryA == "lex-A" && p.EntryB == "lex-B") || (p.EntryA == "lex-B" && p.EntryB == "lex-A") {
			if p.Jaccard != 1.0 {
				t.Errorf("identical firings should give Jaccard=1, got %v", p.Jaccard)
			}
			return
		}
	}
	t.Errorf("did not find lex-A / lex-B pair in redundancy output")
}

func TestPairwiseRedundancySkipsZero(t *testing.T) {
	// Entries that never co-fire should be omitted (output noise reduction).
	models := []NamedModel{{
		ID: "rm-1",
		Excerpts: []Excerpt{
			{ID: "e1", Fired: map[string]bool{"lex-A": true}},
			{ID: "e2", Fired: map[string]bool{"lex-B": true}},
		},
	}}
	res := Compute(models)
	for _, p := range res.PairwiseRedundancy {
		if p.Jaccard == 0 {
			t.Errorf("zero-Jaccard pair should be omitted, got %+v", p)
		}
	}
}
