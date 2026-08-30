package assembly

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// AntiPatternMaxN is the largest decomposes-into size we attempt to
// enumerate (n-1)-subsets for. Larger molecules are warn-skipped to
// avoid combinatorial blow-up.
const AntiPatternMaxN = 8

// AntiPattern is one candidate failure-mode surfaced by enumerating
// (n-1)-subsets of a molecule's `decomposes-into:`. The host molecule
// minus one atom names a recognizable cognitive failure when the
// missing piece is the diagnostic.
//
// Example: lex-kebfa (argument-from-expert-opinion) decomposes into
// {lex-q9asc, lex-dm5te, lex-af9ax, lex-th68b}. Removing lex-dm5te
// (domain-credibility) yields the anti-pattern
// "argument-from-expert-without-domain-check" — the failure mode
// composition-operations.md §"Why bonds matter" promises but never
// delivers.
type AntiPattern struct {
	Name             string   // synthesized: <molecule-name>-without-<missing-atom-name>
	ParentID         string   // host molecule's lex-id
	ParentName       string   // host molecule's name
	MissingID        string   // the atom whose absence defines this anti-pattern
	MissingName      string   // its name (or "<unnamed>" if absent from elements)
	RemainingIDs     []string // (n-1) subset
	RemainingNames   []string // names matching RemainingIDs (or id when name unavailable)
	FailureModeTODO  string   // human review needed
	AttestationTODO  string   // human review needed
}

// EnumerateAntiPatterns generates anti-pattern candidates for every
// molecule with len(decomposes-into) ≥ 2 and ≤ AntiPatternMaxN.
// `[MISSING: ...]` placeholders in decomposes-into are skipped.
//
// Output is stable-sorted: by parent ID, then by missing-atom ID.
func EnumerateAntiPatterns(elements map[string]*types.LexEntry) []AntiPattern {
	var ids []string
	for id := range elements {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []AntiPattern
	for _, id := range ids {
		e := elements[id]
		atoms := cleanDecompose(e.DecomposesInto)
		if len(atoms) < 2 || len(atoms) > AntiPatternMaxN {
			continue
		}
		for _, missing := range atoms {
			ap := buildAntiPattern(e, atoms, missing, elements)
			out = append(out, ap)
		}
	}
	return out
}

// cleanDecompose returns decomposes-into entries with [MISSING: ...]
// placeholders and blank entries dropped, in source order.
func cleanDecompose(in []string) []string {
	var out []string
	for _, x := range in {
		x = strings.TrimSpace(x)
		if x == "" || strings.HasPrefix(x, "[MISSING") {
			continue
		}
		out = append(out, x)
	}
	return out
}

func buildAntiPattern(host *types.LexEntry, allAtoms []string, missing string, elements map[string]*types.LexEntry) AntiPattern {
	missingName := atomName(missing, elements)
	remainingIDs := make([]string, 0, len(allAtoms)-1)
	remainingNames := make([]string, 0, len(allAtoms)-1)
	for _, a := range allAtoms {
		if a == missing {
			continue
		}
		remainingIDs = append(remainingIDs, a)
		remainingNames = append(remainingNames, atomName(a, elements))
	}
	return AntiPattern{
		Name:            fmt.Sprintf("%s-without-%s", host.Name, missingName),
		ParentID:        host.ID,
		ParentName:      host.Name,
		MissingID:       missing,
		MissingName:     missingName,
		RemainingIDs:    remainingIDs,
		RemainingNames:  remainingNames,
		FailureModeTODO: "TODO: name the cognitive failure mode",
		AttestationTODO: "TODO: cite ≥1 cross-domain attestation",
	}
}

// atomName looks up an atom's `name:` field; falls back to "<unnamed:id>"
// when the atom isn't in the elements yet.
func atomName(id string, elements map[string]*types.LexEntry) string {
	if e, ok := elements[id]; ok && e.Name != "" {
		return e.Name
	}
	return fmt.Sprintf("<unnamed:%s>", id)
}
