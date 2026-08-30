package main

// taxonomyCheck validates each entry's small-enum fields (_tier, status,
// severity-tier, type-in, type-out) against the canonical vocabularies.
// Hand-fixed instances drift back over time when new mining passes invent
// adjacent values (`atom` vs `atomic`, `refs-grounded` vs `active`,
// `information` vs `info`, free-form type-outs like `administrable-
// uniform-schema`). This check makes the rule a gate, not a convention.

import (
	"fmt"

	"github.com/justinstimatze/lexicon/render/internal/assembly"
	"github.com/justinstimatze/lexicon/render/internal/types"
	"github.com/justinstimatze/lexicon/render/internal/viz"
)

var (
	validTiers         = map[string]bool{"atomic": true, "molecule": true, "reaction": true}
	validStatuses      = map[string]bool{"active": true, "under-review": true}
	validSeverityTiers = map[string]bool{"": true, "info": true, "warning": true, "critical": true}
	validLineageSrcs   = map[string]bool{
		"primary":           true,
		"practitioner":      true,
		"discovery-loop":    true,
		"cross-attestation": true,
		"secondary":         true,
	}
)

func taxonomyCheck(e *types.LexEntry) []assembly.Diagnostic {
	if e == nil {
		return nil
	}
	var out []assembly.Diagnostic
	emit := func(code, msg string) {
		out = append(out, assembly.Diagnostic{
			Severity: "error",
			Code:     code,
			Message:  msg,
			EntryID:  e.ID,
			Pos:      -1,
		})
	}
	if !validTiers[e.Tier] {
		emit("invalid-tier", fmt.Sprintf("_tier=%q not in {atomic, molecule, reaction}", e.Tier))
	}
	if !validStatuses[e.Status] {
		emit("invalid-status", fmt.Sprintf("status=%q not in {active, under-review}", e.Status))
	}
	if !validSeverityTiers[e.SeverityTier] {
		emit("invalid-severity-tier", fmt.Sprintf("severity-tier=%q not in {info, warning, critical}", e.SeverityTier))
	}
	if !inSlice(e.TypeIn, viz.PivotRowOrder) {
		emit("invalid-type-in", fmt.Sprintf("type-in=%q not in PivotRowOrder %v", e.TypeIn, viz.PivotRowOrder))
	}
	if !inSlice(e.TypeOut, viz.PivotColOrder) {
		emit("invalid-type-out", fmt.Sprintf("type-out=%q not in PivotColOrder %v", e.TypeOut, viz.PivotColOrder))
	}
	for i, l := range e.Lineage {
		if !validLineageSrcs[l.Source] {
			emit("invalid-lineage-source", fmt.Sprintf("lineage[%d].source=%q not in {primary, practitioner, discovery-loop, cross-attestation, secondary} — tradition shorthand goes in lineage[].tradition instead", i, l.Source))
		}
	}
	return out
}

func inSlice(s string, xs []string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
