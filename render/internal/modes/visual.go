package modes

import (
	"fmt"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Visual renders a mermaid flowchart for the entry. No LLM call.
// Molecules use flowchart TD with decomposes-into edges; atoms use
// flowchart LR with related-as-siblings.
//
// pool is optional — if nil or missing the related entry, the renderer
// falls back to "(not in pool)" rather than failing. Callers that
// want full child labels should pass loader.LoadAll() output.
func Visual(e *types.LexEntry, pool map[string]*types.LexEntry) types.RenderOutput {
	var graph string
	switch e.Tier {
	case "reaction":
		graph = renderReactionGraph(e, pool)
	case "molecule", "compound":
		graph = renderMoleculeGraph(e, pool)
	default:
		graph = renderAtomGraph(e, pool)
	}
	text := fmt.Sprintf(
		"# %s %s — visual (%s)\n\n```mermaid\n%s\n```",
		e.ID, e.Name, e.Tier, graph,
	)
	return types.RenderOutput{
		PrimitiveID: e.ID,
		Mode:        types.ModeVisual,
		Text:        text,
	}
}

func renderMoleculeGraph(e *types.LexEntry, pool map[string]*types.LexEntry) string {
	lines := []string{"flowchart TD"}
	lines = append(lines, fmt.Sprintf("    %s[%q]", e.ID, nodeLabel(e)))

	decompSet := map[string]bool{}
	for _, childID := range e.DecomposesInto {
		if !strings.HasPrefix(childID, "lex-") {
			continue
		}
		decompSet[childID] = true
		lines = append(lines, fmt.Sprintf("    %s[%q]", childID, lookupLabel(childID, pool)))
		lines = append(lines, fmt.Sprintf("    %s -->|decomposes| %s", e.ID, childID))
	}
	for _, relID := range e.Related {
		if decompSet[relID] {
			continue
		}
		lines = append(lines, fmt.Sprintf("    %s[%q]", relID, lookupLabel(relID, pool)))
		lines = append(lines, fmt.Sprintf("    %s -.->|related| %s", e.ID, relID))
	}
	return strings.Join(lines, "\n")
}

// renderReactionGraph draws the transformation: reactants --> [reaction]
// --> products, with catalysts (accelerate) and inhibitors (block) as
// dotted modulator edges into the reaction node. lex-NNNN slot entries
// get looked up in the pool; free-text slot entries render as their own
// synthetic-id nodes.
func renderReactionGraph(e *types.LexEntry, pool map[string]*types.LexEntry) string {
	lines := []string{"flowchart LR", fmt.Sprintf("    %s[%q]", e.ID, nodeLabel(e))}
	edge := func(prefix string, i int, v, fromTo string, rel string) {
		nid := slotNodeID(prefix, i, v)
		lines = append(lines, fmt.Sprintf("    %s[%q]", nid, slotLabel(v, pool)))
		switch fromTo {
		case "in":
			lines = append(lines, fmt.Sprintf("    %s --> %s", nid, e.ID))
		case "out":
			lines = append(lines, fmt.Sprintf("    %s --> %s", e.ID, nid))
		case "mod":
			lines = append(lines, fmt.Sprintf("    %s -.->|%s| %s", nid, rel, e.ID))
		}
	}
	for i, r := range e.Reactants {
		edge("react", i, r, "in", "")
	}
	for i, p := range e.Products {
		edge("prod", i, p, "out", "")
	}
	for i, c := range e.Catalysts {
		edge("cat", i, c, "mod", "catalyzes")
	}
	for i, inh := range e.Inhibitors {
		edge("inh", i, inh, "mod", "inhibits")
	}
	return strings.Join(lines, "\n")
}

// slotNodeID returns a mermaid-safe node id for a reaction-slot value:
// the lex-id itself when the value is a bare lex-NNNN (so shared atoms
// merge), else a synthetic prefix+index id for free-text slots.
func slotNodeID(prefix string, i int, v string) string {
	if isLexID(v) {
		return v
	}
	return fmt.Sprintf("%s%d", prefix, i)
}

// slotLabel renders a reaction-slot value's node label: the looked-up
// name for lex-ids, else the (truncated) free text.
func slotLabel(v string, pool map[string]*types.LexEntry) string {
	if isLexID(v) {
		return lookupLabel(v, pool)
	}
	if len(v) > 80 {
		v = v[:79] + "…"
	}
	return sanitizeLabel(v)
}

func isLexID(s string) bool {
	return strings.HasPrefix(s, "lex-") && !strings.ContainsAny(s, " \t")
}

func renderAtomGraph(e *types.LexEntry, pool map[string]*types.LexEntry) string {
	lines := []string{"flowchart LR"}
	lines = append(lines, fmt.Sprintf("    %s[%q]", e.ID, nodeLabel(e)))
	for _, relID := range e.Related {
		lines = append(lines, fmt.Sprintf("    %s[%q]", relID, lookupLabel(relID, pool)))
		lines = append(lines, fmt.Sprintf("    %s --- %s", e.ID, relID))
	}
	return strings.Join(lines, "\n")
}

func nodeLabel(e *types.LexEntry) string {
	return sanitizeLabel(fmt.Sprintf("%s<br/>%s → %s<br/>(%s)", e.Name, e.TypeIn, e.TypeOut, e.Tier))
}

func lookupLabel(id string, pool map[string]*types.LexEntry) string {
	if pool == nil {
		return sanitizeLabel(fmt.Sprintf("%s<br/>(not in pool)", id))
	}
	entry, ok := pool[id]
	if !ok {
		return sanitizeLabel(fmt.Sprintf("%s<br/>(not in pool)", id))
	}
	return sanitizeLabel(fmt.Sprintf("%s<br/>%s → %s", entry.Name, entry.TypeIn, entry.TypeOut))
}

func sanitizeLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
