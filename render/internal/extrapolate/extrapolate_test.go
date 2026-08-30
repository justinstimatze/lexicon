package extrapolate

import (
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

func TestFrontier(t *testing.T) {
	atoms := map[string]*types.LexEntry{
		"lex-0001": {ID: "lex-0001", Name: "A", Tier: "atom", Status: "active",
			Related: []string{"lex-4yhqs", "lex-chp44"}},
		"lex-0002": {ID: "lex-0002", Name: "B", Tier: "atom", Status: "active",
			Related: []string{"lex-4yhqs", "lex-68abd", "lex-0001"}},
		"lex-0003": {ID: "lex-0003", Name: "C", Tier: "atom", Status: "active",
			Related: []string{"lex-4yhqs"}},
		"lex-4yhqs": {ID: "lex-4yhqs", Name: "Frontier-3", Tier: "atom", Status: "active"},
		"lex-chp44": {ID: "lex-chp44", Name: "Frontier-1a", Tier: "atom", Status: "active"},
		"lex-68abd": {ID: "lex-68abd", Name: "Frontier-1b", Tier: "atom", Status: "active"},
	}
	r := Frontier([]string{"lex-0001", "lex-0002", "lex-0003"}, atoms)
	if len(r.Missing) != 0 {
		t.Fatalf("expected no missing, got %v", r.Missing)
	}
	// lex-0001 referenced by lex-0002 but lex-0001 is IN the constellation, so excluded.
	// Expected frontier: lex-4yhqs (×3), lex-chp44 (×1), lex-68abd (×1).
	if len(r.Candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %+v", len(r.Candidates), r.Candidates)
	}
	if r.Candidates[0].ID != "lex-4yhqs" || r.Candidates[0].AdjacencyCount != 3 {
		t.Errorf("expected lex-4yhqs ×3 at top, got %+v", r.Candidates[0])
	}
	// Lexicographic ID order decides the tie: "lex-68abd" comes before
	// "lex-chp44" as strings, a fact of these specific random ids, not a
	// rule about which one was named first.
	if r.Candidates[1].ID != "lex-68abd" || r.Candidates[2].ID != "lex-chp44" {
		t.Errorf("expected lex-68abd then lex-chp44 in tie, got %s then %s",
			r.Candidates[1].ID, r.Candidates[2].ID)
	}
	if r.Candidates[0].Name != "Frontier-3" {
		t.Errorf("expected name populated from elements, got %q", r.Candidates[0].Name)
	}
}

func TestFrontierMissingConstellationID(t *testing.T) {
	atoms := map[string]*types.LexEntry{
		"lex-0001": {ID: "lex-0001", Related: []string{"lex-4yhqs"}},
	}
	r := Frontier([]string{"lex-0001", "lex-9999"}, atoms)
	if len(r.Missing) != 1 || r.Missing[0] != "lex-9999" {
		t.Errorf("expected lex-9999 in missing, got %v", r.Missing)
	}
	if len(r.Candidates) != 1 || r.Candidates[0].ID != "lex-4yhqs" {
		t.Errorf("expected frontier from lex-0001 only, got %+v", r.Candidates)
	}
}

func TestFrontierUnknownRelatedSurfaced(t *testing.T) {
	// Related points to an atom not in the map (data-integrity gap).
	// Should still be surfaced — caller may want to act on it.
	atoms := map[string]*types.LexEntry{
		"lex-0001": {ID: "lex-0001", Related: []string{"lex-9998"}},
	}
	r := Frontier([]string{"lex-0001"}, atoms)
	if len(r.Candidates) != 1 || r.Candidates[0].ID != "lex-9998" {
		t.Fatalf("expected lex-9998 surfaced, got %+v", r.Candidates)
	}
	if r.Candidates[0].Name != "" {
		t.Errorf("unknown related should have empty name, got %q", r.Candidates[0].Name)
	}
}
