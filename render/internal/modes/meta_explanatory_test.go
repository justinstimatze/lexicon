package modes

import (
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

var fixtureMolecule = &types.LexEntry{
	ID:                 "lex-kebfa",
	Name:               "argument-from-expert-opinion",
	TypeIn:             "claim",
	TypeOut:            "posture",
	Tier:               "molecule",
	Related:            []string{"lex-af9ax"},
	Evokes:             []string{"appeal-to-authority"},
	DecomposesInto:     []string{"lex-q9asc", "lex-dm5te"},
	CriticalQuestions:  []string{"is E really an expert?"},
	Lineage:            []types.LineageEntry{{Source: "walton", Text: "wrm-2008", Citation: "ch.3"}},
	CanonicalInstances: []string{"Dr. X says T helps C"},
	SeverityTier:       "info",
	Status:             "under-review",
}

// MetaExplanatory must surface the load-bearing fields a
// design-conversation reader needs at-a-glance: tier-in-prose, type
// signature, evokes-as-near-synonyms, decomposition, lineage,
// example, status.
func TestMetaExplanatoryIncludesLoadBearingFields(t *testing.T) {
	out := MetaExplanatory(fixtureMolecule)
	checks := []string{
		"argument-from-expert-opinion",
		"lex-kebfa",
		"named assembly of atoms", // tier-in-prose
		"claim → posture",
		"appeal-to-authority", // evokes
		"lex-q9asc",            // decomposes
		"lex-dm5te",
		"lex-af9ax", // related
		"walton/wrm-2008 ch.3",
		"Dr. X says T helps C",
		"`under-review`",
	}
	for _, want := range checks {
		if !strings.Contains(out.Text, want) {
			t.Errorf("missing %q in output:\n%s", want, out.Text)
		}
	}
}
