// Package db is the SQLite-backed structural index for the lexicon
// elements. YAMLs remain canonical; the index is derived via `lexicon
// db build` and dropped on rebuild. The point is making cross-citation
// queries, adjacency traversal, and bulk lints cheap — at the elements'
// size, with thick cross-linking, walking YAMLs every query stops scaling.
//
// Pattern lifted from sibling project cupel (github.com/justinstimatze/cupel;
// same modernc.org/sqlite, STRICT tables, transactional batch insert).
// Lexicon-specific differences:
// (a) atoms have ~10 multi-valued fields (related, evokes, lineage,
// canonical-instances, critical-questions, reaction tier) so each gets
// its own child table with seq ordinals; (b) no foreign keys on cross-
// references because targets may legitimately not exist yet (caught by
// lint).
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const SchemaSQL = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS atoms (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    common_name     TEXT,
    type_in         TEXT,
    type_out        TEXT,
    tier            TEXT,
    status          TEXT,
    assembly        TEXT,
    mechanism       TEXT,
    formal_if_any   TEXT,
    severity_tier   TEXT,
    reversibility   TEXT,
    agent_instruction TEXT,
    encounter_tier_override INTEGER
) STRICT;
CREATE INDEX IF NOT EXISTS atoms_tier ON atoms(tier);
CREATE INDEX IF NOT EXISTS atoms_status ON atoms(status);

-- related, evokes, premises, decomposes-into, scaffolds-from: edge tables.
-- No FK on target_id (or evoke) — elements convention is that referenced
-- atoms may not exist yet (lint flags). Self-references are tolerated.
CREATE TABLE IF NOT EXISTS atom_related (
    atom_id     TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL,
    seq         INTEGER NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE INDEX IF NOT EXISTS atom_related_target ON atom_related(target_id);

CREATE TABLE IF NOT EXISTS atom_scaffolds_from (
    atom_id     TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL,
    seq         INTEGER NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE INDEX IF NOT EXISTS atom_scaffolds_from_target ON atom_scaffolds_from(target_id);

CREATE TABLE IF NOT EXISTS atom_evokes (
    atom_id     TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    evoke       TEXT NOT NULL,
    seq         INTEGER NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE INDEX IF NOT EXISTS atom_evokes_evoke ON atom_evokes(evoke);

CREATE TABLE IF NOT EXISTS atom_premises (
    atom_id     TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL,
    seq         INTEGER NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE INDEX IF NOT EXISTS atom_premises_target ON atom_premises(target_id);

CREATE TABLE IF NOT EXISTS atom_decomposes_into (
    atom_id     TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL,
    seq         INTEGER NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE INDEX IF NOT EXISTS atom_decomposes_into_target ON atom_decomposes_into(target_id);

CREATE TABLE IF NOT EXISTS atom_lineage (
    atom_id     TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    source      TEXT,
    tradition   TEXT,
    text        TEXT,
    citation    TEXT,
    quote       TEXT,
    PRIMARY KEY (atom_id, seq)
) STRICT;

CREATE TABLE IF NOT EXISTS atom_canonical_instances (
    atom_id     TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    instance    TEXT NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;

CREATE TABLE IF NOT EXISTS atom_critical_questions (
    atom_id     TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    question    TEXT NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;

-- Reaction-tier fields. Each is multi-valued, so own table.
CREATE TABLE IF NOT EXISTS atom_reactants (
    atom_id TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE TABLE IF NOT EXISTS atom_products (
    atom_id TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE TABLE IF NOT EXISTS atom_catalysts (
    atom_id TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE TABLE IF NOT EXISTS atom_inhibitors (
    atom_id TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;
CREATE TABLE IF NOT EXISTS atom_conditions (
    atom_id TEXT NOT NULL REFERENCES atoms(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (atom_id, seq)
) STRICT;

-- Derived: a flat extraction of every lex-NNNN mention that appears in
-- atom prose. Populated by the sync step; queryable by lint and
-- downstream consumers. is_dangling = 1 when target_id doesn't resolve
-- to any atom at all (renamed-and-retired, merged away, or a planning
-- placeholder that was never minted) — kebab_follow is '' in that case,
-- since there's no live atom to have asserted a name against.
-- is_stale_name = 1 when target_id resolves but kebab_follow no longer
-- matches its current name. The two are mutually exclusive.
CREATE TABLE IF NOT EXISTS atom_prose_lex_refs (
    atom_id         TEXT NOT NULL,
    field           TEXT NOT NULL,
    field_seq       INTEGER,
    target_id       TEXT NOT NULL,
    kebab_follow    TEXT NOT NULL,
    is_stale_name   INTEGER NOT NULL,
    is_dangling     INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX IF NOT EXISTS atom_prose_lex_refs_target ON atom_prose_lex_refs(target_id);
CREATE INDEX IF NOT EXISTS atom_prose_lex_refs_stale ON atom_prose_lex_refs(is_stale_name);
CREATE INDEX IF NOT EXISTS atom_prose_lex_refs_dangling ON atom_prose_lex_refs(is_dangling);
`

// Open opens (or creates) the SQLite database at path with the lexicon
// schema applied. Caller closes.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := db.Exec(SchemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema in %s: %w", path, err)
	}
	return db, nil
}
