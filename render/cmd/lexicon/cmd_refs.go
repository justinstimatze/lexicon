package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdRefs: find things in refs/.
//
// This exists because `ls refs/ | rg -i surname` — the obvious thing to
// type — is wrong in three ways at once, and looks right every time:
//
//   - refs/ holds far more files than top-level entries suggest — most
//     live several directories deep, and `ls` without -R misses nearly
//     all of that depth.
//   - matching a surname misses works filed under a translator or an
//     editor. The Bhagavad Gita is Arnold-1885-BhagavadGita.txt; no
//     search for "gita" that greps author fields will find it, and no
//     search for "krishna" finds it at all.
//   - a miss is indistinguishable from an absence. Empty output reads
//     as "we don't have it", which is how atoms end up saying
//     "acquisition pending" about files already on disk.
//
// So this matches every token of the query against every token of every
// path, recursively, and reports partial matches rather than staying
// silent. A partial match is the useful output: it says "something with
// this author but a different year is here", which is exactly the case
// that a surname grep answers wrongly.
type refHit struct {
	Path    string   `json:"path"`
	Matched []string `json:"matched"`
	Missed  []string `json:"missed"`
	Score   float64  `json:"score"`
}

func cmdRefs(renderDir string, args []string) {
	fl := flag.NewFlagSet("refs", flag.ExitOnError)
	refsDir := fl.String("refs-dir", "", "refs/ directory (default: <project>/refs)")
	asJSON := fl.Bool("json", false, "JSON output (default: text)")
	all := fl.Bool("all", false, "include bulk corpora subdirectories")
	limit := fl.Int("limit", 20, "max hits (0 = all)")
	_ = fl.Parse(args)

	query := strings.Join(fl.Args(), " ")
	if strings.TrimSpace(query) == "" {
		fatal("refs: give a query — surname, title words, year, or any mix")
	}
	if *refsDir == "" {
		*refsDir = filepath.Join(renderDir, "..", "refs")
	}

	want := tokenize(query)
	if len(want) == 0 {
		fatal("refs: query has no tokens of 3+ characters")
	}

	hits := []refHit{}
	_ = filepath.WalkDir(*refsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(*refsDir, path)
		if !*all && isBulkCorpus(rel) {
			return nil
		}
		// Tokenize the whole relative path, not just the basename: a
		// file's directory often carries the author or series name that
		// the filename leaves out.
		have := tokenize(rel)
		matched, missed := []string{}, []string{}
		for t := range want {
			if have[t] || tokenPrefixHit(have, t) {
				matched = append(matched, t)
			} else {
				missed = append(missed, t)
			}
		}
		if len(matched) == 0 {
			return nil
		}
		sort.Strings(matched)
		sort.Strings(missed)
		hits = append(hits, refHit{
			Path:    rel,
			Matched: matched,
			Missed:  missed,
			Score:   float64(len(matched)) / float64(len(want)),
		})
		return nil
	})

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Path < hits[j].Path
	})
	if *limit > 0 && *limit < len(hits) {
		hits = hits[:*limit]
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"query": query, "hits": hits}); err != nil {
			fatal("refs: encode: %v", err)
		}
		return
	}
	if len(hits) == 0 {
		fmt.Printf("refs: no file matches any token of %q — this is an absence, not a miss\n", query)
		return
	}
	for _, h := range hits {
		note := ""
		if len(h.Missed) > 0 {
			note = "   (no " + strings.Join(h.Missed, ", ") + ")"
		}
		fmt.Printf("%3.0f%%  %s%s\n", h.Score*100, h.Path, note)
	}
}

// isBulkCorpus reports whether a path sits inside a scraped bulk
// collection rather than being a work in its own right. These are real
// refs and stay searchable under --all, but 3,240 xkcd explanations
// would otherwise drown every query.
func isBulkCorpus(rel string) bool {
	dir := filepath.Dir(rel)
	if dir == "." {
		return false
	}
	switch {
	case strings.HasPrefix(dir, "explainxkcd-corpus"),
		strings.HasPrefix(dir, "Chapin-Substack"),
		strings.HasSuffix(dir, "_files"):
		return true
	}
	return false
}

// tokenPrefixHit lets a query token match a longer path token, so
// "toulmin" finds "toulmins" and "argument" finds "argumentation".
// Deliberately one-directional: a 3-character query token would
// otherwise match most of the corpus.
func tokenPrefixHit(have map[string]bool, t string) bool {
	if len(t) < 5 {
		return false
	}
	for h := range have {
		if strings.HasPrefix(h, t) || strings.HasPrefix(t, h) && len(h) >= 5 {
			return true
		}
	}
	return false
}
