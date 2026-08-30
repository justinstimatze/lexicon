// sync.go — populate the SQLite index from YAMLs. One-shot rebuild on
// every `lexicon db build`; the file is removed first so the schema can
// evolve without migration logic during this phase. YAMLs remain the
// canonical source of truth; the DB is a derived view.

package db

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Build rebuilds the SQLite index at path from the in-memory elements.
// Removes any existing DB file first so schema changes during this phase
// don't require a migration step.
func Build(path string, sub map[string]*types.LexEntry) error {
	_ = os.Remove(path)
	d, err := Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertAtoms(tx, sub); err != nil {
		return err
	}
	if err := insertProseLexRefs(tx, sub); err != nil {
		return err
	}
	return tx.Commit()
}

func insertAtoms(tx *sql.Tx, sub map[string]*types.LexEntry) error {
	insAtom, err := tx.Prepare(`INSERT INTO atoms
        (id, name, common_name, type_in, type_out, tier, status, assembly, mechanism, formal_if_any, severity_tier, reversibility, agent_instruction, encounter_tier_override)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare atoms: %w", err)
	}
	defer func() { _ = insAtom.Close() }()

	stmts := map[string]*sql.Stmt{}
	for _, t := range []string{"atom_related", "atom_premises", "atom_decomposes_into", "atom_scaffolds_from"} {
		s, err := tx.Prepare(fmt.Sprintf(`INSERT INTO %s (atom_id, target_id, seq) VALUES (?, ?, ?)`, t))
		if err != nil {
			return fmt.Errorf("prepare %s: %w", t, err)
		}
		defer func() { _ = s.Close() }()
		stmts[t] = s
	}
	insEvokes, err := tx.Prepare(`INSERT INTO atom_evokes (atom_id, evoke, seq) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare atom_evokes: %w", err)
	}
	defer func() { _ = insEvokes.Close() }()

	insLineage, err := tx.Prepare(`INSERT INTO atom_lineage (atom_id, seq, source, tradition, text, citation, quote) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare atom_lineage: %w", err)
	}
	defer func() { _ = insLineage.Close() }()

	insCanon, err := tx.Prepare(`INSERT INTO atom_canonical_instances (atom_id, seq, instance) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare atom_canonical_instances: %w", err)
	}
	defer func() { _ = insCanon.Close() }()

	insCQ, err := tx.Prepare(`INSERT INTO atom_critical_questions (atom_id, seq, question) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare atom_critical_questions: %w", err)
	}
	defer func() { _ = insCQ.Close() }()

	reactStmts := map[string]*sql.Stmt{}
	for _, t := range []string{"atom_reactants", "atom_products", "atom_catalysts", "atom_inhibitors", "atom_conditions"} {
		s, err := tx.Prepare(fmt.Sprintf(`INSERT INTO %s (atom_id, seq, value) VALUES (?, ?, ?)`, t))
		if err != nil {
			return fmt.Errorf("prepare %s: %w", t, err)
		}
		defer func() { _ = s.Close() }()
		reactStmts[t] = s
	}

	ids := make([]string, 0, len(sub))
	for id := range sub {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		e := sub[id]
		if _, err := insAtom.Exec(
			e.ID, e.Name, nullStr(e.CommonName), e.TypeIn, e.TypeOut, e.Tier, e.Status,
			nullStr(e.Assembly), nullStr(e.Mechanism), nullStr(e.FormalIfAny),
			nullStr(e.SeverityTier), nullStr(e.Reversibility),
			nullStr(e.AgentInstruction), nullInt(e.EncounterTierOverride),
		); err != nil {
			return fmt.Errorf("insert atom %s: %w", e.ID, err)
		}

		for i, v := range e.Related {
			if _, err := stmts["atom_related"].Exec(e.ID, v, i); err != nil {
				return fmt.Errorf("insert related %s seq %d: %w", e.ID, i, err)
			}
		}
		for i, v := range e.ScaffoldsFrom {
			if _, err := stmts["atom_scaffolds_from"].Exec(e.ID, v, i); err != nil {
				return fmt.Errorf("insert scaffolds_from %s seq %d: %w", e.ID, i, err)
			}
		}
		for i, v := range e.Evokes {
			if _, err := insEvokes.Exec(e.ID, v, i); err != nil {
				return fmt.Errorf("insert evokes %s seq %d: %w", e.ID, i, err)
			}
		}
		for i, v := range e.Premises {
			if _, err := stmts["atom_premises"].Exec(e.ID, v, i); err != nil {
				return fmt.Errorf("insert premises %s seq %d: %w", e.ID, i, err)
			}
		}
		for i, v := range e.DecomposesInto {
			if _, err := stmts["atom_decomposes_into"].Exec(e.ID, v, i); err != nil {
				return fmt.Errorf("insert decomposes_into %s seq %d: %w", e.ID, i, err)
			}
		}
		for i, l := range e.Lineage {
			if _, err := insLineage.Exec(e.ID, i, l.Source, nullStr(l.Tradition), l.Text, l.Citation, nullStr(l.Quote)); err != nil {
				return fmt.Errorf("insert lineage %s seq %d: %w", e.ID, i, err)
			}
		}
		for i, v := range e.CanonicalInstances {
			if _, err := insCanon.Exec(e.ID, i, v); err != nil {
				return fmt.Errorf("insert canonical_instances %s seq %d: %w", e.ID, i, err)
			}
		}
		for i, v := range e.CriticalQuestions {
			if _, err := insCQ.Exec(e.ID, i, v); err != nil {
				return fmt.Errorf("insert critical_questions %s seq %d: %w", e.ID, i, err)
			}
		}
		for _, tbl := range []struct {
			name string
			data []string
		}{
			{"atom_reactants", e.Reactants},
			{"atom_products", e.Products},
			{"atom_catalysts", e.Catalysts},
			{"atom_inhibitors", e.Inhibitors},
			{"atom_conditions", e.Conditions},
		} {
			for i, v := range tbl.data {
				if _, err := reactStmts[tbl.name].Exec(e.ID, i, v); err != nil {
					return fmt.Errorf("insert %s %s seq %d: %w", tbl.name, e.ID, i, err)
				}
			}
		}
	}
	return nil
}

// Same regexes as cmd_lint's cross-citation checks. Kept private here so
// the DB build is self-contained; if both stay in sync the DB query can
// fully replace the in-memory lint loop.
// \b-anchored (migration 2026-08-20): the new alphabet spans most of a-z,
// so an unanchored `lex-` prefix match false-positives inside ordinary
// compound words — "complex-systems" contains the literal substring
// "lex-syste". The old 4-digit alphabet never had this problem; no English
// word puts 4 digits right after "lex". \b before "lex" rejects a match
// starting mid-word (no boundary between "p" and "l" in "complex-...").
var proseLexRefRe = regexp.MustCompile(`\blex-[23456789abcdefghjkmnpqrstuvwxyz]{5}\b\s+([a-z][a-z0-9]+(?:-[a-z0-9]+){2,})`)

// bareLexIDRe matches any lex-xxxxx mention independent of whether a
// name-claim follows — mirrors cmd_lint's danglingCitationCheck. Used to
// catch dangling refs proseLexRefRe's kebab-name requirement would miss
// (bare-ID lists, single-hyphen names, function-call-style references).
var bareLexIDRe = regexp.MustCompile(`\blex-[23456789abcdefghjkmnpqrstuvwxyz]{5}\b`)

// insertProseLexRefs scans every atom's prose fields and populates the
// atom_prose_lex_refs flat table. Each lex-NNNN mention in prose becomes
// a row tagged with the field of origin and whether it's dangling
// (target doesn't exist) or, for resolved targets with a name-claim
// following, whether the kebab matches the referenced atom's current
// name.
func insertProseLexRefs(tx *sql.Tx, sub map[string]*types.LexEntry) error {
	ins, err := tx.Prepare(`INSERT INTO atom_prose_lex_refs
        (atom_id, field, field_seq, target_id, kebab_follow, is_stale_name, is_dangling)
        VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare atom_prose_lex_refs: %w", err)
	}
	defer func() { _ = ins.Close() }()

	ids := make([]string, 0, len(sub))
	for id := range sub {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		e := sub[id]
		if err := scanField(ins, e, "assembly", -1, e.Assembly, sub); err != nil {
			return err
		}
		if err := scanField(ins, e, "mechanism", -1, e.Mechanism, sub); err != nil {
			return err
		}
		for i, l := range e.Lineage {
			if err := scanField(ins, e, "lineage_citation", i, l.Citation, sub); err != nil {
				return err
			}
			if err := scanField(ins, e, "lineage_quote", i, l.Quote, sub); err != nil {
				return err
			}
		}
		for i, v := range e.CanonicalInstances {
			if err := scanField(ins, e, "canonical_instance", i, v, sub); err != nil {
				return err
			}
		}
		for i, v := range e.CriticalQuestions {
			if err := scanField(ins, e, "critical_question", i, v, sub); err != nil {
				return err
			}
		}
	}
	return nil
}

func scanField(ins *sql.Stmt, e *types.LexEntry, field string, seq int, text string, sub map[string]*types.LexEntry) error {
	if text == "" {
		return nil
	}
	var seqArg sql.NullInt64
	if seq >= 0 {
		seqArg = sql.NullInt64{Valid: true, Int64: int64(seq)}
	}

	danglingSeen := map[string]bool{}
	for _, target := range bareLexIDRe.FindAllString(text, -1) {
		if _, ok := sub[target]; ok {
			continue
		}
		if danglingSeen[target] {
			continue
		}
		danglingSeen[target] = true
		if _, err := ins.Exec(e.ID, field, seqArg, target, "", 0, 1); err != nil {
			return fmt.Errorf("insert dangling prose_lex_ref for %s: %w", e.ID, err)
		}
	}

	for _, m := range proseLexRefRe.FindAllStringSubmatchIndex(text, -1) {
		target := text[m[0] : m[0]+8]
		kebab := text[m[2]:m[3]]
		t, ok := sub[target]
		if !ok {
			continue // recorded by the dangling sweep above
		}
		if kebabStartsWithStopWord(kebab) {
			continue // descriptive context, not a name claim
		}
		stale := 0
		if !kebabMatchesName(kebab, t.Name) && !kebabMatchesEvokes(kebab, t.Evokes) {
			stale = 1
		}
		if _, err := ins.Exec(e.ID, field, seqArg, target, kebab, stale, 0); err != nil {
			return fmt.Errorf("insert prose_lex_ref for %s: %w", e.ID, err)
		}
	}
	return nil
}

// kebabStartsWithStopWord mirrors the in-memory linter's stop-word filter.
// Excludes kebabs that lead with a preposition/article/conjunction —
// signals descriptive context, not a name claim.
func kebabStartsWithStopWord(kebab string) bool {
	first := kebab
	if i := strings.IndexByte(kebab, '-'); i >= 0 {
		first = kebab[:i]
	}
	switch first {
	case "across", "within", "between", "under", "over", "above", "below",
		"as", "the", "a", "an", "of", "in", "on", "at", "by", "for",
		"with", "without", "into", "onto", "via", "and", "or", "but",
		"so", "if", "when", "while", "during", "after", "before",
		"co", "vs", "is", "are", "was", "were", "be", "to":
		return true
	}
	return false
}

// kebabMatchesName mirrors the in-memory linter's name check. Kept in
// this package so the DB build is self-contained; eventual consolidation
// moves both callers onto one definition.
func kebabMatchesName(kebab, name string) bool {
	k := strings.ToLower(kebab)
	n := strings.ToLower(name)
	if strings.Contains(n, k) {
		return true
	}
	kSeg := strings.Split(k, "-")
	nSeg := strings.Split(n, "-")
	prefix := 3
	if len(kSeg) < prefix {
		prefix = len(kSeg)
	}
	if len(nSeg) < prefix {
		return false
	}
	for i := 0; i < prefix; i++ {
		if kSeg[i] != nSeg[i] {
			return false
		}
	}
	return true
}

// kebabMatchesEvokes mirrors the in-memory linter's evokes check. A kebab
// that matches one of the target atom's own declared evokes-aliases is a
// legitimate term-of-art citation ("lex-2kjmb free-energy-principle"), not a
// stale name-drift -- kebabMatchesName alone doesn't know about evokes and
// over-flagged 19 legitimate citations as stale before this was added.
func kebabMatchesEvokes(kebab string, evokes []string) bool {
	k := strings.ToLower(kebab)
	for _, e := range evokes {
		if strings.ToLower(e) == k {
			return true
		}
	}
	return false
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: s}
}

// nullInt treats 0 as absent -- EncounterTierOverride is documented in
// types.LexEntry as "integer 1-5; absent for the common case where derived
// is correct," so the Go zero value and "no override" are the same state.
func nullInt(n int) sql.NullInt64 {
	if n == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Valid: true, Int64: int64(n)}
}
