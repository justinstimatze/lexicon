package main

import (
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// TestAmbiguityInterview verifies the V71 interview-bisection trigger:
// close top-two scores yield an interview directive naming both
// candidates; a dominant winner or a single match yields nothing.
func TestAmbiguityInterview(t *testing.T) {
	pool := map[string]*types.LexEntry{
		"lex-9000": {ID: "lex-9000", Name: "alpha-pattern"},
		"lex-9001": {ID: "lex-9001", Name: "beta-pattern"},
	}

	nearTie := []types.GateResult{
		{PrimitiveID: "lex-9000", Score: 0.82},
		{PrimitiveID: "lex-9001", Score: 0.75},
	}
	got := ambiguityInterview(nearTie, pool, 0.15)
	for _, want := range []string{"Ambiguous", "INTERVIEW", "alpha-pattern", "beta-pattern"} {
		if !strings.Contains(got, want) {
			t.Errorf("close-match interview missing %q; got: %q", want, got)
		}
	}

	dominant := []types.GateResult{
		{PrimitiveID: "lex-9000", Score: 0.95},
		{PrimitiveID: "lex-9001", Score: 0.40},
	}
	if out := ambiguityInterview(dominant, pool, 0.15); out != "" {
		t.Errorf("dominant winner should yield no interview; got: %q", out)
	}

	if out := ambiguityInterview(nearTie[:1], pool, 0.15); out != "" {
		t.Errorf("single match should yield no interview; got: %q", out)
	}
}
