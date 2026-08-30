package modes

import (
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

var (
	fixtureMoleculeForViz = &types.LexEntry{
		ID:                 "lex-kebfa",
		Name:               "argument-from-expert-opinion",
		TypeIn:             "claim",
		TypeOut:            "posture",
		Tier:               "molecule",
		Related:            []string{"lex-0047"},
		DecomposesInto:     []string{"lex-q9asc", "lex-af9ax"},
		Lineage:            []types.LineageEntry{{Source: "walton", Text: "wrm-2008", Citation: "ch.3"}},
		CanonicalInstances: []string{"x"},
		Status:             "under-review",
	}
	fixtureAtomForViz = &types.LexEntry{
		ID:                 "lex-q9asc",
		Name:               "source-attribution",
		TypeIn:             "claim",
		TypeOut:            "frame",
		Tier:               "atomic",
		Related:            []string{"lex-dm5te"},
		Lineage:            []types.LineageEntry{{Source: "walton", Text: "wrm-2008", Citation: "ch.3"}},
		CanonicalInstances: []string{"x"},
		Status:             "under-review",
	}
	fixturePool = map[string]*types.LexEntry{
		fixtureMoleculeForViz.ID: fixtureMoleculeForViz,
		fixtureAtomForViz.ID:     fixtureAtomForViz,
	}
)

func TestVisualMoleculeUsesFlowchartTD(t *testing.T) {
	out := Visual(fixtureMoleculeForViz, fixturePool)
	if !strings.Contains(out.Text, "flowchart TD") {
		t.Fatalf("missing flowchart TD: %s", out.Text)
	}
	if !strings.Contains(out.Text, "lex-kebfa -->|decomposes| lex-q9asc") {
		t.Fatalf("missing decomposes edge: %s", out.Text)
	}
	if !strings.Contains(out.Text, "lex-kebfa -.->|related| lex-0047") {
		t.Fatalf("missing dotted related edge: %s", out.Text)
	}
}

func TestVisualAtomUsesFlowchartLR(t *testing.T) {
	out := Visual(fixtureAtomForViz, fixturePool)
	if !strings.Contains(out.Text, "flowchart LR") {
		t.Fatalf("missing flowchart LR: %s", out.Text)
	}
}

// Pool lookup populates the child label with name+type-signature.
// Without it, the diagram is ID-only and operationally useless.
func TestVisualLooksUpChildLabels(t *testing.T) {
	out := Visual(fixtureMoleculeForViz, fixturePool)
	if !strings.Contains(out.Text, "source-attribution") || !strings.Contains(out.Text, "claim → frame") {
		t.Fatalf("child label missing: %s", out.Text)
	}
}

// Missing pool entry must degrade gracefully — a (not in pool)
// label is informative; a panic or empty diagram isn't.
func TestVisualHandlesMissingPoolEntry(t *testing.T) {
	out := Visual(fixtureMoleculeForViz, fixturePool)
	if !strings.Contains(out.Text, "(not in pool)") {
		t.Fatalf("expected fallback label for lex-0047: %s", out.Text)
	}
}

func TestVisualWrapsInMermaidFence(t *testing.T) {
	out := Visual(fixtureMoleculeForViz, fixturePool)
	if !strings.Contains(out.Text, "```mermaid") || !strings.HasSuffix(strings.TrimSpace(out.Text), "```") {
		t.Fatalf("mermaid fence wrong: %s", out.Text)
	}
}
