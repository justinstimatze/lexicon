package main

// cmd_redaction.go — generic content-pattern audit + PreToolUse hook.
// Two surfaces:
//
//	lexicon redaction-audit          scan tracked elements/docs against
//	                                 the project-local pattern set; exit
//	                                 1 if any hits.
//	lexicon redaction-hook           Claude Code PreToolUse:Write|Edit
//	                                 hook; reads JSON from stdin; exit 2
//	                                 (block) if proposed content matches
//	                                 any pattern; exit 0 otherwise.
//
// Patterns are loaded at runtime from a project-local config file at
//
//	$LEXICON_REDACTION_PATTERNS  (default: ./.lexicon/redaction-patterns.txt)
//
// Each non-blank, non-comment line is `name|regex`. Patterns are
// applied case-insensitively. If the file is absent or unreadable, the
// audit and the hook no-op silently.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type redactionRule struct {
	Name string
	Re   *regexp.Regexp
}

func loadRedactionRules(scanDir string) []redactionRule {
	path := os.Getenv("LEXICON_REDACTION_PATTERNS")
	if path == "" {
		// Look for .lexicon/ at the scan-dir root (typical: --dir .. from
		// render/ resolves to repo root, where .lexicon/ lives) before
		// falling back to CWD.
		candidate := filepath.Join(scanDir, ".lexicon", "redaction-patterns.txt")
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
		} else {
			path = filepath.Join(".lexicon", "redaction-patterns.txt")
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var rules []redactionRule
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '|')
		if i <= 0 || i == len(line)-1 {
			continue
		}
		name := strings.TrimSpace(line[:i])
		expr := strings.TrimSpace(line[i+1:])
		re, err := regexp.Compile(`(?i)` + expr)
		if err != nil {
			continue
		}
		rules = append(rules, redactionRule{Name: name, Re: re})
	}
	return rules
}

type redactionHit struct {
	Path    string
	Line    int
	Pattern string
	Excerpt string
}

func cmdRedactionAudit(args []string) {
	fs := flag.NewFlagSet("redaction-audit", flag.ContinueOnError)
	root := fs.String("dir", ".", "directory to scan recursively (defaults to repo root)")
	target := fs.String("target", "", "single file to scan (overrides --dir)")
	quiet := fs.Bool("quiet", false, "print only hits (no per-file summary)")
	if err := fs.Parse(args); err != nil {
		return
	}

	rules := loadRedactionRules(*root)
	if len(rules) == 0 {
		if !*quiet {
			fmt.Println("redaction-audit: no patterns configured (set LEXICON_REDACTION_PATTERNS or populate .lexicon/redaction-patterns.txt)")
		}
		return
	}

	var files []string
	if *target != "" {
		files = []string{*target}
	} else {
		files = collectRedactionTargets(*root)
	}

	total := 0
	for _, path := range files {
		hits := scanRedactionFile(path, rules)
		if !*quiet && len(hits) > 0 {
			fmt.Printf("%s — %d hit(s)\n", path, len(hits))
		}
		for _, h := range hits {
			fmt.Printf("  %s:%d  pattern=%s\n", h.Path, h.Line, h.Pattern)
			fmt.Printf("    %s\n", trimForRedactionDisplay(h.Excerpt, 160))
		}
		total += len(hits)
	}
	if total > 0 {
		fmt.Fprintf(os.Stderr, "\nredaction-audit: %d hit(s) across %d file(s)\n", total, len(files))
		os.Exit(1)
	}
	if !*quiet {
		fmt.Printf("redaction-audit: clean across %d file(s)\n", len(files))
	}
}

func collectRedactionTargets(root string) []string {
	excludedBasenames := map[string]bool{
		"wanted-materials.md":   true,
		"acquired-materials.md": true,
		"mining-queue.md":       true,
		"CLAUDE.md":             true,
	}
	var out []string
	roots := []string{
		filepath.Join(root, "elements"),
		filepath.Join(root, "docs"),
		filepath.Join(root, "README.md"),
	}
	for _, r := range roots {
		info, err := os.Stat(r)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if !excludedBasenames[filepath.Base(r)] {
				out = append(out, r)
			}
			continue
		}
		_ = filepath.WalkDir(r, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			// Skip gitignored working dirs at the directory level so the
			// walk doesn't even descend into them — these hold working
			// state that doesn't appear in the published repo.
			if d.IsDir() {
				base := filepath.Base(p)
				if base == "passes" || base == "working" || base == "audits" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".md") && !strings.HasSuffix(p, ".yaml") {
				return nil
			}
			base := filepath.Base(p)
			if excludedBasenames[base] {
				return nil
			}
			if strings.HasPrefix(base, "SESSION-") && strings.HasSuffix(base, "-HANDOFF.md") {
				return nil
			}
			out = append(out, p)
			return nil
		})
	}
	return out
}

func scanRedactionFile(path string, rules []redactionRule) []redactionHit {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return scanRedactionContent(path, string(raw), rules)
}

func scanRedactionContent(path, content string, rules []redactionRule) []redactionHit {
	var out []redactionHit
	for _, r := range rules {
		for _, idx := range r.Re.FindAllStringIndex(content, -1) {
			line := 1 + strings.Count(content[:idx[0]], "\n")
			start := idx[0] - 30
			if start < 0 {
				start = 0
			}
			end := idx[1] + 30
			if end > len(content) {
				end = len(content)
			}
			out = append(out, redactionHit{
				Path:    path,
				Line:    line,
				Pattern: r.Name,
				Excerpt: content[start:end],
			})
		}
	}
	return out
}

func trimForRedactionDisplay(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---- PreToolUse hook (Claude Code) ----

type redactionHookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type redactionWriteInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type redactionEditInput struct {
	FilePath  string `json:"file_path"`
	NewString string `json:"new_string"`
}

type redactionMultiEditInput struct {
	FilePath string `json:"file_path"`
	Edits    []struct {
		NewString string `json:"new_string"`
	} `json:"edits"`
}

func cmdRedactionHook() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(0)
	}
	var in redactionHookInput
	if json.Unmarshal(raw, &in) != nil {
		os.Exit(0)
	}

	var filePath, newContent string
	switch in.ToolName {
	case "Write":
		var w redactionWriteInput
		if json.Unmarshal(in.ToolInput, &w) != nil {
			os.Exit(0)
		}
		filePath = w.FilePath
		newContent = w.Content
	case "Edit":
		var e redactionEditInput
		if json.Unmarshal(in.ToolInput, &e) != nil {
			os.Exit(0)
		}
		filePath = e.FilePath
		newContent = e.NewString
	case "MultiEdit":
		var m redactionMultiEditInput
		if json.Unmarshal(in.ToolInput, &m) != nil {
			os.Exit(0)
		}
		filePath = m.FilePath
		var b strings.Builder
		for _, ed := range m.Edits {
			b.WriteString(ed.NewString)
			b.WriteString("\n")
		}
		newContent = b.String()
	default:
		os.Exit(0)
	}

	if !redactionInScope(filePath) {
		os.Exit(0)
	}

	// PreToolUse hook: the patterns file lives at REPO_ROOT/.lexicon/.
	// Discover the root by walking up from the file path; fall back to
	// CWD-relative resolution inside loadRedactionRules if not found.
	rules := loadRedactionRules(findRepoRoot(filepath.Dir(filePath)))
	if len(rules) == 0 {
		os.Exit(0)
	}

	hits := scanRedactionContent(filePath, newContent, rules)
	if len(hits) == 0 {
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "lexicon redaction-hook: blocked %s — proposed content matches configured redaction pattern(s):\n", filePath)
	seen := map[string]bool{}
	for _, h := range hits {
		if seen[h.Pattern] {
			continue
		}
		seen[h.Pattern] = true
		fmt.Fprintf(os.Stderr, "  pattern=%s\n", h.Pattern)
	}
	fmt.Fprintln(os.Stderr, "  (rewrite to remove matched content, then retry)")
	os.Exit(2)
}

func redactionInScope(file string) bool {
	if file == "" {
		return false
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	base := filepath.Base(abs)
	if base == "wanted-materials.md" || base == "acquired-materials.md" || base == "mining-queue.md" || base == "CLAUDE.md" {
		return false
	}
	if strings.HasPrefix(base, "SESSION-") && strings.HasSuffix(base, "-HANDOFF.md") {
		return false
	}
	if strings.HasSuffix(abs, ".yaml") && strings.Contains(abs, string(filepath.Separator)+"elements"+string(filepath.Separator)) {
		return true
	}
	if !strings.HasSuffix(abs, ".md") {
		return false
	}
	if base == "README.md" {
		return true
	}
	for _, seg := range []string{
		string(filepath.Separator) + "docs" + string(filepath.Separator),
	} {
		if strings.Contains(abs, seg) {
			return true
		}
	}
	return false
}
