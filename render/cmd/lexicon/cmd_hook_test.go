package main

import (
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/lens"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// TestReactionSteeringRendersSlots verifies the V71 reaction-steering
// block renders products/catalysts/inhibitors and resolves lex-id slot
// values to names (the predict/intervene view the agent translates).
func TestReactionSteeringRendersSlots(t *testing.T) {
	pool := map[string]*types.LexEntry{
		"lex-9001": {ID: "lex-9001", Name: "the-catalyst-atom", Tier: "atomic", TypeIn: "x", TypeOut: "y"},
		"lex-9000": {
			ID: "lex-9000", Name: "test-reaction", Tier: "reaction",
			Products:   []string{"a value-inversion posture"},
			Catalysts:  []string{"lex-9001", "felt impotence"},
			Inhibitors: []string{"stoic acceptance"},
		},
	}
	got := reactionSteering(pool["lex-9000"], pool)
	for _, want := range []string{
		"Reaction steering", "test-reaction", "heading toward",
		"value-inversion posture", "lever to watch",
		"lex-9001 (the-catalyst-atom)", "felt impotence",
		"intervention", "stoic acceptance",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reactionSteering output missing %q\n--- got ---\n%s", want, got)
		}
	}
}

// TestFormatHookInjectionIncludesReactionSteering verifies the hook
// injection appends the steering block when a reaction is among the
// fired matches (and not otherwise).
func TestFormatHookInjectionIncludesReactionSteering(t *testing.T) {
	reactionPool := map[string]*types.LexEntry{
		"lex-9000": {
			ID: "lex-9000", Name: "test-reaction", Tier: "reaction",
			TypeIn: "claim", TypeOut: "posture",
			Products:           []string{"a defended posture"},
			Catalysts:          []string{"high identity stakes"},
			CanonicalInstances: []string{"an example instance"},
		},
	}
	out := formatHookInjection([]types.GateResult{{PrimitiveID: "lex-9000", Score: 0.99}}, reactionPool, lens.Result{})
	if !strings.Contains(out, "Reaction steering") {
		t.Errorf("expected reaction steering in injection; got:\n%s", out)
	}

	// A non-reaction match must NOT produce a steering block.
	atomPool := map[string]*types.LexEntry{
		"lex-9002": {
			ID: "lex-9002", Name: "test-atom", Tier: "atomic",
			TypeIn: "claim", TypeOut: "posture",
			CanonicalInstances: []string{"an example instance"},
		},
	}
	out2 := formatHookInjection([]types.GateResult{{PrimitiveID: "lex-9002", Score: 0.99}}, atomPool, lens.Result{})
	if strings.Contains(out2, "Reaction steering") {
		t.Errorf("did not expect reaction steering for an atom match; got:\n%s", out2)
	}
}
