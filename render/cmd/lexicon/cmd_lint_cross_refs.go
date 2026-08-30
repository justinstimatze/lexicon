package main

// `lexicon lint-cross-refs` — unified cross-reference linter.
//
// Replaces both scripts/check-reciprocation.py and scripts/audit-stale-refs.py
// with a single Go subcommand that runs in one pass.
//
// What it checks:
//   1. Stale `lex-NNNN` references in any tracked .md or elements .yaml file
//      where the referenced ID is not in the current elements.
//   2. Concept-mismatch — a `lex-NNNN <name>` cite where <name> is the actual
//      name of a DIFFERENT atom, so the ID and the sentence disagree about
//      which atom is meant. Any other divergence between cited and actual name
//      is shorthand and reported only under --show-shorthand.
//   3. Reciprocation — every related[] target must list the source back.
//
// Algorithm: O(atoms + tracked-files + edges).
//   - Build ID→name map once via loader.LoadAll (O(N)).
//   - Walk tracked files via `git ls-files` (gitignore-aware) and scan each
//     once with a single precompiled regex.
//   - Reciprocation is HashMap O(1) lookups across the related[] graph.
//
// Exit code: non-zero if any errors. Wired into the pre-commit hook.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/loader"
)

// Matches `lex-NNNN` optionally followed by a kebab-form atom name. Group
// 1 = id; Group 2 = cited name (requires at least one hyphen) if present.
// The hyphen requirement filters single-word English prose like "lex-sp7fa
// motivates" — those aren't citations, just sentences using the id as a
// noun. Real shorthand cites ("lex-2kjmb free-energy-principle") all have
// hyphens and still match.
// \b-anchored (migration 2026-08-20): see db/sync.go's proseLexRefRe comment
// — the new alphabet spans most of a-z, so an unanchored match false-
// positives inside compound words ("complex-systems" contains the literal
// substring "lex-syste").
var crossRefPat = regexp.MustCompile(`\b(lex-[23456789abcdefghjkmnpqrstuvwxyz]{5})\b(?:[ \t]+([a-z][a-z0-9]*(?:-[a-z0-9]+)+))?`)

// Schema field names. "lex-8ennw agent-instruction" points at a FIELD of an
// atom, not at a name for it, so there is nothing to compare against.
var schemaFields = map[string]bool{
	"agent-instruction": true, "canonical-instances": true,
	"canonical-instances-edge": true, "common-name": true,
	"critical-questions": true, "decomposes-into": true,
	"encounter-tier-override": true, "formal-if-any": true,
	"pedagogy-gloss": true, "scaffolds-from": true, "severity-tier": true,
	"type-in": true, "type-out": true,
}

// IDs retired before the elements entered git. The first commit touching
// elements/ already holds 1024 atoms and starts at lex-75r77; `git log` has
// never seen a file for any ID below it, so these are not deletions anyone
// can trace — they are an earlier numbering that was pruned in the rebuild.
// The design notes in docs/principles/ were written against it and record
// decisions made at that time; remapping their IDs to today's nearest atom
// would half-modernise a historical sentence. A closed set: it cannot grow,
// because anything deleted from here on IS traceable and should be fixed.
var preGitIDs = map[string]bool{
	"lex-0001": true, "lex-0002": true, "lex-0003": true, "lex-0021": true,
	"lex-0042": true, "lex-0045": true, "lex-0046": true, "lex-0047": true,
	"lex-0048": true,
}

type crossRefFinding struct {
	Severity   string `json:"severity"`
	Code       string `json:"code"`
	File       string `json:"file"`
	Line       int    `json:"line,omitempty"`
	RefID      string `json:"ref_id"`
	CitedName  string `json:"cited_name,omitempty"`
	ActualName string `json:"actual_name,omitempty"`
	Message    string `json:"message"`
}

func cmdLintCrossRefs(renderDir string, args []string) {
	fl := flag.NewFlagSet("lint-cross-refs", flag.ExitOnError)
	jsonOut := fl.Bool("json", false, "emit JSONL diagnostics")
	showShorthand := fl.Bool("show-shorthand", false, "include shorthand cites (info-level)")
	strict := fl.Bool("strict", false, "promote stale-id findings (in prose) to errors (default: warnings)")
	fl.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lexicon lint-cross-refs [flags]")
		fmt.Fprintln(os.Stderr, "  Stale lex-NNNN IDs in tracked MDs + YAMLs, concept-mismatch (cited name")
		fmt.Fprintln(os.Stderr, "  != actual), and related[] reciprocation. Exits non-zero on any error.")
		fl.PrintDefaults()
	}
	_ = fl.Parse(args)

	repoRoot, err := filepath.Abs(filepath.Join(renderDir, ".."))
	if err != nil {
		fatal("resolve repo root: %v", err)
	}
	elementsDir := filepath.Join(repoRoot, "elements")
	sub, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("load elements: %v", err)
	}

	idToName := make(map[string]string, len(sub))
	nameToID := make(map[string]string, len(sub))
	for id, e := range sub {
		idToName[id] = e.Name
		nameToID[e.Name] = id
	}

	var findings []crossRefFinding

	// Reciprocation check.
	atomIDs := make([]string, 0, len(sub))
	for id := range sub {
		atomIDs = append(atomIDs, id)
	}
	sort.Strings(atomIDs)
	edgeCount := 0
	for _, a := range atomIDs {
		for _, b := range sub[a].Related {
			edgeCount++
			target, ok := sub[b]
			if !ok {
				findings = append(findings, crossRefFinding{
					Severity: "error",
					Code:     "stale-related-target",
					File:     filepath.Join("elements", a+".yaml"),
					RefID:    b,
					Message:  fmt.Sprintf("%s.related cites %s, not in elements", a, b),
				})
				continue
			}
			reciprocated := false
			for _, r := range target.Related {
				if r == a {
					reciprocated = true
					break
				}
			}
			if !reciprocated {
				findings = append(findings, crossRefFinding{
					Severity: "error",
					Code:     "unreciprocated-edge",
					File:     filepath.Join("elements", a+".yaml"),
					RefID:    b,
					Message:  fmt.Sprintf("%s -> %s but %s lacks back-reference", a, b, b),
				})
			}
		}
	}

	// Tracked-file scan via git ls-files (gitignore-aware, fast).
	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "*.md", "elements/*.yaml")
	out, err := cmd.Output()
	if err != nil {
		fatal("git ls-files: %v", err)
	}
	rawFiles := strings.Split(strings.TrimSpace(string(out)), "\n")

	for _, relPath := range rawFiles {
		if relPath == "" {
			continue
		}
		absPath := filepath.Join(repoRoot, relPath)
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		isMD := strings.HasSuffix(relPath, ".md")
		inFence := false
		for lineIdx, line := range lines {
			// A fenced block in a .md is an example, not a citation. SCHEMA.md
			// illustrates the format with `id: lex-0001 / related: [lex-0002,
			// lex-0003]`; those IDs are placeholders and always will be.
			if isMD && strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			if !strings.Contains(line, "lex-") {
				continue
			}
			matches := crossRefPat.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				refID := m[1]
				citedName := ""
				if len(m) > 2 {
					citedName = m[2]
				}
				if schemaFields[citedName] {
					citedName = ""
				}
				actualName, exists := idToName[refID]
				if !exists {
					if preGitIDs[refID] {
						if *showShorthand {
							findings = append(findings, crossRefFinding{
								Severity:  "info",
								Code:      "pre-git-id",
								File:      relPath,
								Line:      lineIdx + 1,
								RefID:     refID,
								CitedName: citedName,
								Message:   fmt.Sprintf("%s belongs to the pre-rebuild numbering", refID),
							})
						}
						continue
					}
					sev := "warning"
					if *strict {
						sev = "error"
					}
					findings = append(findings, crossRefFinding{
						Severity:  sev,
						Code:      "stale-id",
						File:      relPath,
						Line:      lineIdx + 1,
						RefID:     refID,
						CitedName: citedName,
						Message:   fmt.Sprintf("%s not in current elements", refID),
					})
					continue
				}
				if citedName == "" || citedName == actualName {
					continue
				}
				// Only ONE disagreement is worth a warning: the cited name is
				// the real name of a DIFFERENT atom, so the sentence and the ID
				// point at two things and a reader following either is misled.
				// Everything else is conceptual shorthand — lex-mwgep called
				// 'path-dependence' when its formal name is
				// positive-feedback-amplifies-early-events-into-locked-in-trajectories
				// tells a reader more, not less. 565 warnings collapse to 4.
				if otherID, isAtomName := nameToID[citedName]; isAtomName {
					findings = append(findings, crossRefFinding{
						Severity:   "error",
						Code:       "concept-mismatch",
						File:       relPath,
						Line:       lineIdx + 1,
						RefID:      refID,
						CitedName:  citedName,
						ActualName: actualName,
						Message: fmt.Sprintf("%s cited as '%s', which is the name of %s; %s is '%s'",
							refID, citedName, otherID, refID, actualName),
					})
				} else if *showShorthand {
					findings = append(findings, crossRefFinding{
						Severity:   "info",
						Code:       "shorthand",
						File:       relPath,
						Line:       lineIdx + 1,
						RefID:      refID,
						CitedName:  citedName,
						ActualName: actualName,
						Message:    fmt.Sprintf("%s cited as '%s' (actual: '%s')", refID, citedName, actualName),
					})
				}
			}
		}
	}

	errCount := 0
	warnCount := 0
	infoCount := 0
	for _, f := range findings {
		switch f.Severity {
		case "error":
			errCount++
		case "warning":
			warnCount++
		default:
			infoCount++
		}
	}

	if *jsonOut {
		for _, f := range findings {
			j, _ := json.Marshal(f)
			fmt.Println(string(j))
		}
	} else {
		byCode := make(map[string][]crossRefFinding)
		for _, f := range findings {
			byCode[f.Code] = append(byCode[f.Code], f)
		}
		order := []string{"stale-id", "concept-mismatch", "stale-related-target", "unreciprocated-edge", "shorthand", "pre-git-id"}
		for _, code := range order {
			entries := byCode[code]
			if len(entries) == 0 {
				continue
			}
			fmt.Printf("=== %s (%d) ===\n", code, len(entries))
			for _, f := range entries {
				if f.Line > 0 {
					fmt.Printf("  %s:%d  %s\n", f.File, f.Line, f.Message)
				} else {
					fmt.Printf("  %s  %s\n", f.File, f.Message)
				}
			}
		}
	}
	fmt.Printf("cross-refs: %d edges across %d atoms; %d error(s), %d warning(s), %d info\n",
		edgeCount, len(sub), errCount, warnCount, infoCount)
	if errCount > 0 {
		os.Exit(1)
	}
}
