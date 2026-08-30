// Command lexicon is the v0 render-function CLI. Subcommands:
//
//	render      Render one entry in a chosen mode.
//	shuffle     Pick N entries at random and render each.
//	gate        Run the deterministic filter+rank against the elements.
//	log         Append one per-session vibe entry to session-log.jsonl.
//	hook        Claude Code UserPromptSubmit hook (see cmd_hook.go).
//	mark-fire   Tag a specific hook fire (by hook_event_id from
//	            fires.jsonl) as useful / mixed / not-useful / autonomous.
//	shell       Emit the legacy composed matrix+pivot HTML view.
//	export-graph  Emit the elements graph as JSON — the web/ SPA's data
//	            contract, and lexicon.github.io's primary UI.
//
// Single binary; per-subcommand handlers in cmd_*.go.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinstimatze/lexicon/render/internal/client"
)

const help = `lexicon — v0 render function (CLI)

Usage:
  lexicon render <lex-id> [--mode <mode>] [--context <text>] [--why]
  lexicon shuffle [--n N] [--mode <mode>] [--filter key=value] [--seed N]
  lexicon gate [--context <text>] [--vocab <comma,sep,words>] [--top-k N] [--no-lens]
                                (V13: lens applies when --context is set;
                                 --no-lens forces pure lexical re-rank on
                                 the full pool, the pre-V13 behavior)
  lexicon log <vibe> [--count N] [--notes "..."]
  lexicon hook                  (Claude Code UserPromptSubmit hook;
                                 reads JSON from stdin, emits to stdout)
  lexicon mark-fire <event-id> <vibe> [--notes "..."]
                                (tag a per-fire vibe to fire-tags.jsonl;
                                 hook_event_id comes from fires.jsonl
                                 or the "event=..." in hook.log)
  lexicon shell [--out <path>]
                                (2026-08-20: legacy view, superseded as
                                 the landing page by the web/ SPA — kept
                                 for its matrix Canvas2D tab, which the
                                 SPA doesn't have yet. Emits matrix +
                                 pivot as tabs + persistent detail pane.
                                 Mobile-responsive. Default
                                 render/viz/index.html. Companion
                                 standalone pages: matrix, pivot.)
  lexicon matrix [--out <path>] | lexicon pivot [--out <path>]
                                (standalone single-view pages; both
                                 keep working as stable legacy URLs.
                                 pivot's own rendering is superseded by
                                 the SPA's Pivot tab; matrix's Canvas2D
                                 view has no SPA equivalent yet.)
  lexicon export-graph [--out <path>]
                                (2026-08-20: emit viz.Graph as raw JSON —
                                 nodes, edges, clusters, precomputed 3D
                                 layouts. Same data every HTML renderer
                                 inlines at build time; consumed by the
                                 web/ frontend's build step. Default
                                 stdout.)
  lexicon doctor [--last N] [--json]
                                (V13: observability snapshot. Reads
                                 ~/.claude/lexicon/{metrics,fires}.jsonl
                                 + hook.log; reports file growth, hook
                                 latency p50/p95, outcome distribution,
                                 estimated API spend, and alerts that
                                 catch silent degradation. --last N to
                                 widen rollup window past 100 calls.)
  lexicon tarot [--n N] [--seed N] [--under-review]
                                (V13: explicit oblique-provocation
                                 draw. Pure random N primitives from
                                 elements, framed as Eno cards. Hook
                                 auto-fires this same shape inline when
                                 lens detects stuck-signals; this is
                                 the manual override for when you want
                                 to draw regardless of signal.)
  lexicon lint [-v] [--json] [<lex-id> ...]
                                (V16: type-check assembly: fields
                                 against elements' typed primitives.
                                 Surfaces type-mismatches in sequential/
                                 parallel/choice, unresolvable atoms
                                 (forcing function for next mining
                                 pass), and decomposes-into ↔ assembly
                                 consistency.)
  lexicon lint-cross-refs [--json] [--show-shorthand]
                                (Unified cross-reference linter. One
                                 pass over elements + every tracked MD:
                                 flags stale lex-NNNN refs, concept-
                                 mismatch when cited name disagrees with
                                 actual atom name, and unreciprocated
                                 related[] edges. Replaces the older
                                 check-reciprocation.py + audit-stale-
                                 refs.py scripts. Exits non-zero on any
                                 error.)
  lexicon anti-patterns [--parent <lex-id>]
                                (V16: enumerate (n−1)-subsets of every
                                 molecule's decomposes-into and emit a
                                 markdown table of candidate failure-
                                 modes. Delivers composition-operations.md
                                 §"Why bonds matter".)
  lexicon distance <id-1> <id-2>
  lexicon distance --all [-k N]
  lexicon distance --markdown [-k N]
                                (V16: weighted molecular distance =
                                 w_atom·Jaccard(atoms) + w_assembly·TED
                                 + w_type·type-mismatch. Zhang-Shasha
                                 tree-edit distance over parsed
                                 assembly trees. The chess-opening-
                                 tree analog.)
  lexicon recovery <set.yaml> [--top-pairs N] [--chi-threshold X]
                                (V16: composition-recovery rate +
                                 per-entry χ² discriminativity +
                                 pairwise firing-pattern redundancy.
                                 Replaces the IB framing per round-2
                                 review.)
  lexicon zim-fetch <title> [--port N] [--slug S] [--raw]
                                (V25: fetch Wikipedia article via
                                 local gozimhttpd, pipe through pandoc
                                 for verbatim staking. Assumes
                                 gozimhttpd is running on the given
                                 port; start manually with
                                 'gozimhttpd -path <zim> -port N'.)
  lexicon calib [--top-k K] [-v]
                                (V54: derive embedding-gate threshold from
                                 held-out POS/NEG probe corpus. Prints
                                 distributions, sweep table, recommended
                                 LEXICON_GATE_THRESHOLD. Builtin corpus in
                                 render/internal/embedgate/probe.go; override
                                 with LEXICON_CALIB_CORPUS=/path.json.)
  lexicon replay-fires [-n N] [-v]
                                (V54.2: replay last N V13 fires from
                                 ~/.claude/lexicon/fires.jsonl through the V14
                                 embedding gate. Real-world validation —
                                 reports silence-rate vs fire-rate, V14 vs V13
                                 agreement on top-atom, V13-pick rank in V14
                                 top-K.)
  lexicon gaps [--no-cq-gaps] [--no-under-review] [--no-spinoff]
                                (V96: Tier 1 self-gap-detection — atoms
                                 with empty critical-questions, atoms
                                 with status: under-review, and unminted
                                 candidate sections aggregated across
                                 mining-pass docs. Roadmap #13 Tier 1.)
  lexicon want check [--detail] [--apply]
                                (V117 n: verify wanted-materials.md TOP
                                 block items against lexicon/refs/, plus any
                                 dirs in LEXICON_SIBLING_REFS_DIRS (optional,
                                 colon-separated — see render/.env.example).
                                 Author surname required for PRESENT verdict;
                                 year-only matches → AMBIGUOUS. --apply
                                 rewrites the file, removing confirmed-
                                 present items. --detail shows matched
                                 filename + source dir per hit.)
  lexicon backrefs <lex-NNNN> [--status STATUS] [--ids]
                                (V104 c: reverse related: traversal —
                                 print atoms that reference the target
                                 in their related: list, with name +
                                 status badge. Roadmap #16.2 from
                                 inkling sibling-project feedback.)
  lexicon patch-related <lex-id> <add|remove> <ref-id> [<ref-id> ...]
                                (V109 d: safe line-level patch of an
                                 atom's related: field in elements
                                 YAML (inline or block-list form).
                                 Replaces hand-rolled block-replace
                                 regex helpers that have wiped sibling
                                 atoms from multi-atom MDs. --dry-run
                                 prints the would-be line without
                                 writing.)
  lexicon renumber <plan|apply-rename|apply-content|next-id>
                                (2026-08-20: migrates lex-NNNN sequential
                                 ids to non-sequential 5-char opaque codes.
                                 plan generates/loads docs/id-migration-
                                 map.csv and reports a dry-run sweep;
                                 apply-rename git mv's elements files;
                                 apply-content rewrites every mapped
                                 occurrence in place; next-id prints one
                                 fresh unused id for minting new atoms.)
  lexicon mcp                   (V111 c: MCP stdio server exposing
                                 lexicon_read, lexicon_list,
                                 lexicon_extrapolate, lexicon_predict,
                                 lexicon_constellation, and
                                 lexicon_distinctness. Registered
                                 in ~/.claude.json so other Claude
                                 sessions can call the elements without
                                 shelling out. Hand-rolled JSON-RPC 2.0
                                 stdio matching the house style also used by
                                 github.com/justinstimatze/be-my-geminis and
                                 github.com/justinstimatze/hindcast.)
  lexicon coverage [--text] [--uncovered] [--refs-dir DIR] [--passes-dir DIR]
                                (Audit refs/ against docs/passes/ + elements
                                 lineage. For each long-form ref (pdf/epub/
                                 djvu/mobi/azw3/lit), extract (author-surname,
                                 year) and check if any mining-pass MD or
                                 elements YAML lineage covers it. Emits
                                 JSON summary {refs_total, covered, uncovered,
                                 items} by default; --text for TSV.
                                 --uncovered filters to just the gaps.
                                 NOTE: "covered" means a mining-pass MD
                                 exists, which happens well before the
                                 atom is finished — see "backlog" for a
                                 queue that ranks unfinished work first.)
  lexicon backlog [--text] [--kind all|under-review|uncovered] [--limit N]
                                (One ranked mining queue, ordered by
                                 in-degree — how many atoms point at the
                                 hole. Under-review atoms rank first
                                 because a shipped-but-unsettled atom is
                                 visible to readers; uncovered refs
                                 follow at rank 0, their true in-degree,
                                 since nothing points at them yet. Each
                                 under-review item carries whether its
                                 source is already in refs/ and how many
                                 lineage quote stubs are still empty.)
  lexicon anki [--out FILE] [--tier T] [--status S]
                                (Export elements as an Anki-importable
                                 TSV deck. One card per atom; front =
                                 id + name; back = agent-instruction +
                                 lineage + canonical example + related.
                                 Tags: lexicon, tier-N, status-N. Import
                                 via Anki's File → Import; deck uses the
                                 Basic note type with HTML enabled.)
  lexicon list [--text] [--tier T] [--status S]
                                (Flat enumeration of every atom: id,
                                 name, tier, status. Emits JSON by
                                 default (--text for TSV). Optional
                                 --tier / --status filters.)
  lexicon extrapolate <lex-id> [<lex-id> ...] [--text] [--top-k K]
                                (V114: adjacency-frontier read on a
                                 constellation of atom IDs. Pure
                                 elements-graph walk — no LLM, no
                                 model confound. Emits JSON by default
                                 (--text for human-readable). Reads
                                 IDs from args, or whitespace/comma-
                                 separated stdin if no args given.
                                 Motivated by sluice's forecasting
                                 back-test: feeding a constellation
                                 of fired atoms back as input names
                                 the atoms the gestalt invokes-but-
                                 doesn't-contain — the ontological
                                 negative space of the chosen frame.)
  lexicon partitions <file1> <file2> [<file3> ...] [--min-k K] [--top-k K] [--no-lens]
                                (Cross-domain firing aggregator: run
                                 pattern-id independently on each file
                                 (one passage per domain/partition) and
                                 surface atoms that fired in at least
                                 --min-k of them (default 2). A pattern
                                 firing in one domain alone is often
                                 that domain's local vocabulary; one
                                 firing across several unrelated domains
                                 is more likely a real slow-variable or
                                 scale-separated primitive. Emits JSON.)
  lexicon read [file|-] [--top-k K] [--no-lens] [--no-explain]
                                (V96: thin paste-into-shell alias for
                                 what-if --mode pattern-id --explain.
                                 Reads from a file, dash for stdin, or
                                 defaults to stdin if no arg. Translates
                                 the structured pattern-id output into
                                 plain conversational language for the
                                 reader. Source-agnostic — works on
                                 article content, transcripts, docs,
                                 code, anything pastable.)
  lexicon council [file|-] [--top-k K] [--no-lens] [--format json|text]
                                (same scoring engine as read/what-if
                                 pattern-id, different output shape: the
                                 top-K atoms as distinct voices, each
                                 arguing from its own agent-instruction,
                                 instead of one synthesized answer. For a
                                 stuck decision where you want a few
                                 independently-sourced takes rather than
                                 a report. --format text prints a
                                 readable council session; json (default)
                                 is {context, voices: [{id, name,
                                 type_in, type_out, score, voice}]}.)
  lexicon what-if --context "..."|- [--mode probe|greedy|intervene|pattern-id]
                                  [--top-k K] [--explain]
                                  [--max-probes N] [--depth N] [--no-lens]
                                (V36: emit disambiguation probes for an
                                 ambiguous situation BEFORE letting a card
                                 fire. Default --mode probe: non-interactive
                                 markdown for Claude to weave into
                                 conversation — generic fortune-teller
                                 staples + card-driven probes (type-in
                                 straddle, surfaced critical-questions).
                                 Elements-driven; future mesh with the
                                 sibling project inkling provides
                                 persistent user-mental-model. --mode
                                 greedy: V31 interactive REPL prototype,
                                 depth-N linear trajectory; see
                                 five-what-ifs-design.md.)

Modes (v0):
  algebraic         raw YAML elements
  meta-explanatory  what the entry is, where it sits, why it matters (default)
  narrative         deployment-mode prose, second-person, 100-300 words (LLM call)
  visual            mermaid decomposition diagram
  introspection     "you deployed X but didn't surface Y" (atoms, defeaters, lineage)

Narrative mode requires ANTHROPIC_API_KEY. Easiest path: put it in
the gitignored render/.env file (see render/.env.example).

Session vibe tags (per-session gut-check, not per-call):
  useful | mixed | not-useful | autonomous

Examples:
  lexicon render lex-kebfa
  lexicon render lex-kebfa --mode algebraic
  lexicon render lex-kebfa --mode narrative --context "your situation here"
  lexicon render lex-kebfa --why --context "design conversation"
  lexicon shuffle
  lexicon shuffle --n 5 --mode algebraic --filter tier=atomic
  lexicon shuffle --n 3 --filter status=active --seed 42
  lexicon gate --context "user mid-bind on expert claims"
  lexicon log autonomous --count 5 --notes "agent run, no human-driven calls"
`

func main() {
	// Resolve render/ dir from binary location so the CLI works from
	// any cwd. .env and session-log.jsonl live in render/; elements
	// lives at <repoRoot>/elements (a sibling of render/), reached
	// via <renderDir>/../elements.
	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = filepath.Abs(os.Args[0])
	}
	renderDir := resolveRenderDir(exePath)
	// Try .env from multiple known locations. When the binary is
	// invoked as a Claude Code hook from arbitrary cwds, resolveRenderDir
	// returns cwd (no elements marker visible from $HOME or /), so
	// render/.env wouldn't be found. ~/.claude/lexicon/.env is the
	// natural per-user fallback (the same dir hook.log + fires.jsonl
	// already live in). LoadDotEnv silently no-ops on missing files,
	// so trying multiple paths is cheap.
	for _, envPath := range envSearchPaths(renderDir) {
		if err := client.LoadDotEnv(envPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
	}

	if len(os.Args) < 2 {
		fmt.Print(help)
		os.Exit(0)
	}
	switch os.Args[1] {
	case "-h", "--help", "help":
		fmt.Print(help)
	case "render":
		cmdRender(renderDir, os.Args[2:])
	case "shuffle":
		cmdShuffle(renderDir, os.Args[2:])
	case "gate":
		cmdGate(renderDir, os.Args[2:])
	case "log":
		cmdLog(renderDir, os.Args[2:])
	case "hook":
		cmdHook(renderDir, os.Args[2:])
	case "mark-fire":
		cmdMarkFire(renderDir, os.Args[2:])
	case "pivot":
		cmdPivot(renderDir, os.Args[2:])
	case "matrix":
		cmdMatrix(renderDir, os.Args[2:])
	case "shell":
		cmdShell(renderDir, os.Args[2:])
	case "export-graph":
		cmdExportGraph(renderDir, os.Args[2:])
	case "reading-order":
		cmdReadingOrder(renderDir, os.Args[2:])
	case "doctor":
		cmdDoctor(renderDir, os.Args[2:])
	case "tarot":
		cmdTarot(renderDir, os.Args[2:])
	case "lint":
		cmdLint(renderDir, os.Args[2:])
	case "lint-cross-refs":
		cmdLintCrossRefs(renderDir, os.Args[2:])
	case "tier-derive":
		cmdTierDerive(renderDir, os.Args[2:])
	case "scaffolded-by":
		cmdScaffoldedBy(renderDir, os.Args[2:])
	case "db":
		cmdDB(renderDir, os.Args[2:])
	case "anti-patterns":
		cmdAntiPatterns(renderDir, os.Args[2:])
	case "distance":
		cmdDistance(renderDir, os.Args[2:])
	case "recovery":
		cmdRecovery(renderDir, os.Args[2:])
	case "zim-fetch":
		cmdZimFetch(renderDir, os.Args[2:])
	case "what-if":
		cmdWhatIf(renderDir, os.Args[2:])
	case "read":
		cmdRead(renderDir, os.Args[2:])
	case "council":
		cmdCouncil(renderDir, os.Args[2:])
	case "extrapolate":
		cmdExtrapolate(renderDir, os.Args[2:])
	case "partitions":
		cmdPartitions(renderDir, os.Args[2:])
	case "list":
		cmdList(renderDir, os.Args[2:])
	case "anki":
		cmdAnki(renderDir, os.Args[2:])
	case "coverage":
		cmdCoverage(renderDir, os.Args[2:])
	case "backlog":
		cmdBacklog(renderDir, os.Args[2:])
	case "refs":
		cmdRefs(renderDir, os.Args[2:])
	case "constellation":
		cmdConstellation(renderDir, os.Args[2:])
	case "distinctness":
		cmdDistinctness(renderDir, os.Args[2:])
	case "gaps":
		cmdGaps(renderDir, os.Args[2:])
	case "backrefs":
		cmdBackrefs(renderDir, os.Args[2:])
	case "want":
		cmdWant(renderDir, os.Args[2:])
	case "build-prototypes":
		cmdBuildPrototypes(renderDir, os.Args[2:])
	case "calib":
		cmdCalib(renderDir, os.Args[2:])
	case "replay-fires":
		cmdReplayFires(renderDir, os.Args[2:])
	case "redaction-audit":
		cmdRedactionAudit(os.Args[2:])
	case "redaction-hook":
		cmdRedactionHook()
	case "patch-related":
		cmdPatchRelated(renderDir, os.Args[2:])
	case "renumber":
		cmdRenumber(renderDir, os.Args[2:])
	case "mcp":
		cmdMCP(renderDir)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, help)
		os.Exit(2)
	}
}

// envSearchPaths returns the .env locations to try, in order. First-
// found wins per LoadDotEnv's "don't overwrite already-set" semantic.
func envSearchPaths(renderDir string) []string {
	paths := []string{
		filepath.Join(renderDir, ".env"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".claude", "lexicon", ".env"))
	}
	return paths
}

// resolveRenderDir picks the render/ directory based on cwd or the
// binary path. The repo root is identified as the directory that
// contains both elements/ and render/ as subdirectories; renderDir
// is then <repoRoot>/render. Falls back to cwd if nothing matches
// (e.g., the CLI running as a Claude Code hook from $HOME).
func resolveRenderDir(exePath string) string {
	cwd, _ := os.Getwd()
	if rr := findRepoRoot(cwd); rr != "" {
		return filepath.Join(rr, "render")
	}
	d := filepath.Dir(exePath)
	for i := 0; i < 5; i++ {
		if rr := findRepoRoot(d); rr != "" {
			return filepath.Join(rr, "render")
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rr := findRepoRoot(filepath.Join(home, "Documents", "lexicon")); rr != "" {
			return filepath.Join(rr, "render")
		}
	}
	return cwd
}

// findRepoRoot walks up from start looking for a directory containing
// both elements/ and render/. Returns "" if none found within 5 levels.
func findRepoRoot(start string) string {
	d := start
	for i := 0; i < 5; i++ {
		_, errS := os.Stat(filepath.Join(d, "elements"))
		_, errR := os.Stat(filepath.Join(d, "render"))
		if errS == nil && errR == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return ""
}

func fatal(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+msg+"\n", args...)
	os.Exit(1)
}
