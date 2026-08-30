# render/ — known concerns + deferred debt

## Elements drift (resolved 2026-08-19)

**Original problem:** `elements/*.yaml` was mirrored into YAML blocks
embedded in `docs/passes/*.md` mining-pass markdowns, and `check-drift`
verified the two copies agreed. The mirror turned out to be gitignored and
per-machine — it never traveled through git, so a fresh clone or a second
machine could fail the check over an atom whose local mirror simply didn't
exist there yet (this happened in practice: `lex-345ta`/`lex-dun6c` were
minted on one machine and reported "orphaned no-source" on another after a
merge, because the minting machine's local MD mirror never left it).
check-drift's only real function was comparing two copies of the same
data; it carried no semantic rule `lint`/`lint-cross-refs` didn't already
provide independently.

**Resolution:** `docs/passes/*.md` is no longer treated as an authoritative
mirror or hand-synced against elements/ edits. `check-drift` and the
`internal/driftcheck` package are deleted. The pre-commit hook now runs
`lexicon db build` (rebuilds the SQLite index at `render/internal/db/`
from staged elements) followed by `lexicon db lint` (blocking on its
error tier, same pattern `lint` already used). `docs/passes/*.md` itself
is left in place, unmaintained — `cmd_coverage.go`/`cmd_backlog.go` still
read its filenames (never content) as one of two corpus signals for
"already mined," so deleting it would shrink that signal for no benefit.

`elements/*.yaml` remains the sole hand-edited source of truth. The
bigger migration this was scoped down from — making the SQLite index
canonical, generating YAML as an export, and building a CRUD-edit CLI to
replace hand-editing YAML — stays a named, larger, unstarted option (see
`../ROADMAP.md`, *Elements maintenance*), not required to unblock the
pivot-table/classification work queued behind it.

## Other concerns (logged for future passes)

- **No CI yet.** Personal-use single-machine doesn't need it; add
  when collaborators or open-source release happens.
- **Elements dir is hardcoded relative path.** `loader` uses
  `elements/` next to the binary (`internal/loader.DefaultElementsDir`).
  Works for a single elements/ set; needs config when there are
  multiple (e.g., personal + shared).
