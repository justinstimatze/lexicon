package modes

import (
	"fmt"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Introspection renders the "what's holding this entry together" view:
// atoms-deployed, defeaters-as-checkboxes, lineage-verification status,
// status caveat. Pure template, no LLM. The deployment-mode framing
// ("if you deployed this entry...") makes the output usable as a
// checklist before or after a real deployment.
//
// Distinct from the --why flag: --why appends a brief introspection-
// trace to ANY mode's output (mode chosen, classifier branch, token
// counts). Introspection-mode is a first-class, fuller render that
// stands on its own.
func Introspection(e *types.LexEntry, pool map[string]*types.LexEntry) types.RenderOutput {
	var lines []string
	add := func(s string) { lines = append(lines, s) }

	add(fmt.Sprintf("# %s %s — introspection", e.ID, e.Name))
	add("")
	add("**If you deployed this entry, here's what fires and what's holding it together.**")
	add("")
	add(fmt.Sprintf(
		"**Tier:** %s. **Status:** `%s`. **Type signature:** %s → %s.",
		e.Tier, e.Status, e.TypeIn, e.TypeOut,
	))
	add("")

	if len(e.DecomposesInto) > 0 {
		add("## Atoms this molecule deploys")
		add("")
		for _, childID := range e.DecomposesInto {
			if !strings.HasPrefix(childID, "lex-") {
				add("- " + childID)
				continue
			}
			child, ok := pool[childID]
			if ok {
				add(fmt.Sprintf(
					"- %s `%s` (%s → %s)",
					childID, child.Name, child.TypeIn, child.TypeOut,
				))
			} else {
				add(fmt.Sprintf("- %s (not loaded — verify against elements)", childID))
			}
		}
		add("")
		if e.Assembly != "" {
			add("## Assembly")
			add("")
			add("*How the atoms above bond into this molecule (per `composition-operations.md`):*")
			add("")
			add("```")
			add(e.Assembly)
			add("```")
			add("")
		}
	} else {
		add("## Atoms deployed")
		add("")
		add("(no decomposes-into declared — this is an atomic move or the field is missing)")
		add("")
	}

	if len(e.CriticalQuestions) > 0 {
		add("## Defeaters you should be holding ready")
		add("")
		add("*If you deploy this entry without considering these, you have under-supported it.*")
		add("")
		for _, q := range e.CriticalQuestions {
			add("- [ ] " + q)
		}
		add("")
	} else {
		add("## Defeaters")
		add("")
		add("(no critical-questions declared — this entry has no explicit defeater set; consider adding one if it would improve operational use)")
		add("")
	}

	add("## Lineage verification status")
	add("")
	for _, l := range e.Lineage {
		add("- " + describeLineageItem(l))
	}
	add("")

	if e.Status == "under-review" {
		add("## Status caveat")
		add("")
		add("Entry is `under-review`. Use provisionally — promote to `active` only after lineage verification per `verification-pass-1.md` discipline.")
	}

	return types.RenderOutput{
		PrimitiveID: e.ID,
		Mode:        types.ModeIntrospection,
		Text:        strings.Join(lines, "\n"),
	}
}

func describeLineageItem(l types.LineageEntry) string {
	srcPart := l.Source
	if l.Tradition != "" {
		srcPart = l.Source + "[" + l.Tradition + "]"
	}
	cite := fmt.Sprintf("%s/%s %s", srcPart, l.Text, l.Citation)
	switch {
	case strings.TrimSpace(l.Quote) == "":
		return cite + " — no quote field (provenance only, not mirror-source-committed)"
	case !l.QuoteStaked():
		return cite + " — quote MISSING (not yet verified)"
	default:
		return cite + " — verified (quote present)"
	}
}
