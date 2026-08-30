package main

// `lexicon renumber` — one-off migration from the sequential
// `lex-NNNN` (4-digit, ingestion-order) id scheme to non-sequential
// 5-character opaque codes drawn from a hand-typing-safe alphabet.
//
// Rationale: the sequential ids visibly encode personal mining-pass order
// (lex-1012 sits after lex-1011 because that's when it was mined), which
// SCHEMA.md's own "stable unique ID, never changes" contract already treats
// as opaque — the migration removes the accidental appearance of order
// rather than granting the ids new meaning.
//
// Mechanism: generate a permanent old->new mapping from the atoms actually
// on disk, sweep the whole tracked tree (plus a short list of gitignored
// working files that are read every session) via a single regex, and
// replace every occurrence that IS a mapping key. Anything matching the
// `lex-\d{4}` shape that is NOT a mapping key — a pre-git placeholder id, a
// synthetic id in a unit-test fixture, a `lex-9999`-style sentinel that was
// never real — is reported as a warning and left untouched. The mapping
// itself is kept forever as `docs/id-migration-map.csv`, since prior
// session notes and cross-session memory reference old ids that would
// otherwise become silently unresolvable.
//
// Split into three subcommands rather than one big --apply so renaming and
// content-rewriting can land as two separate git commits (a pure rename,
// then a content change), removing any dependence on git's rename-
// similarity heuristic:
//
//	lexicon renumber plan            generate (or load) the mapping, sweep,
//	                                  print a report. Writes the mapping CSV
//	                                  as its only side effect.
//	lexicon renumber apply-rename     git mv every elements/<old>.yaml to
//	                                  elements/<new>.yaml. Idempotent —
//	                                  skips pairs already renamed.
//	lexicon renumber apply-content    rewrite every swept occurrence in
//	                                  place, across every in-scope file.
//	lexicon renumber next-id          print one fresh unused 5-char id and
//	                                  exit. No mapping needed, no side
//	                                  effects — for minting new atoms going
//	                                  forward without hand-picking the next
//	                                  sequential integer.
import (
	"crypto/rand"
	"encoding/csv"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// idAlphabet excludes 0/1/i/l/o — visually ambiguous in a hand-typed,
// hand-edited elements. Not literal Crockford base32 (which keeps 0 and 1
// but drops O/I/L/U); this is the simpler ad-hoc set the plan settled on.
const idAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"

const newIDLen = 5

var oldIDRe = regexp.MustCompile(`lex-\d{4}`)

// extraSweepPaths are gitignored working files read every session, in
// scope for the same reason the tracked tree is: staying correct matters
// for personal use, and they're never authoritative sources (docs/passes/
// is deliberately excluded — see render/CONCERNS.md).
var extraSweepPaths = []string{
	"wanted-materials.md",
	"mining-queue.md",
	"acquired-materials.md",
}

func cmdRenumber(renderDir string, args []string) {
	if len(args) == 0 {
		renumberUsage()
		os.Exit(2)
	}
	sub := args[0]
	rest := args[1:]

	repoRoot, err := filepath.Abs(filepath.Join(renderDir, ".."))
	if err != nil {
		fatal("renumber: resolve repo root: %v", err)
	}
	elementsDir := filepath.Join(repoRoot, "elements")
	mapPath := filepath.Join(repoRoot, "docs", "id-migration-map.csv")

	switch sub {
	case "plan":
		cmdRenumberPlan(repoRoot, elementsDir, mapPath, rest)
	case "apply-rename":
		cmdRenumberApplyRename(repoRoot, elementsDir, mapPath, rest)
	case "apply-content":
		cmdRenumberApplyContent(repoRoot, mapPath, rest)
	case "next-id":
		cmdRenumberNextID(elementsDir, mapPath, rest)
	case "-h", "--help", "help":
		renumberUsage()
	default:
		fmt.Fprintf(os.Stderr, "renumber: unknown subcommand %q\n\n", sub)
		renumberUsage()
		os.Exit(2)
	}
}

func renumberUsage() {
	fmt.Fprintln(os.Stderr, `usage: lexicon renumber <plan|apply-rename|apply-content|next-id>

  plan             generate (or load) docs/id-migration-map.csv, sweep the
                   tracked tree + working files, print a report. Dry-run:
                   the only write is the mapping file itself.
  apply-rename     git mv every elements/<old>.yaml to <new>.yaml per the
                   mapping. Idempotent.
  apply-content    rewrite every swept lex-NNNN occurrence that is a
                   mapping key, in place, across every in-scope file.
  next-id          print one fresh unused 5-char id and exit.`)
}

// --- mapping ---

type idMapping struct {
	oldToNew map[string]string
	order    []string // old ids, sorted, for stable CSV/report output
}

func loadOrGenerateMapping(elementsDir, mapPath string) (idMapping, bool, error) {
	if data, err := os.ReadFile(mapPath); err == nil {
		m, perr := parseMappingCSV(data)
		return m, false, perr
	}
	oldIDs, err := scanElementsIDs(elementsDir, oldIDFilenamePattern)
	if err != nil {
		return idMapping{}, false, err
	}
	m := generateMapping(oldIDs)
	if err := writeMappingCSV(mapPath, m); err != nil {
		return idMapping{}, false, err
	}
	return m, true, nil
}

var oldIDFilenamePattern = regexp.MustCompile(`^lex-(\d{4})\.yaml$`)

// scanElementsIDs lists the ids for every elements file whose name
// matches pattern (used both for the pre-migration 4-digit scan and, in
// next-id, a wider new-or-old scan).
func scanElementsIDs(elementsDir string, pattern *regexp.Regexp) ([]string, error) {
	entries, err := os.ReadDir(elementsDir)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", elementsDir, err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !pattern.MatchString(e.Name()) {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(ids)
	return ids, nil
}

func generateMapping(oldIDs []string) idMapping {
	used := make(map[string]bool, len(oldIDs))
	m := idMapping{oldToNew: make(map[string]string, len(oldIDs)), order: append([]string(nil), oldIDs...)}
	for _, old := range oldIDs {
		var n string
		for {
			n = "lex-" + drawCode(newIDLen)
			if !used[n] {
				break
			}
		}
		used[n] = true
		m.oldToNew[old] = n
	}
	return m
}

// drawCode returns n characters drawn uniformly at random from idAlphabet
// via crypto/rand — collisions are handled by the caller's used-set check,
// so this needs no bias-correction beyond per-character uniformity.
func drawCode(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(idAlphabet)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			fatal("renumber: crypto/rand: %v", err)
		}
		b[i] = idAlphabet[idx.Int64()]
	}
	return string(b)
}

func writeMappingCSV(path string, m idMapping) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"old_id", "new_id"}); err != nil {
		return err
	}
	for _, old := range m.order {
		if err := w.Write([]string{old, m.oldToNew[old]}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func parseMappingCSV(data []byte) (idMapping, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	rows, err := r.ReadAll()
	if err != nil {
		return idMapping{}, fmt.Errorf("parse mapping csv: %w", err)
	}
	m := idMapping{oldToNew: map[string]string{}}
	for i, row := range rows {
		if i == 0 || len(row) < 2 {
			continue // header
		}
		m.oldToNew[row[0]] = row[1]
		m.order = append(m.order, row[0])
	}
	sort.Strings(m.order)
	return m, nil
}

// --- sweep ---

type sweepResult struct {
	occurrences map[string]int // file -> count of mapped occurrences rewritten/found
	warnings    map[string][]string
	totalOcc    int
	totalWarn   int
}

func inScopeFiles(repoRoot, mapPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "ls-files")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	mapRel, _ := filepath.Rel(repoRoot, mapPath)
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l == "" || l == mapRel {
			continue
		}
		files = append(files, l)
	}
	for _, extra := range extraSweepPaths {
		if _, err := os.Stat(filepath.Join(repoRoot, extra)); err == nil {
			files = append(files, extra)
		}
	}
	handoffDir := filepath.Join(repoRoot, ".session-handoffs")
	if hs, err := os.ReadDir(handoffDir); err == nil {
		for _, h := range hs {
			if !h.IsDir() && strings.HasSuffix(h.Name(), ".md") {
				files = append(files, filepath.Join(".session-handoffs", h.Name()))
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

// sweep scans every in-scope file for lex-\d{4} occurrences. If apply is
// true, mapped occurrences are rewritten in place; unmapped tokens are
// always left untouched and always reported, never silently touched.
func sweep(repoRoot string, files []string, m idMapping, apply bool) (sweepResult, error) {
	res := sweepResult{occurrences: map[string]int{}, warnings: map[string][]string{}}
	for _, rel := range files {
		abs := filepath.Join(repoRoot, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			continue // deleted-but-still-tracked race, or a symlink target gone; skip
		}
		content := string(data)
		if !strings.Contains(content, "lex-") {
			continue
		}
		matches := oldIDRe.FindAllString(content, -1)
		if len(matches) == 0 {
			continue
		}
		mappedCount := 0
		for _, tok := range matches {
			if _, ok := m.oldToNew[tok]; ok {
				mappedCount++
			} else {
				res.warnings[rel] = append(res.warnings[rel], tok)
				res.totalWarn++
			}
		}
		if mappedCount > 0 {
			res.occurrences[rel] = mappedCount
			res.totalOcc += mappedCount
		}
		if apply && mappedCount > 0 {
			rewritten := oldIDRe.ReplaceAllStringFunc(content, func(tok string) string {
				if n, ok := m.oldToNew[tok]; ok {
					return n
				}
				return tok
			})
			if err := os.WriteFile(abs, []byte(rewritten), 0644); err != nil {
				return res, fmt.Errorf("write %s: %w", abs, err)
			}
		}
	}
	return res, nil
}

func printSweepReport(res sweepResult) {
	files := make([]string, 0, len(res.occurrences))
	for f := range res.occurrences {
		files = append(files, f)
	}
	sort.Strings(files)
	fmt.Printf("=== occurrences (%d files, %d total) ===\n", len(files), res.totalOcc)
	for _, f := range files {
		fmt.Printf("  %-60s %d\n", f, res.occurrences[f])
	}
	warnFiles := make([]string, 0, len(res.warnings))
	for f := range res.warnings {
		warnFiles = append(warnFiles, f)
	}
	sort.Strings(warnFiles)
	fmt.Printf("=== unmapped tokens (%d files, %d total) — left untouched ===\n", len(warnFiles), res.totalWarn)
	for _, f := range warnFiles {
		toks := res.warnings[f]
		sort.Strings(toks)
		fmt.Printf("  %-60s %s\n", f, strings.Join(uniqueStrings(toks), ", "))
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// --- subcommands ---

func cmdRenumberPlan(repoRoot, elementsDir, mapPath string, args []string) {
	m, generated, err := loadOrGenerateMapping(elementsDir, mapPath)
	if err != nil {
		fatal("renumber plan: %v", err)
	}
	if generated {
		fmt.Printf("renumber plan: generated fresh mapping for %d atoms -> %s\n", len(m.order), mapPath)
	} else {
		fmt.Printf("renumber plan: loaded existing mapping (%d atoms) from %s\n", len(m.order), mapPath)
	}
	files, err := inScopeFiles(repoRoot, mapPath)
	if err != nil {
		fatal("renumber plan: %v", err)
	}
	res, err := sweep(repoRoot, files, m, false)
	if err != nil {
		fatal("renumber plan: %v", err)
	}
	printSweepReport(res)
	fmt.Printf("\nswept %d in-scope files. Next: `lexicon renumber apply-rename`, then `apply-content`.\n", len(files))
}

func cmdRenumberApplyRename(repoRoot, elementsDir, mapPath string, args []string) {
	m, err := readMappingOrFatal(mapPath)
	if err != nil {
		fatal("renumber apply-rename: %v", err)
	}
	renamed, skipped := 0, 0
	for _, old := range m.order {
		newID := m.oldToNew[old]
		oldPath := filepath.Join(elementsDir, old+".yaml")
		if _, err := os.Stat(oldPath); err != nil {
			skipped++
			continue
		}
		cmd := exec.Command("git", "-C", repoRoot, "mv", filepath.Join("elements", old+".yaml"), filepath.Join("elements", newID+".yaml"))
		if out, err := cmd.CombinedOutput(); err != nil {
			fatal("renumber apply-rename: git mv %s -> %s: %v\n%s", old, newID, err, out)
		}
		renamed++
	}
	fmt.Printf("renumber apply-rename: renamed %d, skipped %d (already renamed)\n", renamed, skipped)
}

func cmdRenumberApplyContent(repoRoot, mapPath string, args []string) {
	m, err := readMappingOrFatal(mapPath)
	if err != nil {
		fatal("renumber apply-content: %v", err)
	}
	files, err := inScopeFiles(repoRoot, mapPath)
	if err != nil {
		fatal("renumber apply-content: %v", err)
	}
	res, err := sweep(repoRoot, files, m, true)
	if err != nil {
		fatal("renumber apply-content: %v", err)
	}
	printSweepReport(res)
	fmt.Printf("\nrewrote %d occurrences across %d files.\n", res.totalOcc, len(res.occurrences))
}

func readMappingOrFatal(mapPath string) (idMapping, error) {
	data, err := os.ReadFile(mapPath)
	if err != nil {
		return idMapping{}, fmt.Errorf("no mapping at %s — run `lexicon renumber plan` first: %w", mapPath, err)
	}
	return parseMappingCSV(data)
}

func cmdRenumberNextID(elementsDir, mapPath string, args []string) {
	used := map[string]bool{}
	// current elements, whatever format its ids are in mid-migration
	if entries, err := os.ReadDir(elementsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				used[strings.TrimSuffix(e.Name(), ".yaml")] = true
			}
		}
	}
	// any new ids already assigned in an in-progress mapping, even before
	// apply-rename has landed them on disk
	if data, err := os.ReadFile(mapPath); err == nil {
		if m, err := parseMappingCSV(data); err == nil {
			for _, n := range m.oldToNew {
				used[n] = true
			}
		}
	}
	for {
		cand := "lex-" + drawCode(newIDLen)
		if !used[cand] {
			fmt.Println(cand)
			return
		}
	}
}
