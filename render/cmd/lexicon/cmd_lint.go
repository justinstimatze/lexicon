package main

// `lexicon lint` — type-check every molecule's `assembly:` field against
// the elements' typed primitives. Surfaces:
//
//   - error: type-mismatch in sequential / parallel / choice
//   - warning: unresolvable-atom (lex-NNNN not in elements, or bare-name
//             atom without an id) — forcing function for next mining pass
//   - warning: iteration arg without fixed-point shape
//   - info:    decomposes-into entries not referenced in assembly
//
// Exit code: non-zero if any errors. Used by CI/lint hooks.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/assembly"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func cmdLint(renderDir string, args []string) {
	fl := flag.NewFlagSet("lint", flag.ExitOnError)
	verbose := fl.Bool("v", false, "include info-level diagnostics in output")
	jsonOut := fl.Bool("json", false, "emit JSONL diagnostics, one per line")
	fl.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lexicon lint [flags] [<lex-id> ...]")
		fmt.Fprintln(os.Stderr, "  Type-check assembly: fields against the elements' type-in/type-out signatures.")
		fmt.Fprintln(os.Stderr, "  Without arguments, lints every entry with an assembly field.")
		fmt.Fprintln(os.Stderr, "Flags:")
		fl.PrintDefaults()
	}
	_ = fl.Parse(args)

	elementsDir := filepath.Join(renderDir, "..", "elements")
	sub, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("load elements: %v", err)
	}

	var ids []string
	if fl.NArg() > 0 {
		ids = fl.Args()
		for _, id := range ids {
			if _, ok := sub[id]; !ok {
				fatal("no entry %s in elements", id)
			}
		}
	} else {
		for id := range sub {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}

	var allDiags []assembly.Diagnostic
	checked := 0
	withAssembly := 0
	for _, id := range ids {
		e := sub[id]
		checked++
		if e.Assembly == "" {
			continue
		}
		withAssembly++
		node, perr := assembly.Parse(e.Assembly)
		if perr != nil {
			allDiags = append(allDiags, assembly.Diagnostic{
				Severity: "error",
				Code:     "parse-error",
				Message:  perr.Error(),
				EntryID:  id,
				Pos:      -1,
			})
			continue
		}
		allDiags = append(allDiags, assembly.TypeCheck(node, e, sub)...)
		allDiags = append(allDiags, decomposeConsistencyCheck(e, node)...)
	}

	// Cross-citation checks run over every atom (not just those with
	// assembly fields). danglingCitationCheck catches any prose mention
	// of a lex-NNNN that no longer resolves at all; crossCitationCheck
	// catches the narrower case of a name-claim that's gone stale on a
	// still-live atom.
	for _, id := range ids {
		allDiags = append(allDiags, danglingCitationCheck(sub[id], sub)...)
		allDiags = append(allDiags, crossCitationCheck(sub[id], sub)...)
	}

	// Taxonomy check: enforce the small-enum vocabularies for _tier,
	// status, severity-tier, type-in, type-out. Errors block commits.
	for _, id := range ids {
		allDiags = append(allDiags, taxonomyCheck(sub[id])...)
	}

	// Related-existence check: every lex-NNNN in related[] must resolve
	// to an existing atom. Errors block commits — prevents hallucinated
	// IDs (V120 r: lex-0007 written when Klein pre-mortem is lex-eb3wf)
	// from landing.
	for _, id := range ids {
		allDiags = append(allDiags, relatedExistenceCheck(sub[id], sub)...)
	}

	// Scaffolds-from check: dangling refs (warning), self-reference
	// (error), cycles (info — mutually-priming pairs are meaningful),
	// same-tradition same-tier edges (info — probably belong in
	// related[]). See docs/principles/scaffolds-from-design.md.
	for _, id := range ids {
		allDiags = append(allDiags, scaffoldsFromCheck(sub[id], sub)...)
	}
	allDiags = append(allDiags, scaffoldsFromCycleCheck(sub, ids)...)

	// Name-length check: warn when a name's word count suggests it may
	// carry more than the headline claim. Forward-looking gate, not a
	// retroactive grind — see nameLengthCheck doc comment.
	for _, id := range ids {
		allDiags = append(allDiags, nameLengthCheck(sub[id])...)
	}

	emitLintDiagnostics(allDiags, ids, *verbose, *jsonOut)

	var errs, warns, infos int
	for _, d := range allDiags {
		switch d.Severity {
		case "error":
			errs++
		case "warning":
			warns++
		case "info":
			infos++
		}
	}
	fmt.Fprintf(os.Stderr, "\nlint: %d entries, %d with assembly; %d error(s), %d warning(s), %d info\n",
		checked, withAssembly, errs, warns, infos)
	if errs > 0 {
		os.Exit(1)
	}
}

func emitLintDiagnostics(diags []assembly.Diagnostic, idOrder []string, verbose, asJSON bool) {
	byID := map[string][]assembly.Diagnostic{}
	for _, d := range diags {
		if !verbose && d.Severity == "info" {
			continue
		}
		byID[d.EntryID] = append(byID[d.EntryID], d)
	}
	for _, id := range idOrder {
		ds := byID[id]
		if len(ds) == 0 {
			continue
		}
		for _, d := range ds {
			if asJSON {
				fmt.Printf("{\"id\":%q,\"severity\":%q,\"code\":%q,\"message\":%q,\"pos\":%d}\n",
					d.EntryID, d.Severity, d.Code, d.Message, d.Pos)
				continue
			}
			fmt.Printf("%s\t%-7s\t%s\t%s\n", id, d.Severity, d.Code, d.Message)
		}
	}
}

// decomposeConsistencyCheck flags atoms referenced in `assembly:` but
// missing from `decomposes-into:` (warning), and atoms listed in
// `decomposes-into:` but not referenced in the parsed assembly (info).
// `[MISSING: ...]` placeholders in decomposes-into are tolerated.
func decomposeConsistencyCheck(e *types.LexEntry, n assembly.Node) []assembly.Diagnostic {
	inAssembly := map[string]bool{}
	assembly.CollectAtomIDs(n, inAssembly)

	inDecompose := map[string]bool{}
	for _, x := range e.DecomposesInto {
		x = strings.TrimSpace(x)
		if strings.HasPrefix(x, "[MISSING") || x == "" {
			continue
		}
		inDecompose[x] = true
	}

	var out []assembly.Diagnostic
	missingFromDecomp := sortedKeysIfMissing(inAssembly, inDecompose)
	for _, id := range missingFromDecomp {
		out = append(out, assembly.Diagnostic{
			Severity: "warning",
			Code:     "assembly-not-in-decomposes-into",
			Message:  fmt.Sprintf("assembly references %s but it's not listed in decomposes-into", id),
			EntryID:  e.ID,
			Pos:      -1,
		})
	}
	missingFromAssembly := sortedKeysIfMissing(inDecompose, inAssembly)
	for _, id := range missingFromAssembly {
		out = append(out, assembly.Diagnostic{
			Severity: "info",
			Code:     "decomposes-into-not-in-assembly",
			Message:  fmt.Sprintf("decomposes-into lists %s but it's not referenced in assembly", id),
			EntryID:  e.ID,
			Pos:      -1,
		})
	}
	return out
}

// crossCitationRe matches `lex-NNNN <kebab-name>` in prose. We require
// the captured kebab to have at least two hyphens (filtered below) so
// that incidental verb-form follow-ups ("lex-4snxu describes", "lex-utuy6
// generalizes") are not flagged as cross-citations.
// \b-anchored (migration 2026-08-20): see db/sync.go's proseLexRefRe
// comment — the new alphabet spans most of a-z, so an unanchored match
// false-positives inside compound words ("complex-systems" contains the
// literal substring "lex-syste").
var crossCitationRe = regexp.MustCompile(`\blex-[23456789abcdefghjkmnpqrstuvwxyz]{5}\b\s+([a-z][a-z0-9]+(?:-[a-z0-9]+){2,})`)

// crossCitationCheck scans an atom's text fields for prose of the form
// `lex-NNNN <kebab-name>` and checks the reference two ways: if lex-NNNN
// no longer resolves to any atom (renamed-and-old-id-retired, merged
// away, or never existed), that's a dangling-cross-citation error; if it
// resolves but <kebab-name> no longer matches the atom's current name,
// that's a stale-cross-citation warning. Surfaces failure modes the rest
// of the gate stack (drift/lint-typecheck/reciprocation) doesn't catch,
// since prose citations live outside related[]/assembly[] and outside
// the YAML/MD drift-mirror surface.

// relatedExistenceCheck flags any lex-NNNN in `related:` that doesn't
// resolve to an actual elements atom. Promoted to error severity
// because the failure mode it catches — hallucinated IDs written from
// memory rather than verified — leaves broken edges in the elements
// graph that downstream tooling (probe, extrapolate, viz) silently
// works around. The drift gate doesn't catch this (related[] IDs
// aren't part of the YAML/MD drift surface) and the assembly type-
// check only sees IDs inside assembly:.
func relatedExistenceCheck(e *types.LexEntry, sub map[string]*types.LexEntry) []assembly.Diagnostic {
	if e == nil {
		return nil
	}
	var out []assembly.Diagnostic
	for _, ref := range e.Related {
		if _, ok := sub[ref]; !ok {
			out = append(out, assembly.Diagnostic{
				Severity: "error",
				Code:     "related-unresolved",
				Message:  fmt.Sprintf("related[] references %s but it's not in the elements", ref),
				EntryID:  e.ID,
				Pos:      -1,
			})
		}
	}
	return out
}

// crossCitationProseFields returns the text fields both crossCitationCheck
// and danglingCitationCheck scan for lex-NNNN mentions: everywhere a prose
// citation can appear, but outside related[]/assembly[] (which have their
// own existence checks) and outside the YAML/MD drift-mirror surface.
func crossCitationProseFields(e *types.LexEntry) []string {
	var fields []string
	for _, l := range e.Lineage {
		fields = append(fields, l.Citation, l.Quote)
	}
	fields = append(fields, e.CanonicalInstances...)
	fields = append(fields, e.CriticalQuestions...)
	fields = append(fields, e.Mechanism)
	fields = append(fields, e.Reactants...)
	fields = append(fields, e.Products...)
	fields = append(fields, e.Catalysts...)
	fields = append(fields, e.Inhibitors...)
	fields = append(fields, e.Conditions...)
	return fields
}

// bareLexIDRe matches any lex-NNNN mention, independent of whether a
// name-claim follows — deliberately broader than crossCitationRe so it
// also catches bare-ID references ("lex-0046 slippery-slope" — a single
// hyphen, below crossCitationRe's 2-hyphen name-claim threshold),
// comma-separated ID lists ("lex-kebfa, lex-0045, lex-0047"), and
// function-call-style references ("iteration(lex-0057)").
var bareLexIDRe = regexp.MustCompile(`\blex-[23456789abcdefghjkmnpqrstuvwxyz]{5}\b`)

// danglingCitationCheck flags any lex-NNNN mention in prose (regardless
// of surrounding shape) whose ID doesn't resolve to a live atom. This is
// the general case; crossCitationCheck's dangling branch below only
// catches the subset shaped like a name-claim. An atom that's renamed,
// merged away, or retired leaves its old ID orphaned in whichever prose
// still names it, in ANY shape, and none of that rot is caught by
// drift/lint-typecheck/reciprocation.
func danglingCitationCheck(e *types.LexEntry, sub map[string]*types.LexEntry) []assembly.Diagnostic {
	if e == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []assembly.Diagnostic
	for _, text := range crossCitationProseFields(e) {
		if text == "" {
			continue
		}
		for _, id := range bareLexIDRe.FindAllString(text, -1) {
			if _, ok := sub[id]; ok {
				continue
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, assembly.Diagnostic{
				Severity: "error",
				Code:     "dangling-cross-citation",
				Message:  fmt.Sprintf("prose cites %s but it is not in the elements (renamed, merged, or retired?)", id),
				EntryID:  e.ID,
				Pos:      -1,
			})
		}
	}
	return out
}

func crossCitationCheck(e *types.LexEntry, sub map[string]*types.LexEntry) []assembly.Diagnostic {
	if e == nil {
		return nil
	}
	fields := crossCitationProseFields(e)

	seen := map[string]bool{}
	var out []assembly.Diagnostic
	for _, text := range fields {
		if text == "" {
			continue
		}
		for _, m := range crossCitationRe.FindAllStringSubmatchIndex(text, -1) {
			id := text[m[0] : m[0]+8] // "lex-NNNN"
			kebab := text[m[2]:m[3]]
			target, ok := sub[id]
			if !ok {
				continue // handled by danglingCitationCheck
			}
			if kebabStartsWithStopWord(kebab) {
				continue // descriptive context, not a name claim
			}
			if kebabMatchesName(kebab, target.Name) {
				continue
			}
			if kebabMatchesEvokes(kebab, target.Evokes) {
				continue
			}
			key := id + "/" + kebab
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, assembly.Diagnostic{
				Severity: "warning",
				Code:     "stale-cross-citation",
				Message:  fmt.Sprintf("prose names %s as %q but current name is %q", id, kebab, target.Name),
				EntryID:  e.ID,
				Pos:      -1,
			})
		}
	}
	return out
}

// kebabStartsWithStopWord filters kebab-chunks that are descriptive
// context ("across-research-programs", "within-research-investigation",
// "as-a-special-case") rather than a name claim. Names start with nouns
// or noun-like adjectives; prepositions/articles/conjunctions at position
// zero signal the chunk is following lex-id with prose, not an alleged
// name. Keeps the linter's precision usable without losing real renames.
func kebabStartsWithStopWord(kebab string) bool {
	first := kebab
	if i := strings.IndexByte(kebab, '-'); i >= 0 {
		first = kebab[:i]
	}
	switch first {
	case "across", "within", "between", "under", "over", "above", "below",
		"as", "the", "a", "an", "of", "in", "on", "at", "by", "for",
		"with", "without", "into", "onto", "via", "and", "or", "but",
		"so", "if", "when", "while", "during", "after", "before",
		"co", "vs", "is", "are", "was", "were", "be", "to":
		return true
	}
	return false
}

// kebabMatchesName returns true when the kebab follow-up after a lex-id
// plausibly refers to the atom's current name. We accept either (a) the
// kebab appearing as a substring of the current name, or (b) the kebab's
// first 3 segments matching the name's first 3 segments — covering both
// "full name copied" and "first chunk of name as shorthand."
func kebabMatchesName(kebab, name string) bool {
	k := strings.ToLower(kebab)
	n := strings.ToLower(name)
	if strings.Contains(n, k) {
		return true
	}
	kSeg := strings.Split(k, "-")
	nSeg := strings.Split(n, "-")
	prefix := 3
	if len(kSeg) < prefix {
		prefix = len(kSeg)
	}
	if len(nSeg) < prefix {
		return false
	}
	for i := 0; i < prefix; i++ {
		if kSeg[i] != nSeg[i] {
			return false
		}
	}
	return true
}

// kebabMatchesEvokes returns true when the kebab is an exact (case-
// insensitive) match against any entry in the target atom's `evokes`
// list. Per SCHEMA.md, evokes is the "gestural near-synonymous handle"
// field — readable shortforms a writer might naturally reach for. Lets
// prose like `lex-2rpxq hedgehog-vs-fox` resolve against the canonical
// long-form name without forcing the prose to spell out the full name.
func kebabMatchesEvokes(kebab string, evokes []string) bool {
	k := strings.ToLower(kebab)
	for _, e := range evokes {
		if strings.ToLower(e) == k {
			return true
		}
	}
	return false
}

// sortedKeysIfMissing returns the keys of `from` that are absent from
// `target`, in sorted order so diagnostic output is stable.
func sortedKeysIfMissing(from, target map[string]bool) []string {
	var out []string
	for k := range from {
		if !target[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// scaffoldsFromCheck validates the per-atom invariants for
// `scaffolds-from`:
//   - error:   self-reference (an atom listing itself is a structural bug)
//   - warning: dangling reference (target atom not in elements)
//   - info:    same-tradition + same-derived-tier targets (probably belong
//             in related[] — symmetric clustering — not in the directed
//             pedagogical-scaffolding field)
//
// Cycles are checked separately by scaffoldsFromCycleCheck (cross-atom
// pass). Cycles emit info, not warning — mutually-priming pairs are
// meaningful per the V118 at panel refinement, not a violation.
func scaffoldsFromCheck(e *types.LexEntry, sub map[string]*types.LexEntry) []assembly.Diagnostic {
	if e == nil || len(e.ScaffoldsFrom) == 0 {
		return nil
	}
	var out []assembly.Diagnostic
	for _, target := range e.ScaffoldsFrom {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if target == e.ID {
			out = append(out, assembly.Diagnostic{
				Severity: "error",
				Code:     "scaffolds-from-self-reference",
				Message:  fmt.Sprintf("scaffolds-from cannot contain self (%s)", e.ID),
				EntryID:  e.ID,
				Pos:      -1,
			})
			continue
		}
		t, ok := sub[target]
		if !ok {
			out = append(out, assembly.Diagnostic{
				Severity: "warning",
				Code:     "scaffolds-from-dangling-ref",
				Message:  fmt.Sprintf("scaffolds-from references %s which is not in elements", target),
				EntryID:  e.ID,
				Pos:      -1,
			})
			continue
		}
		if e.Tier == t.Tier && primaryTradition(e) != "" && primaryTradition(e) == primaryTradition(t) {
			out = append(out, assembly.Diagnostic{
				Severity: "info",
				Code:     "scaffolds-from-same-tradition-same-tier",
				Message:  fmt.Sprintf("scaffolds-from %s is same tradition (%s) and tier (%s) — consider related[] instead", target, primaryTradition(e), e.Tier),
				EntryID:  e.ID,
				Pos:      -1,
			})
		}
	}
	return out
}

// scaffoldsFromCycleCheck does a global cycle scan over the
// scaffolds-from edges and emits info for each cycle found. Mutually-
// priming pairs (A scaffolds B, B scaffolds A) are surfaced but not
// flagged as errors per V118 at panel refinement — directed but cycles
// ARE allowed. Iterative DFS to avoid stack issues on a deeply-linked
// elements.
func scaffoldsFromCycleCheck(sub map[string]*types.LexEntry, ids []string) []assembly.Diagnostic {
	const (
		unvisited = 0
		onStack   = 1
		done      = 2
	)
	state := map[string]int{}
	for _, id := range ids {
		state[id] = unvisited
	}
	seenCycle := map[string]bool{}
	var out []assembly.Diagnostic
	for _, root := range ids {
		if state[root] != unvisited {
			continue
		}
		// Iterative DFS with explicit stack of (id, childIdx).
		type frame struct {
			id    string
			child int
		}
		stk := []frame{{id: root, child: 0}}
		state[root] = onStack
		path := []string{root}
		for len(stk) > 0 {
			top := &stk[len(stk)-1]
			e := sub[top.id]
			if e == nil || top.child >= len(e.ScaffoldsFrom) {
				state[top.id] = done
				if len(path) > 0 {
					path = path[:len(path)-1]
				}
				stk = stk[:len(stk)-1]
				continue
			}
			next := strings.TrimSpace(e.ScaffoldsFrom[top.child])
			top.child++
			if next == "" || sub[next] == nil {
				continue
			}
			switch state[next] {
			case onStack:
				// Found a cycle — extract the chain from the path.
				cycle := []string{next}
				for i := len(path) - 1; i >= 0; i-- {
					cycle = append(cycle, path[i])
					if path[i] == next {
						break
					}
				}
				// Canonicalize the cycle so we don't double-report (sort + join).
				sorted := append([]string(nil), cycle...)
				sort.Strings(sorted)
				key := strings.Join(sorted, "|")
				if !seenCycle[key] {
					seenCycle[key] = true
					out = append(out, assembly.Diagnostic{
						Severity: "info",
						Code:     "scaffolds-from-cycle",
						Message:  fmt.Sprintf("mutually-priming cycle: %s", strings.Join(cycle, " ← ")),
						EntryID:  next,
						Pos:      -1,
					})
				}
			case unvisited:
				state[next] = onStack
				path = append(path, next)
				stk = append(stk, frame{id: next, child: 0})
			}
		}
	}
	return out
}

// nameLengthThreshold is the word count above which a name gets flagged
// for review. Not a hard cap — some headline claims are legitimately
// this long (e.g. Buber's I-Thou/I-It distinction, Wittgenstein's family
// resemblance both run 14-15 words and carry no padding once reviewed).
// The threshold exists to catch the OTHER failure mode: a qualifier or
// caveat that should live in agent-instruction got folded into the name
// instead. Per feedback_verbose_name_tendency — the mint-time tendency
// is to draft long and need a tightening pass before shipping; this is
// that pass, automated, so it runs on every future mint instead of only
// when someone happens to notice (V121 d, user: "you do it all the time").
const nameLengthThreshold = 12

func nameLengthCheck(e *types.LexEntry) []assembly.Diagnostic {
	if e == nil || e.Name == "" {
		return nil
	}
	words := strings.Split(e.Name, "-")
	if len(words) <= nameLengthThreshold {
		return nil
	}
	return []assembly.Diagnostic{{
		Severity: "warning",
		Code:     "name-overlong",
		Message: fmt.Sprintf("name is %d words (>%d) — review for a qualifier/caveat that belongs in agent-instruction instead",
			len(words), nameLengthThreshold),
		EntryID: e.ID,
		Pos:     -1,
	}}
}

// primaryTradition returns the lineage[0].Tradition string or "" if
// absent. Used by the same-tradition same-tier scaffolds-from check.
func primaryTradition(e *types.LexEntry) string {
	if len(e.Lineage) == 0 {
		return ""
	}
	return strings.TrimSpace(e.Lineage[0].Tradition)
}
