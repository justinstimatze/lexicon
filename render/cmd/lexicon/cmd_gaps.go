package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// cmdGaps emits a structured report of already-flagged gaps in the
// elements: atoms with empty critical-questions (coverage holes),
// atoms with status: under-review (graduation candidates), and
// "spinoff candidates noted" / "candidates not minted" / "deferred"
// sections aggregated from mining-pass markdown.
//
// Tier 1 of ROADMAP item #13 (self-gap-detection). The roadmap defers
// Tier 2 (evokes-cluster analysis) + Tier 3 (embedding-density holes)
// to later; this is the cheap-and-immediately-useful surface.
func cmdGaps(renderDir string, args []string) {
	fl := flag.NewFlagSet("gaps", flag.ExitOnError)
	noCQ := fl.Bool("no-cq-gaps", false, "skip empty-critical-questions report")
	noUnderReview := fl.Bool("no-under-review", false, "skip under-review status report")
	noSpinoff := fl.Bool("no-spinoff", false, "skip mining-pass spinoff-candidates aggregation")
	_ = fl.Parse(args)

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("%v", err)
	}

	if !*noCQ {
		emitCQGaps(pool)
	}
	if !*noUnderReview {
		emitUnderReview(pool)
	}
	if !*noSpinoff {
		passesDir := filepath.Join(renderDir, "..", "docs", "passes")
		emitSpinoffCandidates(passesDir)
	}
}

func emitCQGaps(pool map[string]*types.LexEntry) {
	type entry struct{ id, name string }
	var missing []entry
	for _, e := range pool {
		if e == nil {
			continue
		}
		if len(e.CriticalQuestions) == 0 {
			missing = append(missing, entry{e.ID, e.Name})
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].id < missing[j].id })
	fmt.Printf("# Empty critical-questions arrays (%d / %d atoms)\n\n", len(missing), len(pool))
	fmt.Println("Critical-questions surface the falsifiability / defeater axes of an atom; atoms")
	fmt.Println("without them are coverage holes. Adding CQs is a non-graduation enrichment task.")
	fmt.Println()
	for _, m := range missing {
		fmt.Printf("- %s  %s\n", m.id, m.name)
	}
	fmt.Println()
}

func emitUnderReview(pool map[string]*types.LexEntry) {
	type entry struct{ id, name, status string }
	var ur []entry
	for _, e := range pool {
		if e == nil {
			continue
		}
		if strings.TrimSpace(e.Status) == "under-review" {
			ur = append(ur, entry{e.ID, e.Name, e.Status})
		}
	}
	sort.Slice(ur, func(i, j int) bool { return ur[i].id < ur[j].id })
	fmt.Printf("# Under-review atoms (%d)\n\n", len(ur))
	fmt.Println("Atoms staged but not yet graduated to active. Typical graduation gates:")
	fmt.Println("primary-source verbatim quote in lineage; canonical-instances >= 2; CQs populated.")
	fmt.Println()
	for _, e := range ur {
		fmt.Printf("- %s  %s\n", e.id, e.name)
	}
	fmt.Println()
}

// emitSpinoffCandidates walks mining-pass markdown for headers that
// flag unminted candidates. Recognizes a few common shapes used across
// mining-pass docs: "Spinoff candidates noted", "Candidates NOT minted",
// "Transferable candidate atoms (deferred)", "candidates deferred".
// Emits each match as filename + header + the bullet text under it (up
// to the next header).
func emitSpinoffCandidates(passesDir string) {
	hits, err := walkSpinoff(passesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spinoff walk: %v\n", err)
		return
	}
	fmt.Printf("# Flagged-but-unminted candidates across mining-pass docs (%d sections)\n\n", len(hits))
	fmt.Println("Each section was flagged at mining-pass close as a transferable candidate")
	fmt.Println("that wasn't minted in that pass — either deferred, scope-out, or distinctness-audit-dropped.")
	fmt.Println()
	for _, h := range hits {
		fmt.Printf("## %s\n", h.file)
		fmt.Printf("**%s**\n\n", h.header)
		for _, ln := range h.body {
			fmt.Printf("%s\n", ln)
		}
		fmt.Println()
	}
}

type spinoffHit struct {
	file   string
	header string
	body   []string
}

var spinoffMarkers = []string{
	"spinoff candidates",
	"candidates not minted",
	"candidate atoms (deferred)",
	"candidates deferred",
	"candidates dropped",
	"opening-move candidates",
	"follow-on candidates",
	"lift-candidates",
}

func walkSpinoff(dir string) ([]spinoffHit, error) {
	var hits []spinoffHit
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		rel, _ := filepath.Rel(dir, path)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		var (
			inHit   bool
			current spinoffHit
			lines   int
		)
		flush := func() {
			if inHit && len(current.body) > 0 {
				hits = append(hits, current)
			}
			inHit = false
			current = spinoffHit{}
			lines = 0
		}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "## ") {
				flush()
				lower := strings.ToLower(line)
				for _, m := range spinoffMarkers {
					if strings.Contains(lower, m) {
						inHit = true
						current = spinoffHit{
							file:   rel,
							header: strings.TrimPrefix(line, "## "),
						}
						break
					}
				}
				continue
			}
			if inHit {
				if strings.HasPrefix(line, "# ") {
					flush()
					continue
				}
				trimmed := strings.TrimSpace(line)
				if trimmed == "" && len(current.body) == 0 {
					continue
				}
				current.body = append(current.body, line)
				lines++
				if lines >= 40 {
					flush()
				}
			}
		}
		flush()
		return nil
	})
	return hits, err
}
