package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// cmdAnki: export the elements as an Anki-importable TSV deck.
//
// Two cards per atom, not one — the minimum-information principle
// (Wozniak's 20 rules; see controlaltbackspace.org/precise) is explicit
// that a card testing more than one fact is a card nobody reviews
// consistently. A single card cramming a citation, an example, and an
// operational rule together is exactly the anti-pattern the principle
// names.
//
//   - Recognition card: front is a concrete scenario (one canonical
//     instance, entries that reference other lex- ids skipped — those
//     presuppose the very recognition this card is testing), back is
//     the pattern's name. Scenario-to-name, not "give an example of X" —
//     the latter has infinitely many valid answers and can't be graded
//     consistently on review.
//   - Recall card: front is the pattern's name plus its type signature
//     (context stated upfront, not implied), back is the single
//     imperative agent-instruction plus a short source attribution line.
//     Nothing else — no canonical-instances, no critical-questions list,
//     no related[] dump. Those are real content but belong in the
//     source atom, not crammed onto a card meant to test one thing.
//
// An atom missing agent-instruction skips its recall card; an atom with
// no standalone (non-comparative) canonical-instance skips its
// recognition card. Neither is required to have both.
//
// Usage:
//
//	lexicon anki                       # write to stdout
//	lexicon anki --out lexicon.tsv     # write to file
//	lexicon anki --tier primitive      # filter by _tier
//	lexicon anki --status active       # filter by status
//	lexicon anki --lint                # report card-quality findings instead of writing the deck
func cmdAnki(renderDir string, args []string) {
	fl := flag.NewFlagSet("anki", flag.ExitOnError)
	outPath := fl.String("out", "", "output file path (default: stdout)")
	tierFilter := fl.String("tier", "", "filter by _tier (e.g. primitive)")
	statusFilter := fl.String("status", "", "filter by status (e.g. active)")
	lint := fl.Bool("lint", false, "report card-quality findings against SuperMemo's Twenty Rules instead of writing the deck")
	_ = fl.Parse(args)

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, "..", "elements")
		if _, err := os.Stat(elementsDir); err != nil {
			elementsDir = filepath.Join(renderDir, "elements")
		}
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("anki: load elements: %v", err)
	}

	entries := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		if *tierFilter != "" && e.Tier != *tierFilter {
			continue
		}
		if *statusFilter != "" && e.Status != *statusFilter {
			continue
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	if *lint {
		runAnkiLint(entries)
		return
	}

	w := os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fatal("anki: create %s: %v", *outPath, err)
		}
		defer f.Close()
		w = f
	}

	fmt.Fprintln(w, "#separator:tab")
	fmt.Fprintln(w, "#html:true")
	fmt.Fprintln(w, "#columns:Front\tBack\tTags")

	recognition, recall := 0, 0
	for _, e := range entries {
		tags := strings.Join([]string{
			"lexicon",
			"tier-" + e.Tier,
			"status-" + e.Status,
			"type-" + e.TypeIn + "-" + e.TypeOut,
		}, " ")

		if scenario := recognitionScenario(e); scenario != "" {
			front := fmt.Sprintf("<i>What pattern is this?</i><br><br>%s", htmlEscape(ankiTruncate(scenario, 380)))
			back := fmt.Sprintf("<b>%s</b>", htmlEscape(displayName(e)))
			fmt.Fprintf(w, "%s\t%s\t%s recognition\n", front, back, tags)
			recognition++
		}

		if e.AgentInstruction != "" {
			front := fmt.Sprintf("<b>%s</b> <i>(%s → %s)</i><br><br>When this pattern is firing, what do you do?",
				htmlEscape(displayName(e)), htmlEscape(e.TypeIn), htmlEscape(e.TypeOut))
			back := htmlEscape(e.AgentInstruction)
			if src := sourceLabel(e); src != "" {
				back += "<br><br><small>— " + htmlEscape(src) + "</small>"
			}
			fmt.Fprintf(w, "%s\t%s\t%s recall\n", front, back, tags)
			recall++
		}
	}

	if *outPath != "" {
		fmt.Fprintf(os.Stderr, "anki: wrote %d recognition + %d recall cards (%d atoms) to %s\n",
			recognition, recall, len(entries), *outPath)
	}
}

// recognitionScenario picks the first canonical-instance that reads as a
// standalone scenario rather than a comparison against another atom.
// Entries starting with "operationally distinct/adjacent/compositional"
// presuppose familiarity with the very id they'd be testing recognition
// of, which fails the front-is-answerable-without-context rule the
// controlaltbackspace.org guide calls out.
func recognitionScenario(e *types.LexEntry) string {
	skipPrefixes := []string{
		"operationally distinct",
		"operationally adjacent",
		"operationally compositional",
		"central critical question",
		"critical critical question",
		"pivotal critical question",
		"key critical question",
	}
	for _, ci := range e.CanonicalInstances {
		lower := strings.ToLower(strings.TrimSpace(ci))
		skip := false
		for _, p := range skipPrefixes {
			if strings.HasPrefix(lower, p) {
				skip = true
				break
			}
		}
		if !skip && ci != "" {
			return ci
		}
	}
	return ""
}

// sourceLabel extracts a short "Author, Work" attribution from the
// primary lineage citation, rather than the full prose citation (which
// can run past 1000 characters and would itself violate the
// one-fact-per-card rule by turning attribution into a second thing to
// read). Splits on the first period-or-comma-delimited clause; falls
// back to a hard truncation if that clause is unreasonably long, since
// some citations open with a long qualifier before the author name.
func sourceLabel(e *types.LexEntry) string {
	if len(e.Lineage) == 0 || e.Lineage[0].Citation == "" {
		return ""
	}
	cite := e.Lineage[0].Citation
	cite = strings.TrimPrefix(cite, "[")
	cut := -1
	for _, sep := range []string{". ", ", "} {
		if idx := strings.Index(cite, sep); idx > 0 && (cut == -1 || idx < cut) {
			cut = idx
		}
	}
	if cut > 0 && cut < 90 {
		return cite[:cut]
	}
	return ankiTruncate(cite, 70)
}

func displayName(e *types.LexEntry) string {
	if e.CommonName != "" {
		return e.CommonName
	}
	return e.Name
}

func ankiTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if idx := strings.LastIndex(cut, " "); idx > max-50 {
		cut = cut[:idx]
	}
	return cut + "…"
}

// ankiFinding is one card-quality issue, checked against the actual
// rules in Soren Bjornstad's "Rules for Designing Precise Anki Cards"
// (controlaltbackspace.org/precise, the same source cmd_anki.go's own
// design comment above already cites), not vibes about what a good card
// looks like. Advisory only -- runAnkiLint always exits 0. Whether any of
// this becomes a blocking pre-commit gate is a separate decision, made
// after seeing what a first pass over the live corpus actually surfaces.
type ankiFinding struct {
	AtomID string
	Card   string // "recognition" or "recall"
	Check  string
	Detail string
}

// crammedBackChars is the length past which a recall card's back is
// treated as likely violating Rule #4, the Minimum Information Principle
// ("cards should never ask about more than one thing"). Not from the
// source -- it gives no number -- but calibrated against the corpus: a
// first live run at 500 flagged 2994 of 3631 atoms, the median case, not
// an actionable tail (median agent-instruction length is 837 chars; this
// corpus authors that field as rich multi-clause prose by design, not
// short imperative snippets -- see project_anki_deck_quality_pass memory).
// Set to roughly p75 instead, so this flags real outliers.
const crammedBackChars = 1500

// severelyCrammedBackChars marks the true pathological tail separately
// from the ordinary over-threshold case above -- a card this long isn't
// "verbose," it's a data-quality defect (the corpus max is 22,286 chars
// on one _tier: atomic entry, which reads as a disambiguation essay, not
// a flashcard back).
const severelyCrammedBackChars = 8000

var (
	mdItalic          = regexp.MustCompile(`\*([^*\n]+)\*`)
	ankiLexIDMention  = regexp.MustCompile(`(?i)\blex-[a-z0-9]{4,8}\b`)
	ankiOrderedMarker = regexp.MustCompile(`(?m)(^\s*\(?[0-9]{1,2}[.)]\s|^\s*\([a-c]\)\s)`)
	ankiOrdinalPair   = regexp.MustCompile(`(?i)\bfirst\b[^.]{0,120}\bsecond\b`)
)

// runAnkiLint builds the same card set cmdAnki would export and checks
// each card against rules from the cited source that are mechanically
// checkable without a semantic model:
//
//   - Rule #4 (minimum information): a recall back over crammedBackChars,
//     or a recognition front built from a canonical-instance so long it
//     had to be truncated -- both are proxies for "more than one fact."
//   - Rules #9-10 (no enumerations/sets): ordered-list markers or an
//     explicit "first ... second" pair inside a card's text.
//   - "Questions should permit exactly one answer": a recognition front
//     that names another lex- id anywhere (not just as a leading
//     comparison, which recognitionScenario already filters at the
//     front) presupposes recognizing that atom too -- two facts, not one.
//   - A mechanical regression check specific to this exporter, not the
//     source: an odd count of literal `*` in the raw field text means
//     mdItalic can't pair it, and it survives into the card as a literal
//     asterisk instead of becoming <i>.
//
// Explicitly NOT covered: "permits exactly one answer" in the sense of
// near-duplicate cards whose fronts are similar enough to invite the
// wrong back (the source's "interference" concern). That needs semantic
// similarity, not regex -- internal/embedgate already does embedding-
// based similarity for the mint-time distinctness check
// (cmd_distinctness.go) and is the right thing to extend for this, not a
// second bag-of-words similarity metric built fresh here. Left for a
// follow-up once this pass's findings are triaged.
func runAnkiLint(entries []*types.LexEntry) {
	var findings []ankiFinding

	for _, e := range entries {
		if scenario := recognitionScenario(e); scenario != "" {
			if len(scenario) > 380 {
				findings = append(findings, ankiFinding{e.ID, "recognition", "crammed-front",
					fmt.Sprintf("canonical-instance is %d chars; ankiTruncate cuts it to 380, which can sever the detail that makes the scenario identifiable", len(scenario))})
			}
			if loc := ankiLexIDMention.FindString(scenario); loc != "" {
				findings = append(findings, ankiFinding{e.ID, "recognition", "presupposes-another-atom",
					fmt.Sprintf("scenario references %s -- recognizing this atom now depends on already recognizing that one too", loc)})
			}
			if ankiOrderedMarker.MatchString(scenario) || ankiOrdinalPair.MatchString(scenario) {
				findings = append(findings, ankiFinding{e.ID, "recognition", "enumeration",
					"scenario reads as an ordered list/set rather than a single scenario"})
			}
			if strings.Count(scenario, "*")%2 != 0 {
				findings = append(findings, ankiFinding{e.ID, "recognition", "unbalanced-italic",
					"odd number of '*' in canonical-instance -- one won't pair into <i>...</i> and will render as a literal asterisk"})
			}
		}

		if e.AgentInstruction != "" {
			back := e.AgentInstruction
			backLen := len(back)
			if src := sourceLabel(e); src != "" {
				backLen += len(src) + len("<br><br><small>— </small>")
			}
			if backLen > crammedBackChars {
				findings = append(findings, ankiFinding{e.ID, "recall", "crammed-back",
					fmt.Sprintf("back is %d chars (agent-instruction + source), over the %d-char line for \"the single imperative agent-instruction\"", backLen, crammedBackChars)})
			}
			if backLen > severelyCrammedBackChars {
				findings = append(findings, ankiFinding{e.ID, "recall", "severely-crammed-back",
					fmt.Sprintf("back is %d chars -- this isn't verbose, it's reading as an essay, not a flashcard", backLen)})
			}
			if ankiOrderedMarker.MatchString(back) || ankiOrdinalPair.MatchString(back) {
				findings = append(findings, ankiFinding{e.ID, "recall", "enumeration",
					"agent-instruction reads as an ordered list/set of steps rather than one rule"})
			}
			if strings.Count(back, "*")%2 != 0 {
				findings = append(findings, ankiFinding{e.ID, "recall", "unbalanced-italic",
					"odd number of '*' in agent-instruction -- one won't pair into <i>...</i> and will render as a literal asterisk"})
			}
		}
	}

	byCheck := map[string]int{}
	for _, f := range findings {
		byCheck[f.Check]++
	}
	checks := make([]string, 0, len(byCheck))
	for c := range byCheck {
		checks = append(checks, c)
	}
	sort.Strings(checks)

	for _, check := range checks {
		fmt.Printf("=== %s (%d) ===\n", check, byCheck[check])
		for _, f := range findings {
			if f.Check != check {
				continue
			}
			fmt.Printf("  %s [%s] %s\n", f.AtomID, f.Card, f.Detail)
		}
	}
	fmt.Printf("\n%d findings across %d checks, %d atoms considered. Advisory only -- not wired into any commit gate.\n",
		len(findings), len(checks), len(entries))
}

// htmlEscape escapes raw text for Anki's HTML-enabled fields, then
// converts the elements corpus's markdown-style *italic* titles into
// real <i> tags. Order matters: escaping first means the < and > this
// step inserts can't collide with anything the escaper would mangle.
func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"\t", " ",
		"\n", "<br>",
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	escaped := r.Replace(s)
	return mdItalic.ReplaceAllString(escaped, "<i>$1</i>")
}
