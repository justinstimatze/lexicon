package modes

import (
	"fmt"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// MetaExplanatory renders "what the entry is, where it sits, why it
// matters" — the default mode for design-conversation contexts. Pure
// template, no LLM. Output is markdown; the CLI passes through to
// stdout.
func MetaExplanatory(e *types.LexEntry) types.RenderOutput {
	tierDesc := describeTier(e.Tier)
	relations := describeRelations(e)
	lineage := describeLineage(e)

	example := "(no canonical instance)"
	if len(e.CanonicalInstances) > 0 {
		example = e.CanonicalInstances[0]
	}

	severity := "info"
	if e.SeverityTier != "" {
		severity = e.SeverityTier
	}

	var lines []string
	add := func(s string) {
		if s != "" {
			lines = append(lines, s)
		}
	}
	add(fmt.Sprintf("# %s (%s)", e.Name, e.ID))
	add("")
	add(fmt.Sprintf("**What it is:** %s.", tierDesc))
	add(fmt.Sprintf("**Type signature:** %s → %s", e.TypeIn, e.TypeOut))
	if len(e.Evokes) > 0 {
		add("")
		add(fmt.Sprintf(
			"**Also called (gestural near-synonyms): %s**",
			strings.Join(e.Evokes, ", "),
		))
	}
	add("")
	add(fmt.Sprintf("**Where it sits:**\n%s", relations))
	if e.Tier == "reaction" {
		add("")
		add(describeReaction(e))
	}
	add("")
	add(fmt.Sprintf("**Why this matters:** lineage from %s.", lineage))
	add("")
	add(fmt.Sprintf("**Example:** %s", example))
	add("")
	add(fmt.Sprintf("**Status:** `%s`. Severity: %s.", e.Status, severity))

	return types.RenderOutput{
		PrimitiveID: e.ID,
		Mode:        types.ModeMetaExplanatory,
		Text:        strings.Join(lines, "\n"),
	}
}

func describeTier(tier string) string {
	switch tier {
	case "sub-atomic":
		return "a sub-atomic elements primitive (used internally for paraphrase tests, not user-facing)"
	case "atomic":
		return "an atomic cognitive move (a single typed operation a thinker performs)"
	case "molecule":
		return "a molecule — a named assembly of atoms with established practitioner use"
	case "compound":
		return "a compound — a higher-tier assembly of molecules with its own practitioner identity"
	case "reaction":
		return "a reaction — a transformation (reactants → products via a mechanism) modulated by catalysts that accelerate it and inhibitors that block it"
	default:
		return fmt.Sprintf("a %s-tier primitive", tier)
	}
}

// describeReaction renders the reaction-tier slots as a steering block.
// Slot values that are lex-NNNN ids are printed as-is (consistent with
// describeRelations, which doesn't resolve ids either — MetaExplanatory
// takes no pool). Empty slots are skipped.
func describeReaction(e *types.LexEntry) string {
	var b strings.Builder
	b.WriteString("**As a reaction:**")
	line := func(label string, vals []string) {
		if len(vals) > 0 {
			fmt.Fprintf(&b, "\n- %s: %s", label, strings.Join(vals, "; "))
		}
	}
	if e.Mechanism != "" {
		fmt.Fprintf(&b, "\n- mechanism: %s", e.Mechanism)
	}
	line("reactants", e.Reactants)
	line("products", e.Products)
	line("conditions (must hold to fire)", e.Conditions)
	line("catalysts (accelerate)", e.Catalysts)
	line("inhibitors (block)", e.Inhibitors)
	if e.Reversibility != "" {
		fmt.Fprintf(&b, "\n- reversibility: %s", e.Reversibility)
	}
	return b.String()
}

func describeLineage(e *types.LexEntry) string {
	parts := make([]string, 0, len(e.Lineage))
	for _, l := range e.Lineage {
		verified := ""
		if l.QuoteStaked() {
			verified = " (verified)"
		}
		srcPart := l.Source
		if l.Tradition != "" {
			srcPart = l.Source + "[" + l.Tradition + "]"
		}
		parts = append(parts, fmt.Sprintf("%s/%s %s%s", srcPart, l.Text, l.Citation, verified))
	}
	return strings.Join(parts, "; ")
}

func describeRelations(e *types.LexEntry) string {
	var lines []string
	if len(e.DecomposesInto) > 0 {
		lines = append(lines, "decomposes into: "+strings.Join(e.DecomposesInto, ", "))
	}
	if len(e.Related) > 0 {
		lines = append(lines, "related to: "+strings.Join(e.Related, ", "))
	}
	if len(lines) == 0 {
		return "(no decomposition or relations declared)"
	}
	return strings.Join(lines, "\n")
}
