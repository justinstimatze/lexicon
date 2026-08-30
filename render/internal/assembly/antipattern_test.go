package assembly

import (
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

func TestEnumerateAntiPatterns(t *testing.T) {
	sub := map[string]*types.LexEntry{
		"lex-0001": {ID: "lex-0001", Name: "alpha"},
		"lex-0002": {ID: "lex-0002", Name: "beta"},
		"lex-0003": {ID: "lex-0003", Name: "gamma"},
		"lex-4yhqs": {
			ID:             "lex-4yhqs",
			Name:           "host-three",
			DecomposesInto: []string{"lex-0001", "lex-0002", "lex-0003"},
		},
	}
	got := EnumerateAntiPatterns(sub)
	// 3 atoms × 1 missing-each = 3 anti-patterns
	if len(got) != 3 {
		t.Fatalf("expected 3 anti-patterns, got %d", len(got))
	}
	names := map[string]bool{}
	for _, ap := range got {
		names[ap.Name] = true
		if ap.ParentID != "lex-4yhqs" {
			t.Errorf("wrong parent: %s", ap.ParentID)
		}
		if len(ap.RemainingIDs) != 2 {
			t.Errorf("expected 2 remaining ids, got %d", len(ap.RemainingIDs))
		}
	}
	want := []string{
		"host-three-without-alpha",
		"host-three-without-beta",
		"host-three-without-gamma",
	}
	for _, w := range want {
		if !names[w] {
			t.Errorf("missing anti-pattern %q in %v", w, names)
		}
	}
}

func TestEnumerateSkipsTooSmall(t *testing.T) {
	sub := map[string]*types.LexEntry{
		"lex-0001": {ID: "lex-0001", Name: "alpha"},
		"lex-4yhqs": {
			ID:             "lex-4yhqs",
			Name:           "single-atom-molecule",
			DecomposesInto: []string{"lex-0001"},
		},
	}
	got := EnumerateAntiPatterns(sub)
	if len(got) != 0 {
		t.Errorf("expected no anti-patterns for n=1, got %d", len(got))
	}
}

func TestEnumerateSkipsTooLarge(t *testing.T) {
	atoms := make([]string, 0, AntiPatternMaxN+1)
	sub := map[string]*types.LexEntry{}
	for i := 1; i <= AntiPatternMaxN+1; i++ {
		id := ""
		switch {
		case i < 10:
			id = "lex-000" + string(rune('0'+i))
		default:
			id = "lex-001" + string(rune('0'+i-10))
		}
		atoms = append(atoms, id)
		sub[id] = &types.LexEntry{ID: id, Name: id}
	}
	sub["lex-uz7g4"] = &types.LexEntry{
		ID:             "lex-uz7g4",
		Name:           "too-big",
		DecomposesInto: atoms,
	}
	got := EnumerateAntiPatterns(sub)
	for _, ap := range got {
		if ap.ParentID == "lex-uz7g4" {
			t.Errorf("expected lex-uz7g4 to be skipped (n=%d > %d), but got anti-pattern %s",
				len(atoms), AntiPatternMaxN, ap.Name)
		}
	}
}

func TestEnumerateSkipsMissingPlaceholders(t *testing.T) {
	sub := map[string]*types.LexEntry{
		"lex-0001": {ID: "lex-0001", Name: "alpha"},
		"lex-0002": {ID: "lex-0002", Name: "beta"},
		"lex-4yhqs": {
			ID:   "lex-4yhqs",
			Name: "host",
			// 2 real atoms + 1 [MISSING] flag — n=2 effective, 2 anti-patterns
			DecomposesInto: []string{"lex-0001", "[MISSING: case-similarity]", "lex-0002"},
		},
	}
	got := EnumerateAntiPatterns(sub)
	if len(got) != 2 {
		t.Fatalf("expected 2 anti-patterns (MISSING placeholder dropped), got %d", len(got))
	}
}

func TestEnumerateUnresolvedAtomGetsFallbackName(t *testing.T) {
	sub := map[string]*types.LexEntry{
		"lex-0001": {ID: "lex-0001", Name: "alpha"},
		"lex-4yhqs": {
			ID:             "lex-4yhqs",
			Name:           "host",
			DecomposesInto: []string{"lex-0001", "lex-uz7g4"}, // lex-uz7g4 not in elements
		},
	}
	got := EnumerateAntiPatterns(sub)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	for _, ap := range got {
		if ap.MissingID == "lex-uz7g4" && ap.MissingName != "<unnamed:lex-uz7g4>" {
			t.Errorf("expected fallback name for unresolved atom, got %q", ap.MissingName)
		}
	}
}
