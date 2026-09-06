# Using lexicon.elements as an LLM's context

This is a library first, a CLI second. The point isn't to browse the
catalog — it's to give an LLM agent, mid-conversation, a set of named
patterns to check its own reasoning against. This page is the plain
version of that; the README says the same thing in a register that
doesn't fit here.

## Two ways to call it

**MCP server** — `lexicon mcp` runs a stdio JSON-RPC server exposing a
curated set of tools an agent calls directly, no shelling out. This is
the primary interface.

`lexicon mcp` handles one request at a time over its single stdio
pipe — there's no request-ID multiplexing. A consumer with concurrent
callers (multiple agents, a fan-out) needs a lock around one shared
subprocess rather than issuing calls in parallel, or it risks
corrupting the newline-delimited-JSON framing. Relatedly, if you're
writing your own NDJSON reader against this pipe rather than an
existing MCP client library, check your runtime's stream reader for a
default max-line-length — some stdlib line readers (not unique to any
one language) truncate or error on a longer-than-expected single line.
`lexicon_read`/`lexicon_predict`/`lexicon_distinctness` each shell out
to a fresh `lexicon` subprocess per call (deliberate crash isolation
for the network-calling tools, not an oversight) — so each of those
calls repeats a full corpus load, even within one long-lived MCP
session. High-volume callers (dozens-to-hundreds of calls per session)
should budget for that per-call cost rather than assume the corpus is
cached across calls.

**CLI** — every MCP tool has a CLI equivalent for scripting or manual
use (`lexicon read`, `lexicon extrapolate`, ...). Build with
`go build -o lexicon ./cmd/lexicon` from `render/`.

## The core tools

Four operations do the actual work. Two never call a model — pure
graph or lexical operations over the elements corpus, deterministic,
free:

- **`lexicon_extrapolate`** (`lexicon extrapolate <id> [<id> ...]`) —
  given a constellation of atom IDs, returns the atoms *not* in the
  set that the constellation points at, ranked by how many of them
  point at it. The ontological negative space of whatever frame the
  constellation names.
- **`lexicon_constellation`** (`lexicon constellation <id>`) — the N-hop
  neighborhood of one atom: what it relates to, decomposes into, and
  is pointed at by, each neighbor carrying its own gloss and
  agent-instruction so a caller can compose without a follow-up call.

Two use an LLM-backed semantic lens by default (skip it with
`no_lens`/`--no-lens` for a faster, less precise lexical-only pass):

- **`lexicon_read`** (`lexicon read [file|-]`) — surface the top-K
  atoms that fire on a passage of text: an article, a transcript
  chunk, your own writing. Each result carries `agent_instruction`
  (the imperative rule) and `critical_questions` (what would confirm
  or reject the pattern actually applying).
- **`lexicon_predict`** (`lexicon what-if --mode intervene --context -`,
  a different CLI name than the MCP tool — the mapping isn't obvious
  from either name alone) — forecast downstream
  effects of a plan or situation via the reaction-tier atoms: what's
  likely, what accelerates it, what blocks it, under what conditions.

`lexicon_list` (`lexicon list`) is plain enumeration — id, name,
tier, status — useful for orientation before a targeted call, not a
reasoning primitive itself.

## The Claude Code hook

`lexicon hook` is the same `read` mechanism wired as a
`UserPromptSubmit` hook, so it runs automatically on every prompt in
a session rather than waiting to be called. Add to
`~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {"hooks": [{"type": "command", "command": "lexicon hook"}]}
    ]
  }
}
```

It reads a JSON envelope from stdin and emits an `additionalContext`
injection when a matched atom clears the relevance threshold —
otherwise it's silent. Every error path swallows quietly (logged to
`~/.claude/lexicon/hook.log`) so a broken lexicon install never blocks
a turn. Tuning knobs (threshold, top-k, lens on/off) are environment
variables documented at the top of `render/cmd/lexicon/cmd_hook.go` —
read that file directly rather than this page for the current list;
it changes faster than this doc would stay accurate.

## Higher-level tools

`lexicon council` and `lexicon what-if` compose the same core scoring
engine into richer output shapes (multiple independent "voices" on a
stuck decision; disambiguation probes for an ambiguous situation).
They're CLI-only right now, not exposed over MCP, and not necessarily
permanent fixtures of this repo — one live possibility is that a tool
like `council` ends up spun out as its own project consuming lexicon
as a library dependency, the way a specialized tool would consume any
other library, rather than living inside lexicon's own CLI forever.
Treat them as worked examples of what the core four can be composed
into, not as a stable contract the way `read`/`extrapolate`/`predict`/
`constellation` are.

## Data

The YAML schema (what fields an atom carries, what they mean) is in
[SCHEMA.md](SCHEMA.md). `lexicon export-graph` emits the whole corpus
as one JSON document — nodes, edges, clusters — if you want to work
against the raw graph directly instead of through the tools above.

`lexicon anki [--out FILE] [--tier T] [--status S]` exports an
Anki-importable TSV deck for human spaced-repetition study — this one's
for a person working through the catalog, not an agent's mid-conversation
lookup. Two cards per atom rather than one: a recognition card (a
concrete scenario on the front, the pattern's name on the back) and a
recall card (the pattern's name and type signature on the front, its
agent-instruction on the back). Splitting this way follows the
minimum-information principle — one card testing one fact reviews
consistently, one card cramming a citation, an example, and a rule
together doesn't. A prebuilt copy is linked from the README.

`lexicon anki --lint` checks the deck it would export against SuperMemo's
Twenty Rules (via controlaltbackspace.org/precise) instead of writing it:
a recall back too long to be "one fact," a recognition front that had to
be truncated or that names another atom's id outright, enumeration-marker
smells, and unbalanced markdown-italic asterisks. Advisory only, always
exits 0 — see the design comment above `runAnkiLint` in `cmd_anki.go` for
what it deliberately doesn't check and why.
