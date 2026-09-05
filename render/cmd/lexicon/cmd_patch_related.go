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
)

// cmdPatchRelated does a safe line-level add or remove on an atom's
// `related:` list in its elements/<id>.yaml file.
//
// Rationale: V109 c hand-rolled a block-replace regex in a python
// reciprocation helper that wiped sibling atoms in multi-atom mining-pass
// MDs (seed-primitives-pass.md lost 27 sibling atoms; mythology-pass.md
// lost 2 of 3; etc.). Recovery worked because the destruction was caught
// pre-commit and reverted via `git checkout HEAD`, but the recipe for the
// destruction lives only in a memory note nobody is forced to read. This
// subcommand is the deterministic replacement: it patches ONLY the
// `related:` field, never the surrounding block, and it locates it by
// walking from the atom's id-line forward — no block-boundary regex to
// get wrong.
//
// YAML-only as of the 2026-08-19 DB-migration pass: this used to also
// patch the matching `related:` line in the atom's source mining-pass
// markdown (docs/passes/*.md), which is gitignored and per-machine —
// patching it never made the change any more shared than the YAML alone,
// and the same unbounded id-to-related-line window scan that's safe
// within one atom's own YAML file (each elements file holds exactly one
// atom) was NOT safe against a multi-atom MD file with no boundary check,
// which is exactly how a target atom's own `related:` line going missing
// or non-inline could silently patch a different atom's line instead.
//
// Usage:
//
//	lexicon patch-related <lex-id> <add|remove> <ref-id> [<ref-id> ...]
//
// Examples:
//
//	lexicon patch-related lex-qhydj add lex-33p2a lex-zrhqq
//	lexicon patch-related lex-qhydj remove lex-33p2a
//
// Behavior:
//   - Adds are deduplicated against the current `related:` list.
//   - The resulting list is re-sorted lexicographically. Ids stopped being
//     numeric with the 2026-08-20 renumbering (opaque 5-char codes), so
//     there's no meaningful numeric order left to sort by.
//   - Removes silently skip refs that aren't in the list.
//   - Both `related: [lex-0001, lex-0002]` (inline) and
//     `related:\n  - lex-0001\n  - lex-0002` (block-list) source forms are
//     read; the field is always rewritten in inline form, since that's
//     the elements' dominant convention.
func cmdPatchRelated(renderDir string, args []string) {
	fl := flag.NewFlagSet("patch-related", flag.ExitOnError)
	dryRun := fl.Bool("dry-run", false, "print the would-be new related[] line; do not write")
	fl.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lexicon patch-related <lex-id> <add|remove> <ref-id> [<ref-id> ...]")
		fmt.Fprintln(os.Stderr, "  Line-level patch the related: list of an atom (YAML + source MD).")
		fl.PrintDefaults()
	}
	args = reorderFlagsFirst(args)
	_ = fl.Parse(args)
	if fl.NArg() < 3 {
		fl.Usage()
		os.Exit(2)
	}
	target := fl.Arg(0)
	op := fl.Arg(1)
	refs := fl.Args()[2:]

	if op != "add" && op != "remove" {
		fatal("patch-related: op must be 'add' or 'remove', got %q", op)
	}
	if !lexIDPattern.MatchString(target) {
		fatal("patch-related: target %q is not a valid lex-id", target)
	}
	for _, r := range refs {
		if !lexIDPattern.MatchString(r) {
			fatal("patch-related: ref %q is not a valid lex-id", r)
		}
		if r == target {
			fatal("patch-related: cannot reference self (%s)", r)
		}
	}

	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	yamlPath := filepath.Join(elementsDir, target+".yaml")
	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		fatal("patch-related: cannot read %s: %v", yamlPath, err)
	}
	yamlContent := string(yamlBytes)

	block, ok := extractRelatedBlock(yamlContent, target)
	if !ok {
		fatal("patch-related: %s: no `related:` field found after id-line", yamlPath)
	}

	merged := applyRelatedOp(block.refs, op, refs)
	if equalStringSlices(block.refs, merged) {
		fmt.Fprintf(os.Stderr, "patch-related: %s: related list unchanged (no-op)\n", target)
		return
	}
	newLine := formatRelatedLine(merged)

	if *dryRun {
		fmt.Printf("would set %s.related = %s\n", target, newLine)
		return
	}

	if err := writeRelatedBlock(yamlPath, yamlContent, block, newLine); err != nil {
		fatal("patch-related: write %s: %v", yamlPath, err)
	}

	fmt.Fprintf(os.Stderr, "patch-related: %s: %d ref(s), new related = %s\n  YAML: %s\n",
		target, len(merged), newLine, yamlPath)
}

var (
	lexIDPattern              = regexp.MustCompile(`^lex-[23456789abcdefghjkmnpqrstuvwxyz]{5}$`)
	relatedInlinePattern      = regexp.MustCompile(`^related:\s*\[([^\]]*)\]\s*$`)
	relatedBlockHeaderPattern = regexp.MustCompile(`^related:\s*$`)
	relatedBlockItemPattern   = regexp.MustCompile(`^\s*-\s*(lex-[23456789abcdefghjkmnpqrstuvwxyz]{5})\s*$`)
)

// relatedBlock is the located `related:` field in a YAML file: its line
// range (startLine inclusive, endLine exclusive) and the parsed refs,
// regardless of whether the source used inline or block-list form.
type relatedBlock struct {
	startLine int
	endLine   int
	refs      []string
}

// extractRelatedBlock finds the `related:` field that follows the first
// `id: <target>` line, in either inline (`related: [a, b]`) or block-list
// (`related:\n  - a\n  - b`) form.
func extractRelatedBlock(content, target string) (relatedBlock, bool) {
	lines := strings.Split(content, "\n")
	idLine := "id: " + target
	idIdx := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == idLine {
			idIdx = i
			break
		}
	}
	if idIdx < 0 {
		return relatedBlock{}, false
	}
	for i := idIdx + 1; i < len(lines) && i < idIdx+200; i++ {
		if m := relatedInlinePattern.FindStringSubmatch(lines[i]); m != nil {
			return relatedBlock{startLine: i, endLine: i + 1, refs: splitRelated(m[1])}, true
		}
		if relatedBlockHeaderPattern.MatchString(lines[i]) {
			var refs []string
			j := i + 1
			for ; j < len(lines); j++ {
				m := relatedBlockItemPattern.FindStringSubmatch(lines[j])
				if m == nil {
					break
				}
				refs = append(refs, m[1])
			}
			return relatedBlock{startLine: i, endLine: j, refs: refs}, true
		}
	}
	return relatedBlock{}, false
}

// writeRelatedBlock replaces block's line range in path with a single
// inline related: line — the field is always normalized to inline form
// on write, regardless of which form it was read in.
func writeRelatedBlock(path, content string, block relatedBlock, newLine string) error {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines)-(block.endLine-block.startLine)+1)
	out = append(out, lines[:block.startLine]...)
	out = append(out, newLine)
	out = append(out, lines[block.endLine:]...)
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

func splitRelated(inner string) []string {
	parts := strings.Split(inner, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func applyRelatedOp(current []string, op string, refs []string) []string {
	set := map[string]bool{}
	for _, c := range current {
		set[c] = true
	}
	switch op {
	case "add":
		for _, r := range refs {
			set[r] = true
		}
	case "remove":
		for _, r := range refs {
			delete(set, r)
		}
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	// Migrated 2026-08-20: ids are opaque random codes now, so numeric
	// order (lex-jv983 before lex-4yhqs) no longer means anything. Plain
	// lexicographic order is the only ordering left that's stable and
	// reproducible.
	sort.Strings(out)
	return out
}

func formatRelatedLine(refs []string) string {
	return "related: [" + strings.Join(refs, ", ") + "]"
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
