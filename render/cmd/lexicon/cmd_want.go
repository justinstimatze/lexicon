package main

// `lexicon want check` — verify wanted-materials.md items against refs/ across
// lexicon + sibling project refs dirs. The static list drifts from reality as
// new sources land (often under varied naming conventions: Author-Year-Title,
// authoryear, lowercase-with-collaborators, numeric-hash-prefix, etc.). The
// verifier:
//
//  1. Parses wanted-materials.md TOP block (the user-procure ranked list,
//     paste-string format).
//  2. Builds a normalized refs index across this project's own refs/ dir
//     plus sibling projects' refs/ dirs when present locally, using
//     os.ReadDir (NOT subject to .gitignore).
//  3. For each item, extracts (author-surname, year, distinctive-title-token)
//     and OR-matches against normalized filenames. Multi-pattern hits raise
//     confidence.
//  4. Reports a three-state diff: present / missing / ambiguous.
//
// `lexicon want check --apply` rewrites wanted-materials.md, removing items
// whose status is `present` with a single high-confidence match.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func cmdWant(renderDir string, args []string) {
	fs := flag.NewFlagSet("want", flag.ExitOnError)
	apply := fs.Bool("apply", false, "rewrite wanted-materials.md removing present items")
	dryDetail := fs.Bool("detail", false, "print matched filename and source dir for each present item")
	_ = fs.Parse(args)
	sub := ""
	if fs.NArg() > 0 {
		sub = fs.Arg(0)
	}

	projectRoot := filepath.Clean(filepath.Join(renderDir, ".."))
	wantedPath := filepath.Join(projectRoot, "wanted-materials.md")

	switch sub {
	case "", "check":
		runWantCheck(wantedPath, *apply, *dryDetail)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %q (try: check)\n", sub)
		os.Exit(2)
	}
}

type wantedItem struct {
	// raw block from md: line1 (paste-string) + line2 (leverage tag)
	line1 string
	line2 string
	// blockStart/blockEnd: byte offsets in wanted-materials.md for rewrite
	blockStart int
	blockEnd   int

	// extracted
	author    string   // lowercase surname
	year      string   // 4-digit
	titleToks []string // distinctive title words (lowercase, len≥4)
}

type wantStatus struct {
	item       wantedItem
	state      string // "present" | "missing" | "ambiguous"
	matches    []string
	matchesDir []string
}

func runWantCheck(wantedPath string, apply, detail bool) {
	raw, err := os.ReadFile(wantedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read wanted-materials.md: %v\n", err)
		os.Exit(1)
	}
	items := parseWantedTop(string(raw))
	if len(items) == 0 {
		fmt.Println("no items parsed from wanted-materials.md TOP block")
		return
	}

	refsDirs := siblingRefsDirs()
	index := buildRefsIndex(refsDirs)

	statuses := make([]wantStatus, len(items))
	for i, it := range items {
		statuses[i] = matchItem(it, index)
	}

	// Counts
	var present, missing, ambig int
	for _, s := range statuses {
		switch s.state {
		case "present":
			present++
		case "missing":
			missing++
		case "ambiguous":
			ambig++
		}
	}

	fmt.Printf("wanted-materials.md TOP block: %d items\n", len(items))
	fmt.Printf("  ✓ present:   %d\n", present)
	fmt.Printf("  ✗ missing:   %d\n", missing)
	fmt.Printf("  ? ambiguous: %d\n", ambig)
	fmt.Printf("refs dirs scanned: %d (%d files indexed)\n", len(refsDirs), len(index))
	fmt.Println()

	// Detail per state
	if present > 0 {
		fmt.Println("PRESENT (consider removing):")
		for _, s := range statuses {
			if s.state != "present" {
				continue
			}
			fmt.Printf("  ✓ %s\n", wantTruncate(s.item.line1, 80))
			if detail {
				for i, m := range s.matches {
					fmt.Printf("      → %s/%s\n", s.matchesDir[i], m)
				}
			}
		}
		fmt.Println()
	}
	if ambig > 0 {
		fmt.Println("AMBIGUOUS (manual review):")
		for _, s := range statuses {
			if s.state != "ambiguous" {
				continue
			}
			fmt.Printf("  ? %s\n", wantTruncate(s.item.line1, 80))
			for i, m := range s.matches {
				fmt.Printf("      ~ %s/%s\n", s.matchesDir[i], m)
			}
		}
		fmt.Println()
	}
	if missing > 0 {
		fmt.Println("MISSING (genuinely not in any refs/):")
		for _, s := range statuses {
			if s.state == "missing" {
				fmt.Printf("  ✗ %s\n", wantTruncate(s.item.line1, 80))
			}
		}
	}

	if apply {
		newRaw := rewriteWithoutPresent(string(raw), statuses)
		if err := os.WriteFile(wantedPath, []byte(newRaw), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "rewrite wanted-materials.md: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n--apply: removed %d present items from wanted-materials.md\n", present)
	}
}

func siblingRefsDirs() []string {
	home := os.Getenv("HOME")
	if u, err := user.Current(); err == nil {
		home = u.HomeDir
	}
	if home == "" {
		return nil
	}
	cand := []string{filepath.Join(home, "Documents/lexicon/refs")}
	if extra := os.Getenv("LEXICON_SIBLING_REFS_DIRS"); extra != "" {
		cand = append(cand, filepath.SplitList(extra)...)
	}
	out := []string{}
	for _, p := range cand {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

type indexedFile struct {
	dir, name, normalized, concat string
}

func buildRefsIndex(dirs []string) []indexedFile {
	var idx []indexedFile
	for _, d := range dirs {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			low := strings.ToLower(name)
			norm := strings.NewReplacer(
				"_", " ", "-", " ", ".", " ", "/", " ", ":", " ",
			).Replace(low)
			// concat: like normalized but with all whitespace removed.
			// Catches concat forms ("godfreysmith" in "...peter godfrey smith...")
			// and concat surnames in filenames like "Bell1964.pdf".
			concat := strings.Join(strings.Fields(norm), "")
			idx = append(idx, indexedFile{
				dir:        filepath.Base(filepath.Dir(d)) + "/" + filepath.Base(d),
				name:       name,
				normalized: norm,
				concat:     concat,
			})
		}
	}
	return idx
}

// parseWantedTop reads the file and returns ONLY items from the user-procure
// top block — between "🔝 TOP" section start and the "### Claude self-acquire"
// section. Items are paste-string format: line1 = "Author Year Title ..."
// optionally followed by a "— leverage tag" line.
func parseWantedTop(raw string) []wantedItem {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	inTop := false
	var lines []string
	var offsets []int
	off := 0
	for scanner.Scan() {
		l := scanner.Text()
		lines = append(lines, l)
		offsets = append(offsets, off)
		off += len(l) + 1
		if !inTop && strings.Contains(l, "🔝 TOP") {
			inTop = true
			continue
		}
		// Stop at the Claude self-acquire backlog section
		if inTop && strings.HasPrefix(l, "### Claude self-acquire backlog") {
			break
		}
	}
	// File-end offset for the last item
	offsets = append(offsets, off)

	if !inTop {
		return nil
	}

	var items []wantedItem
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		// Item-start heuristic: non-empty line that does NOT begin with
		// '#', '*', '-', '(', '|', '_' and contains a 4-digit year between
		// 1700 and 2030.
		if l == "" {
			continue
		}
		first := l[0]
		if first == '#' || first == '*' || first == '-' || first == '(' ||
			first == '|' || first == '_' {
			continue
		}
		if !hasPlausibleYear(l) {
			continue
		}
		// Skip the header italics line that has "Format:" prefix
		if strings.HasPrefix(strings.TrimSpace(l), "Format:") {
			continue
		}
		it := wantedItem{line1: l, blockStart: offsets[i]}
		// Pull the next line if it starts with "—" (em dash) — that's the leverage tag
		if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "—") {
			it.line2 = lines[i+1]
			// Block end = start of next non-blank or end of file
			j := i + 2
			for j < len(lines) && lines[j] == "" {
				j++
			}
			if j < len(offsets) {
				it.blockEnd = offsets[j]
			} else {
				it.blockEnd = offsets[len(offsets)-1]
			}
			i++ // consume the line2
		} else {
			// Single-line item
			it.blockEnd = offsets[i+1]
		}
		extractTokens(&it)
		items = append(items, it)
	}
	return items
}

var yearRe = regexp.MustCompile(`\b(1[7-9]\d\d|20[0-3]\d)\b`)

func hasPlausibleYear(s string) bool {
	return yearRe.MatchString(s)
}

// extractTokens fills item.author, year, titleToks heuristically from line1.
func extractTokens(it *wantedItem) {
	// Tokenize on spaces; strip punctuation
	toks := []string{}
	for _, t := range strings.Fields(it.line1) {
		t = strings.Trim(t, ",.;:()[]{}\"'")
		if t != "" {
			toks = append(toks, t)
		}
	}
	if len(toks) == 0 {
		return
	}
	// First token is usually the author surname; but for two-author items
	// like "Doudna and Charpentier 2012 ..." we'll prefer the first surname.
	// Capitalized leading word that isn't a year.
	if isLikelySurname(toks[0]) {
		it.author = strings.ToLower(toks[0])
	}
	for _, t := range toks {
		if m := yearRe.FindString(t); m != "" {
			it.year = m
			break
		}
	}
	// Title tokens: distinctive long words AFTER year, lowercase, len≥4,
	// excluding common stopwords.
	stop := map[string]bool{
		"the": true, "and": true, "of": true, "in": true, "a": true,
		"an": true, "to": true, "for": true, "on": true, "with": true,
		"by": true, "from": true, "or": true, "as": true, "is": true,
		"how": true, "why": true, "what": true, "where": true, "when": true,
		"university": true, "press": true, "publishing": true, "books": true,
		"company": true, "inc": true, "llc": true, "ltd": true,
	}
	afterYear := false
	for _, t := range toks {
		if yearRe.MatchString(t) {
			afterYear = true
			continue
		}
		if !afterYear {
			continue
		}
		lt := strings.ToLower(strings.Trim(t, ",.;:()[]{}\"'"))
		if len(lt) < 4 {
			continue
		}
		if stop[lt] {
			continue
		}
		// Only alphabetic
		alpha := true
		for _, r := range lt {
			if (r < 'a' || r > 'z') && r != '-' && r != '\'' {
				alpha = false
				break
			}
		}
		if !alpha {
			continue
		}
		it.titleToks = append(it.titleToks, lt)
	}
	// Limit to the most distinctive few
	if len(it.titleToks) > 6 {
		it.titleToks = it.titleToks[:6]
	}
}

func isLikelySurname(s string) bool {
	if len(s) < 2 {
		return false
	}
	if s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	// Reject common non-surname capitalized openers
	switch s {
	case "The", "A", "An", "In", "On", "At", "From", "How", "Why", "What":
		return false
	}
	return true
}

// matchItem scans the refs index for filenames that match the item.
// Scoring tracks (author, year, title-tokens-matched) separately:
//   - authorHit: author surname appears as a word in normalized filename
//   - yearHit: year appears in normalized filename
//   - titleHits: count of title tokens appearing
//
// Verdict:
//   - PRESENT requires authorHit (no false-positives from year-coincidence).
//     A single authorHit + 1 supporting signal (year or title) is enough.
//     Tied authorHit results pick highest score; if multiple identical-best
//     authorHit matches survive, present with the first (likely same work).
//   - AMBIGUOUS: year matched + ≥1 title token, but author not matched.
//     User reviews manually.
//   - MISSING: nothing meaningful matched.
func matchItem(it wantedItem, index []indexedFile) wantStatus {
	type scored struct {
		f          indexedFile
		score      int
		authorHit  bool
		yearHit    bool
		titleHits  int
	}
	var authorHits []scored
	var weakHits []scored
	for _, f := range index {
		s := scored{f: f}
		if it.author != "" {
			// match against word-bounded normalized OR no-space concat form
			// (handles hyphenated authors like "Godfrey-Smith" against
			// "godfrey smith" in filename, and compact authoryear forms
			// like "Bell1964.pdf")
			if containsWord(f.normalized, it.author) || strings.Contains(f.concat, it.author) {
				s.authorHit = true
				s.score += 3
			}
		}
		if it.year != "" && strings.Contains(f.normalized, it.year) {
			s.yearHit = true
			s.score += 2
		}
		for _, t := range it.titleToks {
			if containsWord(f.normalized, t) {
				s.titleHits++
				s.score++
			}
		}
		// PRESENT candidates: author matched + at least one other signal
		if s.authorHit && (s.yearHit || s.titleHits >= 1) {
			authorHits = append(authorHits, s)
		} else if !s.authorHit && s.yearHit && s.titleHits >= 2 {
			// Ambiguous: year + ≥2 title tokens but no author
			weakHits = append(weakHits, s)
		}
	}
	if len(authorHits) > 0 {
		sort.Slice(authorHits, func(i, j int) bool {
			return authorHits[i].score > authorHits[j].score
		})
		top := authorHits[0].score
		st := wantStatus{item: it, state: "present"}
		// Take top + any tied at same score
		for _, h := range authorHits {
			if h.score < top {
				break
			}
			st.matches = append(st.matches, h.f.name)
			st.matchesDir = append(st.matchesDir, h.f.dir)
		}
		// Trim if many — single representative is enough
		if len(st.matches) > 1 {
			st.matches = st.matches[:1]
			st.matchesDir = st.matchesDir[:1]
		}
		return st
	}
	if len(weakHits) > 0 {
		sort.Slice(weakHits, func(i, j int) bool {
			return weakHits[i].score > weakHits[j].score
		})
		st := wantStatus{item: it, state: "ambiguous"}
		top := weakHits[0].score
		for i, h := range weakHits {
			if h.score < top || i >= 5 {
				break
			}
			st.matches = append(st.matches, h.f.name)
			st.matchesDir = append(st.matchesDir, h.f.dir)
		}
		return st
	}
	return wantStatus{item: it, state: "missing"}
}

// containsWord checks if normalized (already lowercase + spaces-only) contains
// `tok` as a whole word (space-bounded). Accommodates start/end of string.
func containsWord(normalized, tok string) bool {
	if tok == "" {
		return false
	}
	// Quick reject
	idx := strings.Index(normalized, tok)
	if idx == -1 {
		return false
	}
	// Check left boundary
	if idx > 0 {
		c := normalized[idx-1]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			// not a word boundary — try next occurrence
			rest := normalized[idx+1:]
			if i := strings.Index(rest, tok); i >= 0 {
				return containsWord(normalized[idx+1+i:], tok)
			}
			return false
		}
	}
	// Check right boundary
	endIdx := idx + len(tok)
	if endIdx < len(normalized) {
		c := normalized[endIdx]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			rest := normalized[idx+1:]
			if i := strings.Index(rest, tok); i >= 0 {
				return containsWord(normalized[idx+1+i:], tok)
			}
			return false
		}
	}
	return true
}

func wantTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func rewriteWithoutPresent(raw string, statuses []wantStatus) string {
	// Build a sorted list of (start, end) byte ranges to delete
	type rng struct{ start, end int }
	var dels []rng
	for _, s := range statuses {
		if s.state != "present" {
			continue
		}
		if s.item.blockEnd > s.item.blockStart {
			dels = append(dels, rng{s.item.blockStart, s.item.blockEnd})
		}
	}
	sort.Slice(dels, func(i, j int) bool { return dels[i].start < dels[j].start })

	// Splice the file
	var b strings.Builder
	cursor := 0
	for _, d := range dels {
		if d.start < cursor {
			continue // overlap — skip
		}
		b.WriteString(raw[cursor:d.start])
		cursor = d.end
	}
	b.WriteString(raw[cursor:])
	return b.String()
}
