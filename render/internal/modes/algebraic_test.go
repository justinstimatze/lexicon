package modes

import (
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

var fixtureAtom = &types.LexEntry{
	ID:                 "lex-uz7g4",
	Name:               "test-entry",
	TypeIn:             "claim",
	TypeOut:            "posture",
	Tier:               "atomic",
	Lineage:            []types.LineageEntry{{Source: "walton", Text: "wrm-2008", Citation: "ch.1"}},
	CanonicalInstances: []string{"an example"},
	Status:             "under-review",
}

func TestAlgebraicReturnsExpectedShape(t *testing.T) {
	out := Algebraic(fixtureAtom)
	if out.PrimitiveID != "lex-uz7g4" || out.Mode != types.ModeAlgebraic {
		t.Fatalf("output meta wrong: %+v", out)
	}
	if !strings.Contains(out.Text, "id: lex-uz7g4") || !strings.Contains(out.Text, "name: test-entry") {
		t.Fatalf("yaml body missing fields: %q", out.Text)
	}
	if !strings.Contains(out.Text, "algebraic (raw elements)") {
		t.Fatalf("header missing: %q", out.Text)
	}
}
