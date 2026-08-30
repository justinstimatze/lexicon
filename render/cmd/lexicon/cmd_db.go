package main

// cmd_db.go — `lexicon db` subcommands. Build and query the SQLite
// structural index. YAMLs remain canonical; the DB is a derived view
// for cheap cross-citation queries, adjacency traversal, and bulk lints
// that don't scale on full-elements walks.
//
// Phase A: DB is derived, rebuilt with `lexicon db build`. Phase B (when
// elements stops being editable as YAML — likely thousands of atoms in)
// flips the source-of-truth; YAMLs become export.

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/db"
	"github.com/justinstimatze/lexicon/render/internal/loader"
)

func cmdDB(renderDir string, args []string) {
	if len(args) == 0 {
		dbUsage()
		os.Exit(2)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "build":
		dbBuild(renderDir, rest)
	case "query":
		dbQuery(renderDir, rest)
	case "stale-citations":
		dbStaleCitations(renderDir, rest)
	case "stats":
		dbStats(renderDir, rest)
	case "schema":
		dbSchema(renderDir, rest)
	case "lint":
		dbLint(renderDir, rest)
	case "fix-stale-citations":
		dbFixStaleCitations(renderDir, rest)
	default:
		fmt.Fprintf(os.Stderr, "lexicon db: unknown subcommand %q\n", sub)
		dbUsage()
		os.Exit(2)
	}
}

func dbUsage() {
	fmt.Fprintln(os.Stderr, "usage: lexicon db <subcommand>")
	fmt.Fprintln(os.Stderr, "  build [--out path]            rebuild the SQLite index from elements/")
	fmt.Fprintln(os.Stderr, "  query <SQL>                   run a SQL query against the index")
	fmt.Fprintln(os.Stderr, "  stats                         elements-health metrics (counts, top in-degree, lineage)")
	fmt.Fprintln(os.Stderr, "  schema                        dump SQL schema (agent-readable structure)")
	fmt.Fprintln(os.Stderr, "  stale-citations [--target id] list stale cross-citations (--target filters)")
	fmt.Fprintln(os.Stderr, "  lint [--json] [--verbose]     run elements-health gates (SQL-backed)")
	fmt.Fprintln(os.Stderr, "  fix-stale-citations [--apply] strip stale kebab-names after lex-NNNN refs (dry-run default)")
}

func dbBuild(renderDir string, args []string) {
	fl := flag.NewFlagSet("db build", flag.ExitOnError)
	out := fl.String("out", "", "output SQLite path (default render/../lexicon.db)")
	_ = fl.Parse(args)

	path := *out
	if path == "" {
		path = filepath.Join(renderDir, "..", "lexicon.db")
	}

	elementsDir := filepath.Join(renderDir, "..", "elements")
	sub, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("load elements: %v", err)
	}

	if err := db.Build(path, sub); err != nil {
		fatal("build db: %v", err)
	}
	fmt.Printf("db: built %d atoms -> %s\n", len(sub), path)
}

func dbQuery(renderDir string, args []string) {
	fl := flag.NewFlagSet("db query", flag.ExitOnError)
	dbPath := fl.String("db", "", "SQLite path (default render/../lexicon.db)")
	_ = fl.Parse(args)
	if fl.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: lexicon db query <SQL>")
		os.Exit(2)
	}
	sqlStr := strings.Join(fl.Args(), " ")
	path := *dbPath
	if path == "" {
		path = filepath.Join(renderDir, "..", "lexicon.db")
	}

	d, err := db.Open(path)
	if err != nil {
		fatal("open db: %v", err)
	}
	defer func() { _ = d.Close() }()

	rows, err := d.Query(sqlStr)
	if err != nil {
		fatal("query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	cols, _ := rows.Columns()
	fmt.Println(strings.Join(cols, "\t"))
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fatal("scan: %v", err)
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = formatScanned(v)
		}
		fmt.Println(strings.Join(parts, "\t"))
	}
}

func formatScanned(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	case string:
		return x
	default:
		return fmt.Sprintf("%v", v)
	}
}

func dbStats(renderDir string, args []string) {
	fl := flag.NewFlagSet("db stats", flag.ExitOnError)
	dbPath := fl.String("db", "", "SQLite path (default render/../lexicon.db)")
	_ = fl.Parse(args)
	path := *dbPath
	if path == "" {
		path = filepath.Join(renderDir, "..", "lexicon.db")
	}
	d, err := db.Open(path)
	if err != nil {
		fatal("open db: %v", err)
	}
	defer func() { _ = d.Close() }()

	section := func(title string) { fmt.Printf("\n== %s ==\n", title) }

	printQuery := func(q string) {
		rows, err := d.Query(q)
		if err != nil {
			fmt.Fprintf(os.Stderr, "query failed: %v\n", err)
			return
		}
		defer func() { _ = rows.Close() }()
		cols, _ := rows.Columns()
		fmt.Println(strings.Join(cols, "\t"))
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rows.Next() {
			_ = rows.Scan(ptrs...)
			parts := make([]string, len(vals))
			for i, v := range vals {
				parts[i] = formatScanned(v)
			}
			fmt.Println(strings.Join(parts, "\t"))
		}
	}

	section("totals")
	printQuery(`SELECT
        (SELECT COUNT(*) FROM atoms) AS atoms,
        (SELECT COUNT(*) FROM atom_related) AS related_edges,
        (SELECT COUNT(*) FROM atom_lineage) AS lineage_entries,
        (SELECT COUNT(*) FROM atom_prose_lex_refs) AS prose_lex_refs,
        (SELECT COUNT(*) FROM atom_prose_lex_refs WHERE is_stale_name=1) AS stale_citations`)

	section("atoms by tier")
	printQuery(`SELECT tier, COUNT(*) AS n FROM atoms GROUP BY tier ORDER BY n DESC`)

	section("atoms by status")
	printQuery(`SELECT status, COUNT(*) AS n FROM atoms GROUP BY status ORDER BY n DESC`)

	section("lineage source distribution")
	printQuery(`SELECT source, COUNT(*) AS n FROM atom_lineage WHERE source IS NOT NULL GROUP BY source ORDER BY n DESC LIMIT 15`)

	section("top 15 in-degree atoms (most-cited via related)")
	printQuery(`SELECT a.id, a.name, COUNT(*) AS n
        FROM atom_related r JOIN atoms a ON a.id = r.target_id
        GROUP BY a.id, a.name ORDER BY n DESC LIMIT 15`)

	section("orphans (no in-edges from related)")
	printQuery(`SELECT COUNT(*) AS orphan_count FROM atoms a
        WHERE NOT EXISTS (SELECT 1 FROM atom_related WHERE target_id = a.id)`)

	section("atoms with no canonical-instances")
	printQuery(`SELECT COUNT(*) AS no_canonical_instances FROM atoms a
        WHERE NOT EXISTS (SELECT 1 FROM atom_canonical_instances WHERE atom_id = a.id)`)

	section("atoms with no critical-questions")
	printQuery(`SELECT COUNT(*) AS no_critical_questions FROM atoms a
        WHERE NOT EXISTS (SELECT 1 FROM atom_critical_questions WHERE atom_id = a.id)`)

	section("atoms with practitioner-only lineage")
	printQuery(`SELECT COUNT(*) AS practitioner_only FROM atoms a
        WHERE EXISTS (SELECT 1 FROM atom_lineage WHERE atom_id = a.id)
        AND NOT EXISTS (SELECT 1 FROM atom_lineage WHERE atom_id = a.id AND source != 'practitioner')`)

	section("top 10 stale-citation targets")
	printQuery(`SELECT a.id, a.name, COUNT(*) AS stale_refs
        FROM atom_prose_lex_refs r JOIN atoms a ON a.id = r.target_id
        WHERE r.is_stale_name = 1
        GROUP BY a.id, a.name ORDER BY stale_refs DESC LIMIT 10`)
}

func dbSchema(renderDir string, args []string) {
	fl := flag.NewFlagSet("db schema", flag.ExitOnError)
	dbPath := fl.String("db", "", "SQLite path (default render/../lexicon.db)")
	_ = fl.Parse(args)
	path := *dbPath
	if path == "" {
		path = filepath.Join(renderDir, "..", "lexicon.db")
	}
	d, err := db.Open(path)
	if err != nil {
		fatal("open db: %v", err)
	}
	defer func() { _ = d.Close() }()

	rows, err := d.Query(`SELECT sql FROM sqlite_master WHERE type IN ('table','index') AND sql IS NOT NULL ORDER BY type, name`)
	if err != nil {
		fatal("query schema: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sqlStr sql.NullString
		_ = rows.Scan(&sqlStr)
		fmt.Println(sqlStr.String + ";")
	}
}

// dbLintGate is one SQL-backed elements-health check.
type dbLintGate struct {
	Code        string // short stable identifier, e.g. "no-critical-questions"
	Description string // one-line explanation
	Severity    string // "warning" | "info"
	Query       string // SELECT atom_id [, name, extra...]
}

var dbLintGates = []dbLintGate{
	{
		Code:        "no-agent-instruction",
		Description: "atoms lacking agent-instruction — operational 'when-you-see-this-do-this' missing",
		Severity:    "info",
		Query: `SELECT a.id, a.name FROM atoms a
            WHERE a.agent_instruction IS NULL OR a.agent_instruction = ''
            ORDER BY a.id`,
	},
	{
		Code:        "no-critical-questions",
		Description: "atoms lacking critical-questions — operational discriminators missing",
		Severity:    "warning",
		Query: `SELECT a.id, a.name FROM atoms a
            WHERE NOT EXISTS (SELECT 1 FROM atom_critical_questions WHERE atom_id = a.id)
            ORDER BY a.id`,
	},
	{
		Code:        "no-canonical-instances",
		Description: "atoms lacking canonical-instances — no operational examples",
		Severity:    "warning",
		Query: `SELECT a.id, a.name FROM atoms a
            WHERE NOT EXISTS (SELECT 1 FROM atom_canonical_instances WHERE atom_id = a.id)
            ORDER BY a.id`,
	},
	{
		Code:        "practitioner-only-lineage",
		Description: "atoms with only practitioner-tier lineage (no primary or refs-grounded source)",
		Severity:    "info",
		Query: `SELECT a.id, a.name FROM atoms a
            WHERE EXISTS (SELECT 1 FROM atom_lineage WHERE atom_id = a.id)
              AND NOT EXISTS (SELECT 1 FROM atom_lineage WHERE atom_id = a.id AND source != 'practitioner')
            ORDER BY a.id`,
	},
	{
		Code:        "single-source-lineage",
		Description: "atoms with only one lineage entry — no cross-attestation",
		Severity:    "info",
		Query: `SELECT a.id, a.name FROM atoms a
            JOIN atom_lineage l ON l.atom_id = a.id
            GROUP BY a.id, a.name
            HAVING COUNT(*) = 1
            ORDER BY a.id`,
	},
	{
		Code:        "under-review-status",
		Description: "atoms still marked status=under-review — pending promotion to active",
		Severity:    "info",
		Query: `SELECT a.id, a.name FROM atoms a
            WHERE a.status = 'under-review'
            ORDER BY a.id`,
	},
	{
		Code:        "dead-related-ref",
		Description: "related[] entries pointing to a lex-id absent from elements",
		Severity:    "warning",
		Query: `SELECT r.atom_id, r.target_id FROM atom_related r
            LEFT JOIN atoms a ON a.id = r.target_id
            WHERE a.id IS NULL
            ORDER BY r.atom_id, r.target_id`,
	},
	{
		Code:        "missing-reciprocation",
		Description: "atom A names B in related[], but B does not name A back",
		Severity:    "warning",
		Query: `SELECT r1.atom_id AS from_id, r1.target_id AS to_id
            FROM atom_related r1
            WHERE r1.atom_id != r1.target_id
              AND EXISTS (SELECT 1 FROM atoms WHERE id = r1.target_id)
              AND NOT EXISTS (
                SELECT 1 FROM atom_related r2
                WHERE r2.atom_id = r1.target_id AND r2.target_id = r1.atom_id)
            ORDER BY r1.atom_id, r1.target_id`,
	},
	{
		Code:        "high-indegree-low-cqs",
		Description: "highly-cited atoms (>=20 in-edges) with fewer than 3 critical-questions",
		Severity:    "info",
		Query: `SELECT a.id, a.name,
                (SELECT COUNT(*) FROM atom_related WHERE target_id = a.id) AS in_deg,
                (SELECT COUNT(*) FROM atom_critical_questions WHERE atom_id = a.id) AS cqs
            FROM atoms a
            WHERE in_deg >= 20 AND cqs < 3
            ORDER BY in_deg DESC`,
	},
	{
		Code:        "stale-cross-citation",
		Description: "prose mentions lex-NNNN with a kebab that doesn't match the atom's current name",
		Severity:    "warning",
		Query: `SELECT r.atom_id, r.target_id, r.kebab_follow, a.name AS current_name
            FROM atom_prose_lex_refs r
            JOIN atoms a ON a.id = r.target_id
            WHERE r.is_stale_name = 1
            ORDER BY r.target_id, r.atom_id`,
	},
}

func dbLint(renderDir string, args []string) {
	fl := flag.NewFlagSet("db lint", flag.ExitOnError)
	dbPath := fl.String("db", "", "SQLite path (default render/../lexicon.db)")
	asJSON := fl.Bool("json", false, "emit JSONL diagnostics, one per line")
	verbose := fl.Bool("verbose", false, "list all findings per gate (default lists 5 + count)")
	only := fl.String("only", "", "comma-separated list of gate codes to run (default all)")
	_ = fl.Parse(args)

	path := *dbPath
	if path == "" {
		path = filepath.Join(renderDir, "..", "lexicon.db")
	}
	d, err := db.Open(path)
	if err != nil {
		fatal("open db: %v", err)
	}
	defer func() { _ = d.Close() }()

	want := map[string]bool{}
	if *only != "" {
		for _, c := range strings.Split(*only, ",") {
			want[strings.TrimSpace(c)] = true
		}
	}

	totalFindings := 0
	for _, g := range dbLintGates {
		if len(want) > 0 && !want[g.Code] {
			continue
		}
		rows, err := d.Query(g.Query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: query failed: %v\n", g.Code, err)
			continue
		}
		cols, _ := rows.Columns()
		var findings [][]string
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rows.Next() {
			_ = rows.Scan(ptrs...)
			row := make([]string, len(vals))
			for i, v := range vals {
				row[i] = formatScanned(v)
			}
			findings = append(findings, row)
		}
		_ = rows.Close()
		totalFindings += len(findings)

		if *asJSON {
			for _, row := range findings {
				rec := map[string]string{
					"code":     g.Code,
					"severity": g.Severity,
				}
				for i, col := range cols {
					if i < len(row) {
						rec[col] = row[i]
					}
				}
				emitJSON(rec)
			}
			continue
		}

		fmt.Printf("\n[%s] %s  (%s)\n", g.Severity, g.Code, g.Description)
		fmt.Printf("  → %d finding(s)\n", len(findings))
		limit := 5
		if *verbose || len(findings) <= limit {
			limit = len(findings)
		}
		for _, row := range findings[:limit] {
			fmt.Printf("    %s\n", strings.Join(row, "  "))
		}
		if len(findings) > limit {
			fmt.Printf("    ... %d more (run with --verbose to list all)\n", len(findings)-limit)
		}
	}
	if !*asJSON {
		fmt.Fprintf(os.Stderr, "\ntotal findings across %d gate(s): %d\n", len(dbLintGates), totalFindings)
	}
}

func emitJSON(rec map[string]string) {
	parts := make([]string, 0, len(rec))
	keys := make([]string, 0, len(rec))
	for k := range rec {
		keys = append(keys, k)
	}
	// stable key order
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%q:%q", k, rec[k]))
	}
	fmt.Println("{" + strings.Join(parts, ",") + "}")
}

// dbFixStaleCitations rewrites `lex-NNNN <stale-kebab>` → `lex-NNNN` in
// every elements YAML and every source mining-pass MD. The conservative
// strategy avoids the risk of substituting the wrong new name (some
// "stale" cases are merge artifacts where the original atom never had
// the kebab as its name — they came from a different atom that was
// merged in). Dropping the kebab loses inline-readability but preserves
// elements correctness in every case.
//
// Default: dry-run; emits a per-file diff count. --apply commits edits
// across elements/ AND docs/passes/ (drift-check still binds them).
func dbFixStaleCitations(renderDir string, args []string) {
	fl := flag.NewFlagSet("db fix-stale-citations", flag.ExitOnError)
	apply := fl.Bool("apply", false, "actually write edits (default dry-run)")
	target := fl.String("target", "", "filter to one target lex-id (default: all)")
	dbPath := fl.String("db", "", "SQLite path (default render/../lexicon.db)")
	_ = fl.Parse(args)

	path := *dbPath
	if path == "" {
		path = filepath.Join(renderDir, "..", "lexicon.db")
	}
	d, err := db.Open(path)
	if err != nil {
		fatal("open db: %v", err)
	}
	defer func() { _ = d.Close() }()

	q := `SELECT atom_id, target_id, kebab_follow FROM atom_prose_lex_refs WHERE is_stale_name = 1`
	queryArgs := []interface{}{}
	if *target != "" {
		q += ` AND target_id = ?`
		queryArgs = append(queryArgs, *target)
	}
	rows, err := d.Query(q, queryArgs...)
	if err != nil {
		fatal("query stale: %v", err)
	}
	defer func() { _ = rows.Close() }()

	type staleEdit struct {
		atomID   string
		targetID string
		kebab    string
	}
	var edits []staleEdit
	for rows.Next() {
		var e staleEdit
		_ = rows.Scan(&e.atomID, &e.targetID, &e.kebab)
		edits = append(edits, e)
	}

	// Group by atom_id; we'll do one pass per file.
	byAtom := map[string][]staleEdit{}
	for _, e := range edits {
		byAtom[e.atomID] = append(byAtom[e.atomID], e)
	}

	elementsDir := filepath.Join(renderDir, "..", "elements")
	docsDir := filepath.Join(renderDir, "..", "docs")

	totalReplacements := 0
	filesTouched := 0
	for atomID, atomEdits := range byAtom {
		yamlPath := filepath.Join(elementsDir, atomID+".yaml")
		yamlBytes, err := os.ReadFile(yamlPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: read yaml: %v\n", atomID, err)
			continue
		}
		oldYAML := string(yamlBytes)
		newYAML := oldYAML
		fileCount := 0
		for _, e := range atomEdits {
			pat := e.targetID + " " + e.kebab
			repl := e.targetID
			before := strings.Count(newYAML, pat)
			newYAML = strings.ReplaceAll(newYAML, pat, repl)
			fileCount += before
		}
		if newYAML != oldYAML {
			totalReplacements += fileCount
			filesTouched++
			fmt.Printf("%s: %d replacement(s) in YAML\n", atomID, fileCount)
			if *apply {
				if err := os.WriteFile(yamlPath, []byte(newYAML), 0o644); err != nil {
					fmt.Fprintf(os.Stderr, "write yaml %s: %v\n", yamlPath, err)
				}
			}
		}
	}

	// Pass 2: scan every docs/passes/*.md for the same patterns. Multiple
	// atoms may live in one MD, so we walk the file list and apply all
	// applicable edits per file.
	mdFiles, _ := filepath.Glob(filepath.Join(docsDir, "passes", "*.md"))
	mdFiles2, _ := filepath.Glob(filepath.Join(docsDir, "principles", "*.md"))
	mdFiles = append(mdFiles, mdFiles2...)
	for _, mdPath := range mdFiles {
		body, err := os.ReadFile(mdPath)
		if err != nil {
			continue
		}
		oldMD := string(body)
		newMD := oldMD
		fileCount := 0
		for _, e := range edits {
			pat := e.targetID + " " + e.kebab
			repl := e.targetID
			before := strings.Count(newMD, pat)
			newMD = strings.ReplaceAll(newMD, pat, repl)
			fileCount += before
		}
		if newMD != oldMD {
			totalReplacements += fileCount
			filesTouched++
			fmt.Printf("%s: %d replacement(s) in MD\n", filepath.Base(mdPath), fileCount)
			if *apply {
				if err := os.WriteFile(mdPath, []byte(newMD), 0o644); err != nil {
					fmt.Fprintf(os.Stderr, "write md %s: %v\n", mdPath, err)
				}
			}
		}
	}

	mode := "DRY-RUN"
	if *apply {
		mode = "APPLIED"
	}
	fmt.Fprintf(os.Stderr, "\n%s: %d replacement(s) across %d file(s)\n", mode, totalReplacements, filesTouched)
	if !*apply {
		fmt.Fprintln(os.Stderr, "Re-run with --apply to write the edits.")
	}
}

func dbStaleCitations(renderDir string, args []string) {
	fl := flag.NewFlagSet("db stale-citations", flag.ExitOnError)
	target := fl.String("target", "", "filter to one target lex-id")
	dbPath := fl.String("db", "", "SQLite path (default render/../lexicon.db)")
	_ = fl.Parse(args)

	path := *dbPath
	if path == "" {
		path = filepath.Join(renderDir, "..", "lexicon.db")
	}

	d, err := db.Open(path)
	if err != nil {
		fatal("open db: %v", err)
	}
	defer func() { _ = d.Close() }()

	q := `SELECT r.atom_id, r.field, r.target_id, r.kebab_follow, a.name
        FROM atom_prose_lex_refs r
        JOIN atoms a ON a.id = r.target_id
        WHERE r.is_stale_name = 1`
	args2 := []interface{}{}
	if *target != "" {
		q += ` AND r.target_id = ?`
		args2 = append(args2, *target)
	}
	q += ` ORDER BY r.target_id, r.atom_id`

	rows, err := d.Query(q, args2...)
	if err != nil {
		fatal("query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	n := 0
	for rows.Next() {
		var atomID, field, targetID, kebab, name sql.NullString
		if err := rows.Scan(&atomID, &field, &targetID, &kebab, &name); err != nil {
			fatal("scan: %v", err)
		}
		fmt.Printf("%s\t%-20s\t%s\tkebab=%-40s current=%s\n",
			atomID.String, field.String, targetID.String, kebab.String, name.String)
		n++
	}
	fmt.Fprintf(os.Stderr, "\n%d stale cross-citation(s)\n", n)
}
