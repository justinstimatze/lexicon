package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// cmdCoverage: walk refs/ (long-form files only — pdf/epub/djvu/mobi/
// azw3/lit), tokenize each filename into (author-surname, year) keys,
// and anti-join against the coverage corpus (mining-pass MD stems +
// elements YAML lineage source/text fields). Emit a report of which
// refs lack coverage so they can be queued for future mining passes.
//
// Usage:
//
//	lexicon coverage                    # JSON summary + items
//	lexicon coverage --text             # TSV: ref<TAB>status<TAB>author-year<TAB>match
//	lexicon coverage --uncovered        # only emit uncovered refs
//	lexicon coverage --refs-dir DIR     # override refs directory
//	lexicon coverage --passes-dir DIR   # override docs/passes directory
type coverageItem struct {
	Ref        string `json:"ref"`
	AuthorYear string `json:"author_year,omitempty"`
	Covered    bool   `json:"covered"`
	Match      string `json:"match,omitempty"`
}

var (
	yearRE        = regexp.MustCompile(`(?:^|[^0-9])((?:17|18|19|20)\d{2})(?:[^0-9]|$)`)
	nonAlnumRE    = regexp.MustCompile(`[^a-z0-9]+`)
	camelSplitRE  = regexp.MustCompile(`([a-z])([A-Z])`)
	longFormExts  = map[string]bool{".pdf": true, ".epub": true, ".djvu": true, ".mobi": true, ".azw3": true, ".lit": true}
	// textFormExts count as works only at the top level of refs/ — see
	// collectRefs. A Project Gutenberg .txt of Leviathan is as much a
	// primary as a scanned .pdf of it; a .txt inside a scraped corpus
	// directory is not.
	textFormExts = map[string]bool{".txt": true, ".html": true, ".md": true}
	stripSuffixRE = regexp.MustCompile(`-(?:mining-pass|no-mint|survey|pass)(?:-\d+)?(?:-deferred)?$`)
)

func cmdCoverage(renderDir string, args []string) {
	fl := flag.NewFlagSet("coverage", flag.ExitOnError)
	refsDir := fl.String("refs-dir", "", "refs/ directory (default: <project>/refs)")
	passesDir := fl.String("passes-dir", "", "docs/passes directory (default: <project>/docs/passes)")
	elementsDir := fl.String("elements-dir", "", "elements directory (default: <project>/elements)")
	asText := fl.Bool("text", false, "TSV output (default: JSON)")
	onlyUncov := fl.Bool("uncovered", false, "only emit uncovered refs")
	_ = fl.Parse(args)

	project := filepath.Join(renderDir, "..")
	if *refsDir == "" {
		*refsDir = filepath.Join(project, "refs")
	}
	if *passesDir == "" {
		*passesDir = filepath.Join(project, "docs", "passes")
	}
	if *elementsDir == "" {
		*elementsDir = filepath.Join(project, "elements")
		if _, err := os.Stat(*elementsDir); err != nil {
			*elementsDir = filepath.Join(renderDir, "elements")
		}
	}

	refs, err := collectRefs(*refsDir)
	if err != nil {
		fatal("coverage: refs: %v", err)
	}
	corpus := buildCoverageCorpus(*passesDir, *elementsDir)

	items := make([]coverageItem, 0, len(refs))
	totalCovered := 0
	for _, r := range refs {
		keys := authorYearKeys(r)
		match := ""
		for _, k := range keys {
			if hit := corpus.lookup(k); hit != "" {
				match = hit
				break
			}
		}
		// Fallback: ref has no extractable year (some long-format
		// "_advanced" stems strip the publication date). Match by
		// requiring ≥3 stem tokens (author + title fragments) to be
		// present in a single elements source's token set.
		if match == "" && len(keys) == 0 {
			match = corpus.lookupNoYear(r)
		}
		covered := match != ""
		if covered {
			totalCovered++
		}
		if *onlyUncov && covered {
			continue
		}
		ay := ""
		if len(keys) > 0 {
			ay = keys[0]
		}
		items = append(items, coverageItem{Ref: r, AuthorYear: ay, Covered: covered, Match: match})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Ref < items[j].Ref })

	if *asText {
		for _, it := range items {
			status := "uncovered"
			if it.Covered {
				status = "covered"
			}
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\n", it.Ref, status, it.AuthorYear, it.Match)
		}
		return
	}

	out := map[string]any{
		"refs_total": len(refs),
		"covered":    totalCovered,
		"uncovered":  len(refs) - totalCovered,
		"items":      items,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fatal("coverage: encode: %v", err)
	}
}

// collectRefs returns the stems of every file in refs/ that is a work
// in its own right.
//
// The extension list used to be long-form only (pdf/epub/djvu/mobi/
// azw3/lit), which silently excluded 110 plain-text primaries sitting
// at the top level — Hobbes's Leviathan, the Communist Manifesto,
// Sophocles, Astell 1694, Darwin and Wallace 1858. Those are exactly
// the public-domain acquisitions, so the format that costs nothing to
// obtain was the format the coverage audit could not see.
//
// Subdirectories are walked, but only long-form files inside them
// count. The .txt files below refs/ are bulk scraped corpora (3,240
// xkcd explanations, 275 Substack posts) which are refs but are not
// works, and admitting them would add 3,500 items to a backlog that is
// supposed to be readable.
func collectRefs(refsDir string) ([]string, error) {
	entries, err := os.ReadDir(refsDir)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			sub, err := os.ReadDir(filepath.Join(refsDir, name))
			if err != nil {
				continue
			}
			for _, s := range sub {
				if s.IsDir() || !longFormExts[strings.ToLower(filepath.Ext(s.Name()))] {
					continue
				}
				out = append(out, strings.TrimSuffix(s.Name(), filepath.Ext(s.Name())))
			}
			continue
		}
		// OCR sidecars sit next to their PDF as <stem>.ocr.txt so they are
		// findable, but they are a rendering of a work already counted, not
		// a second work. Counting both would inflate the denominator and
		// make every scanned book look like two.
		if strings.HasSuffix(strings.ToLower(name), ".ocr.txt") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !longFormExts[ext] && !textFormExts[ext] {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ext))
	}
	return out, nil
}

// coverageCorpus stores tokens-of-each-source for fast "does this
// source contain author AND year?" lookups. The source label is kept
// so the match can be reported back.
type coverageCorpus struct {
	sources []coverageSource
}

type coverageSource struct {
	label  string
	tokens map[string]bool
}

// lookupNoYear is the fallback when authorYearKeys couldn't extract a
// year (typical of long-format "_advanced" stems). It tokenizes the
// full ref stem and requires ≥3 token overlap with a single source to
// declare coverage. The 3-token threshold avoids spurious matches on
// stopword-rich titles (e.g. "the", "of") while catching the common
// case where author surname + 2 title tokens are present.
func (c coverageCorpus) lookupNoYear(refStem string) string {
	refTokens := tokenize(refStem)
	delete(refTokens, "advanced")
	if len(refTokens) < 3 {
		return ""
	}
	bestLabel, bestHits := "", 0
	for _, s := range c.sources {
		hits := 0
		for tok := range refTokens {
			if s.tokens[tok] {
				hits++
			}
		}
		if hits > bestHits {
			bestHits, bestLabel = hits, s.label
		}
	}
	if bestHits >= 3 {
		return bestLabel
	}
	return ""
}

func (c coverageCorpus) lookup(authorYear string) string {
	parts := strings.SplitN(authorYear, "-", 2)
	if len(parts) != 2 {
		return ""
	}
	author, year := parts[0], parts[1]
	if len(author) < 4 {
		return ""
	}
	for _, s := range c.sources {
		if !s.tokens[year] {
			continue
		}
		for tok := range s.tokens {
			if len(tok) < 4 {
				continue
			}
			if tok == author {
				return s.label
			}
			// Substring match (either direction). Lowered min-length
			// from 5 to 4 so short surnames after prefix-strip (Du Bois
			// → "bois"; Le Guin → "guin") match when contained in the
			// joined corpus form ("dubois", "leguin").
			if len(author) >= 4 && strings.Contains(tok, author) {
				return s.label
			}
			if len(tok) >= 4 && strings.Contains(author, tok) {
				return s.label
			}
		}
	}
	return ""
}

func buildCoverageCorpus(passesDir, elementsDir string) coverageCorpus {
	out := coverageCorpus{}
	if entries, err := os.ReadDir(passesDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			stem := strings.TrimSuffix(name, ".md")
			for stripSuffixRE.MatchString(stem) {
				stem = stripSuffixRE.ReplaceAllString(stem, "")
			}
			out.sources = append(out.sources, coverageSource{label: stem, tokens: tokenize(stem)})
		}
	}
	if entries, err := os.ReadDir(elementsDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "lex-") || !strings.HasSuffix(name, ".yaml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(elementsDir, name))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				trim := strings.TrimSpace(line)
				if !strings.HasPrefix(trim, "- source:") && !strings.HasPrefix(trim, "source:") && !strings.HasPrefix(trim, "text:") {
					continue
				}
				val := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trim, "-"), " "), "source:"))
				val = strings.TrimSpace(strings.TrimPrefix(val, "text:"))
				val = strings.Trim(val, "\"'")
				if val == "" || val == "primary" || val == "secondary" {
					continue
				}
				out.sources = append(out.sources, coverageSource{label: strings.TrimSuffix(name, ".yaml") + "[" + val + "]", tokens: tokenize(val)})
			}
		}
	}
	return out
}

func tokenize(s string) map[string]bool {
	expanded := camelSplitRE.ReplaceAllString(s, "$1-$2")
	lower := strings.ToLower(expanded)
	flat := nonAlnumRE.ReplaceAllString(lower, " ")
	out := map[string]bool{}
	for _, t := range strings.Fields(flat) {
		if len(t) >= 3 {
			out[t] = true
		}
	}
	return out
}

// authorYearKeys returns candidate (author-surname, year) keys for a
// ref filename. Multiple keys are returned when the filename is
// ambiguous (e.g. compound CamelCase authors: AxelrodHamilton →
// {axelrodhamilton-1981, axelrod-1981, hamilton-1981}).
func authorYearKeys(stem string) []string {
	expanded := camelSplitRE.ReplaceAllString(stem, "$1-$2")
	lower := strings.ToLower(expanded)
	flat := nonAlnumRE.ReplaceAllString(lower, "-")
	flat = strings.Trim(flat, "-")
	parts := strings.Split(flat, "-")

	var year string
	yearIdx := -1
	for i, p := range parts {
		if len(p) == 4 && yearRE.MatchString("-"+p+"-") {
			year = p
			yearIdx = i
			break
		}
	}
	if year == "" {
		return nil
	}

	keys := []string{}
	seen := map[string]bool{}
	add := func(author string) {
		if len(author) < 4 {
			return
		}
		k := author + "-" + year
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}

	// Surname-prefix detection. "Du-Bois" → also try "dubois"; "Van-Der-
	// Sluis" → also try "vandersluis"; "Le-Guin" → "leguin". The walk
	// below would otherwise discard "du"/"van"/"le" as 2-char noise and
	// emit only "bois"/"sluis"/"guin", which often fail to match
	// elements forms that use the joined surname.
	surnamePrefix := map[string]bool{
		"du": true, "de": true, "van": true, "von": true, "le": true,
		"la": true, "el": true, "al": true, "der": true, "den": true,
		"da": true, "di": true, "do": true, "dos": true, "des": true,
		"del": true, "mac": true, "mc": true,
	}
	for i := yearIdx - 1; i > 0; i-- {
		prev := parts[i-1]
		cur := parts[i]
		if cur == "" || len(cur) < 3 {
			continue
		}
		if surnamePrefix[prev] {
			add(prev + cur)
		}
	}

	// Walk backward from year to find the author tokens, skipping noise.
	for i := yearIdx - 1; i >= 0; i-- {
		tok := parts[i]
		if tok == "" || len(tok) < 3 {
			continue
		}
		// skip obvious non-author tokens
		switch tok {
		case "the", "and", "von", "der", "des", "del", "vol", "ch", "ed", "by", "of", "in", "on", "for":
			continue
		}
		add(tok)
		if len(keys) >= 3 {
			break
		}
	}

	// Long-format catalogue filenames: "Surname, FirstName" — surname appears
	// after a `--` separator. Pull the first token after `--` as a candidate.
	if dashIdx := strings.Index(stem, " -- "); dashIdx > 0 {
		tail := stem[dashIdx+4:]
		if comma := strings.Index(tail, ","); comma > 0 {
			surname := strings.ToLower(strings.TrimSpace(tail[:comma]))
			surname = nonAlnumRE.ReplaceAllString(surname, "")
			add(surname)
		}
	}
	return keys
}
