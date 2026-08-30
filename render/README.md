# lexicon/render — v0 implementation (Go)

*Per `../render-function-design-v0.md`. Vertical slice — proves the
load → gate → render → CLI loop works for one molecule end-to-end.
Other modes deferred to subsequent passes.*

*Originally TS; rewritten to Go for sibling-ecosystem fit (the
elements-destination tooling is Go, and schema-accretion benefits
from struct-as-source-of-truth).*

## Layout

```
cmd/lexicon/                  CLI binary (one file per subcommand)
  main.go                       dispatch + .env loader
  cmd_render.go                 lexicon render <id> [--mode --context --why]
  cmd_gate.go                   lexicon gate [--context --vocab --top-k]
  cmd_log.go                    lexicon log <vibe> [--count --notes]
internal/types/               LexEntry, RenderMode, SessionTag, etc.
internal/loader/              YAML reader + min-required-field validation
internal/gate/                Deterministic filter+rank, context-stance classifier
internal/session/             Append-only session-log.jsonl writer
internal/db/                  SQLite structural index derived from elements/*.yaml
                              (lexicon db build/query/lint/stats); YAML stays canonical
internal/client/              Anthropic SDK wrapper + .env loader (small interface
                              for testability — narrative_test substitutes a fake)
internal/modes/               One file per render mode (algebraic, meta-
                              explanatory, visual, introspection, narrative)
../elements/                  Extracted YAML entries (lives at repo root, a
                              sibling of render/); canonical source of truth.
```

## Status: v0 vertical slice (Go rewrite landed)

Working modes:
- `algebraic` — raw YAML pretty-print
- `meta-explanatory` — what the entry is + type signature + relations
  + lineage + example + status (pure template, default mode)
- `narrative` — LLM call via anthropic-sdk-go (claude-sonnet-4-6);
  100-300 word second-person deployment-mode prose; weaves critical-
  questions as defeaters; doesn't surface the entry name
- `visual` — mermaid template generator (no LLM); flowchart TD for
  molecules with decomposes-into edges, flowchart LR for atoms with
  related-as-siblings
- `introspection` — first-class mode (not just `--why` flag); atoms-
  deployed, defeaters-as-checkboxes, lineage-verification status

Working subcommands:
- `lexicon render <id> [--mode --context --why]`
- `lexicon gate [--context --vocab --top-k]` (context-dependent
  tier scoring: deployment → molecule first, design → atoms first)
- `lexicon log <vibe> [--count --notes]` — vibes: useful | mixed |
  not-useful | autonomous (autonomous added for follow-on agent
  sessions; doesn't game human-driven gut-check totals)

Deferred to subsequent passes:
- `phenomenological` and `dialogical` modes (require entry-level
  fields not yet present in elements/; per design doc, deferred
  past v0)
- `parable` mode as a first-class form (queued per user pointer +
  triple neurobiological warrant — see `../mattson-2014-spp-grounding.md`,
  `../clark-2013-pp-grounding.md`, lit review for details)

## Quick test

```bash
cd render  # from the repo root

# For narrative mode, drop your key into the gitignored .env:
#   cp .env.example .env  (already done; just edit .env)
#   echo "ANTHROPIC_API_KEY=sk-ant-..." > .env

go run ./cmd/lexicon render lex-kebfa
go run ./cmd/lexicon render lex-kebfa --mode algebraic
go run ./cmd/lexicon render lex-kebfa --mode visual
go run ./cmd/lexicon render lex-kebfa --mode introspection
go run ./cmd/lexicon render lex-kebfa --mode narrative --context "your situation here"
go run ./cmd/lexicon render lex-kebfa --why --context "design conversation"
go run ./cmd/lexicon gate --context "user mid-bind on expert claims"
go run ./cmd/lexicon log autonomous --count 5 --notes "session note"
cat session-log.jsonl

# Or build once:
go build -o lexicon ./cmd/lexicon
./lexicon render lex-kebfa
```

The CLI auto-loads `render/.env` at startup if present. `.env` is
gitignored. Falls back to plain `ANTHROPIC_API_KEY` env var if no
.env file exists.

## Testing

```bash
go test ./...    # all packages
go vet ./...
```

7 test files; no real LLM calls in tests (narrative_test substitutes
a fake client.Client implementation).

## Claude Code hook (use-loop integration)

The CLI is human-invoked, which means the use-loop only closes when
the user remembers to invoke it mid-conversation. The
`UserPromptSubmit` hook closes the loop by running `lexicon gate`
against every Claude Code prompt and injecting the top matched
primitives into the conversation context as a `system-reminder` —
Claude can then choose whether to deploy them.

### Install

1. Build + install the binary on PATH:
   ```bash
   cd render  # from the repo root
   go install ./cmd/lexicon
   # → ~/go/bin/lexicon (must be on PATH)
   ```

2. Add to `~/.claude/settings.json`:
   ```json
   {
     "hooks": {
       "UserPromptSubmit": [
         {"hooks": [{"type": "command", "command": "lexicon hook"}]}
       ]
     },
     "env": {
       "LEXICON_ELEMENTS_DIR": "/path/to/lexicon/elements"
     }
   }
   ```

   The `LEXICON_ELEMENTS_DIR` env var is required because hooks
   run from arbitrary CWDs — without it the binary can't find
   elements/.

3. Optional skip:
   ```bash
   export LEXICON_SKIP=1   # short-circuits before any work
   ```

### Behavior

- Reads UserPromptSubmit JSON from stdin (`{prompt, cwd, session_id, ...}`).
- Extracts vocabulary tokens from the prompt (drops stop-words, length<3).
- Runs `gate.Run` with the prompt as context + extracted vocab.
- If top-k score ≥ 0.50, emits `additionalContext` JSON listing the
  top 3 primitives with their type signature, tier, score, and
  first canonical instance.
- Defensive on every error path: panics, decode failures, a missing
  elements/ dir all log silently to `~/.claude/lexicon/hook.log` and
  return without emitting (a broken lexicon never blocks a Claude
  turn).

### Test the hook

```bash
echo '{"prompt":"user mid-bind on expert claims about credibility","cwd":"/tmp","session_id":"test","hook_event_name":"UserPromptSubmit"}' \
  | LEXICON_ELEMENTS_DIR=$PWD/../elements go run ./cmd/lexicon hook
```

Should emit JSON with `lex-kebfa argument-from-expert-opinion` as
the top match (score ≈ 1.12 — molecule deployment-base 0.7 ×
status-mult 0.7 × name-token-match 1.6 = 0.784, but token "expert"
matches name AND vocab-boost compounds: 0.49 × 1.6 = 0.784, capped
because molecule-base wins).

## Using lexicon from another Go project

`pkg/lexicon` is a stable, directly-callable API for the same pattern-
match scoring path `lexicon read`/`what-if` runs — for a sibling project
that wants to call this in-process instead of shelling out to the built
binary and parsing its JSON back out.

```go
import lexicon "github.com/justinstimatze/lexicon/render/pkg/lexicon"

corp, err := lexicon.LoadCorpus(renderDir) // load once, reuse across calls
// ...
result, err := corp.Score(ctx, passageText, lexicon.ReadOptions{TopK: 3})
for _, p := range result.Patterns {
    // p.ID, p.Name, p.Score, p.AgentInstruction, p.Adjacencies, ...
}
```

`lexicon.Read(ctx, renderDir, text, opts)` is a one-shot convenience
wrapper (`LoadCorpus` + `Score`) for a caller scoring a single passage
who doesn't need the `Corpus` afterward. Pass `NoLens: true` for a fully
local, lexical-only score with no network dependency.

Until this module is tagged and published, a consumer on the same
machine points at it directly in its own `go.mod`:

```
require github.com/justinstimatze/lexicon/render v0.0.0
replace github.com/justinstimatze/lexicon/render => /path/to/lexicon/render
```

## Design doc

`../render-function-design-v0.md` — multi-mode operation family,
GATE block spec, resolved design questions Q1-Q5.

## Concerns / known debt

`./CONCERNS.md` — known debt, no CI yet.
