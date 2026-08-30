package modes

import (
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

var fixtureMoleculeForIntrospection = &types.LexEntry{
	ID:             "lex-kebfa",
	Name:           "argument-from-expert-opinion",
	TypeIn:         "claim",
	TypeOut:        "posture",
	Tier:           "molecule",
	DecomposesInto: []string{"lex-q9asc", "lex-dm5te"},
	Assembly:       "sequential(lex-q9asc, lex-dm5te) → defeasibility-attach(presumption, defeaters=checklist)",
	CriticalQuestions: []string{
		"is E really an expert?",
		"is A in D?",
	},
	Lineage: []types.LineageEntry{
		{Source: "walton", Text: "wrm-2008", Citation: "ch.3", Quote: "[MISSING: verify before activation]"},
		{Source: "discovery-loop", Text: "asm-001", Citation: "decomposition"},
	},
	CanonicalInstances: []string{"x"},
	Status:             "under-review",
}

var fixtureAtomForIntrospection = &types.LexEntry{
	ID:                 "lex-q9asc",
	Name:               "source-attribution",
	TypeIn:             "claim",
	TypeOut:            "frame",
	Tier:               "atomic",
	Lineage:            []types.LineageEntry{{Source: "walton", Text: "wrm-2008", Citation: "ch.3"}},
	CanonicalInstances: []string{"x"},
	Status:             "active",
}

var introspectionPool = map[string]*types.LexEntry{
	fixtureAtomForIntrospection.ID: fixtureAtomForIntrospection,
}

func TestIntrospectionListsDeployedAtoms(t *testing.T) {
	out := Introspection(fixtureMoleculeForIntrospection, introspectionPool)
	if !strings.Contains(out.Text, "lex-q9asc `source-attribution`") {
		t.Fatalf("expected pool-resolved atom name: %s", out.Text)
	}
	if !strings.Contains(out.Text, "lex-dm5te") {
		t.Fatalf("expected lex-dm5te ID even without pool entry: %s", out.Text)
	}
}

// Critical questions render as checkbox bullets — the format invites
// the reader to actually check them, which is the whole point.
func TestIntrospectionRendersDefeatersAsCheckboxes(t *testing.T) {
	out := Introspection(fixtureMoleculeForIntrospection, introspectionPool)
	if !strings.Contains(out.Text, "- [ ] is E really an expert?") {
		t.Fatalf("expected checkbox defeater: %s", out.Text)
	}
}

// Lineage status is the lever for promote-to-active decisions —
// MISSING quotes must surface explicitly, not silently pass.
func TestIntrospectionFlagsMissingQuotes(t *testing.T) {
	out := Introspection(fixtureMoleculeForIntrospection, introspectionPool)
	if !strings.Contains(out.Text, "quote MISSING") {
		t.Fatalf("expected MISSING-quote flag: %s", out.Text)
	}
	if !strings.Contains(out.Text, "no quote field") {
		t.Fatalf("expected no-quote-field flag: %s", out.Text)
	}
}

// Assembly field surfaces verbatim under its own header — the bond
// structure is what distinguishes a molecule from a flat atom-list,
// so it must be visible in introspection output.
func TestIntrospectionSurfacesAssemblyWhenPresent(t *testing.T) {
	out := Introspection(fixtureMoleculeForIntrospection, introspectionPool)
	if !strings.Contains(out.Text, "## Assembly") {
		t.Fatalf("expected ## Assembly section: %s", out.Text)
	}
	if !strings.Contains(out.Text, "sequential(lex-q9asc, lex-dm5te)") {
		t.Fatalf("expected assembly string surfaced verbatim: %s", out.Text)
	}
}

// Atom path: no assembly field → no Assembly section. Don't surface
// empty headers.
func TestIntrospectionOmitsAssemblyWhenAbsent(t *testing.T) {
	out := Introspection(fixtureAtomForIntrospection, introspectionPool)
	if strings.Contains(out.Text, "## Assembly") {
		t.Fatalf("atom should not get Assembly section: %s", out.Text)
	}
}

func TestIntrospectionSurfacesUnderReviewCaveat(t *testing.T) {
	out := Introspection(fixtureMoleculeForIntrospection, introspectionPool)
	if !strings.Contains(out.Text, "Use provisionally") {
		t.Fatalf("expected under-review caveat: %s", out.Text)
	}
}

// Atom path: no decomposes-into and no critical-questions should
// degrade to informative messages, not crash or render empty headers.
func TestIntrospectionGracefulOnAtom(t *testing.T) {
	out := Introspection(fixtureAtomForIntrospection, introspectionPool)
	if !strings.Contains(out.Text, "no decomposes-into declared") {
		t.Fatalf("expected no-decomposes message: %s", out.Text)
	}
	if !strings.Contains(out.Text, "no critical-questions declared") {
		t.Fatalf("expected no-critical-questions message: %s", out.Text)
	}
	if strings.Contains(out.Text, "Use provisionally") {
		t.Fatalf("active entry should not get under-review caveat: %s", out.Text)
	}
}
