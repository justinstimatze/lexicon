package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// cmdBacklog: one ranked mining queue, ordered by how many readers hit
// the hole.
//
// `coverage` answers a narrower question than its name suggests: does a
// mining-pass MD exist for this ref? It reports "covered" the moment one
// does, which is *before* the atom is finished. Every atom graduated in
// the 2026-08-04 pass — Turing 1936, Zuboff 2019 — reported covered
// while shipping as status: under-review with empty quote stubs. So the
// tool ranked zero real work items above 337 unmined refs, because it
// could not see the work items at all.
//
// Ranking is by in-degree, not by ref-coverage, and the reason is what
// a reader actually encounters. An unmined ref is invisible: no atom
// cites it, nothing links to it, nobody arrives at it. An atom shipped
// as under-review is visible, and one with in-degree 88 is a hole that
// eighty-eight other atoms point directly into. Under-review work is
// therefore ranked first and uncovered refs follow at rank 0, which is
// their true in-degree — nothing points at them yet.
//
// Usage:
//
//	lexicon backlog                     # JSON, both kinds, ranked
//	lexicon backlog --text              # TSV
//	lexicon backlog --kind under-review # only shipped-but-unfinished atoms
//	lexicon backlog --kind uncovered    # only unmined refs
//	lexicon backlog --limit 10          # top N
type backlogItem struct {
	Kind     string `json:"kind"` // "under-review" | "uncovered-ref"
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Ref      string `json:"ref,omitempty"`
	InDegree int    `json:"in_degree"`

	// SourceOnDisk reports whether a lineage entry's (author, year)
	// matches a file already in refs/. False means the atom is blocked
	// on acquisition; true means it is blocked on nobody having
	// re-opened it since the file landed — which was the case for 52 of
	// the 55 under-review atoms in the first census, so the default
	// assumption should be "the source is already here."
	SourceOnDisk bool `json:"source_on_disk"`

	// EmptyQuotes counts lineage entries carrying `quote: ""`. A
	// non-zero count is the concrete unit of work: read the primary,
	// fill the stub.
	EmptyQuotes int `json:"empty_quotes"`

	// LineageSources is the set of `source:` values across the atom's
	// lineage. An atom with no `primary` in it is asserting something
	// on practitioner testimony alone.
	LineageSources []string `json:"lineage_sources,omitempty"`

	// Blocker names what actually stands in the way, which is not the
	// same question for every item:
	//
	//   "read"       — the source is in refs/; nobody has reopened the
	//                  atom since the file landed. Pure bookkeeping.
	//   "acquire"    — lineage names a primary that is not on disk.
	//   "no-primary" — lineage cites only an encyclopedia or a web
	//                  page, so there is no primary to acquire until
	//                  someone decides what it should be. This is the
	//                  most expensive kind and the easiest to mistake
	//                  for the cheapest, because nothing looks missing.
	Blocker string `json:"blocker,omitempty"`
}

// namesPrimary reports whether a lineage `text` slug points at an
// identifiable published work rather than at an encyclopedia entry or a
// bare web page. "wikipedia-falsifiability-2025-12" parses into
// plausible (author, year) keys — "falsifiability-2025" — so the key
// extraction alone cannot tell the two apart and has to be told.
func namesPrimary(text string) bool {
	switch {
	case strings.HasPrefix(text, "wikipedia-"),
		strings.HasPrefix(text, "web-"),
		strings.HasPrefix(text, "wiki-"):
		return false
	}
	return len(authorYearKeys(text)) > 0
}

func cmdBacklog(renderDir string, args []string) {
	fl := flag.NewFlagSet("backlog", flag.ExitOnError)
	refsDir := fl.String("refs-dir", "", "refs/ directory (default: <project>/refs)")
	passesDir := fl.String("passes-dir", "", "docs/passes directory (default: <project>/docs/passes)")
	elementsDir := fl.String("elements-dir", "", "elements directory (default: <project>/elements)")
	kind := fl.String("kind", "all", "all | under-review | uncovered")
	limit := fl.Int("limit", 0, "emit at most N items (0 = all)")
	asText := fl.Bool("text", false, "TSV output (default: JSON)")
	_ = fl.Parse(args)

	project := filepath.Join(renderDir, "..")
	if *refsDir == "" {
		*refsDir = filepath.Join(project, "refs")
	}
	if *passesDir == "" {
		*passesDir = filepath.Join(project, "docs", "passes")
	}
	if *elementsDir == "" {
		*elementsDir = resolveElementsDir(renderDir)
	}

	pool, err := loader.LoadAll(*elementsDir)
	if err != nil {
		fatal("backlog: elements: %v", err)
	}
	refs, err := collectRefs(*refsDir)
	if err != nil {
		fatal("backlog: refs: %v", err)
	}

	// Every (author, year) key present in refs/, so an atom can be asked
	// whether its own source is already sitting on disk.
	//
	// This deliberately walks EVERY file in refs/, not the long-form
	// set collectRefs returns. The long-form filter is right for "which
	// books are unmined" and wrong here: C.S. Lewis's "The Inner Ring"
	// is a 1944 oration saved as .html, and filtering it out reported
	// lex-78gp5 as blocked on acquisition while the text sat in refs/.
	onDisk := map[string]bool{}
	if entries, err := os.ReadDir(*refsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			for _, k := range authorYearKeys(stem) {
				onDisk[k] = true
			}
		}
	}

	inDeg := inDegrees(pool)
	items := []backlogItem{}

	if *kind == "all" || *kind == "under-review" {
		for _, id := range sortedIDs(pool) {
			e := pool[id]
			if e.Status != "under-review" {
				continue
			}
			items = append(items, underReviewItem(e, inDeg[id], onDisk))
		}
	}

	if *kind == "all" || *kind == "uncovered" {
		corpus := buildCoverageCorpus(*passesDir, *elementsDir)
		for _, r := range refs {
			if coveredRef(corpus, r) {
				continue
			}
			items = append(items, backlogItem{Kind: "uncovered-ref", Ref: r})
		}
	}

	// Rank: in-degree desc, then under-review before uncovered at equal
	// rank, then id/ref for a stable order across runs.
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.InDegree != b.InDegree {
			return a.InDegree > b.InDegree
		}
		if a.Kind != b.Kind {
			return a.Kind == "under-review"
		}
		return a.ID+a.Ref < b.ID+b.Ref
	})

	underReview, uncovered := 0, 0
	for _, it := range items {
		if it.Kind == "under-review" {
			underReview++
		} else {
			uncovered++
		}
	}
	if *limit > 0 && *limit < len(items) {
		items = items[:*limit]
	}

	if *asText {
		for _, it := range items {
			fmt.Fprintf(os.Stdout, "%d\t%s\t%s\t%s\t%s\tstubs=%d\n",
				it.InDegree, it.Kind, it.ID+it.Ref, it.Name, it.Blocker, it.EmptyQuotes)
		}
		return
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"under_review": underReview,
		"uncovered":    uncovered,
		"items":        items,
	}); err != nil {
		fatal("backlog: encode: %v", err)
	}
}

func underReviewItem(e *types.LexEntry, deg int, onDisk map[string]bool) backlogItem {
	it := backlogItem{Kind: "under-review", ID: e.ID, Name: e.Name, InDegree: deg}
	seen := map[string]bool{}
	named := false
	for _, l := range e.Lineage {
		if strings.TrimSpace(l.Quote) == "" {
			it.EmptyQuotes++
		}
		if l.Source != "" && !seen[l.Source] {
			seen[l.Source] = true
			it.LineageSources = append(it.LineageSources, l.Source)
		}
		// Lineage `text` is already a surname-year-slug, so the same
		// key extraction that runs over ref filenames works on it
		// unchanged — including the compound-author case, where
		// "graeber-wengrow-2021" yields both surnames and either can
		// match the file on disk.
		if !namesPrimary(l.Text) {
			continue
		}
		named = true
		for _, k := range authorYearKeys(l.Text) {
			if onDisk[k] {
				it.SourceOnDisk = true
			}
		}
	}
	switch {
	case it.SourceOnDisk:
		it.Blocker = "read"
	case named:
		it.Blocker = "acquire"
	default:
		it.Blocker = "no-primary"
	}
	sort.Strings(it.LineageSources)
	return it
}

// coveredRef mirrors the match logic in cmdCoverage: author-year keys
// first, then the no-year token fallback for stems whose publication
// date was stripped by the cataloguing format.
func coveredRef(corpus coverageCorpus, ref string) bool {
	keys := authorYearKeys(ref)
	for _, k := range keys {
		if corpus.lookup(k) != "" {
			return true
		}
	}
	return len(keys) == 0 && corpus.lookupNoYear(ref) != ""
}

func inDegrees(pool map[string]*types.LexEntry) map[string]int {
	deg := map[string]int{}
	for _, e := range pool {
		for _, rel := range e.Related {
			if _, ok := pool[rel]; ok {
				deg[rel]++
			}
		}
		for _, dec := range e.DecomposesInto {
			if _, ok := pool[dec]; ok {
				deg[dec]++
			}
		}
	}
	return deg
}

func sortedIDs(pool map[string]*types.LexEntry) []string {
	out := make([]string, 0, len(pool))
	for k := range pool {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
