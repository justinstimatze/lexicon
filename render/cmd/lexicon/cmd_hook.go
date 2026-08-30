package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/embedgate"
	"github.com/justinstimatze/lexicon/render/internal/framestatus"
	"github.com/justinstimatze/lexicon/render/internal/gate"
	"github.com/justinstimatze/lexicon/render/internal/lens"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
	pkglexicon "github.com/justinstimatze/lexicon/render/pkg/lexicon"
)

// cmdHook is the UserPromptSubmit Claude Code hook entry point. It
// reads a JSON envelope from stdin, runs lexicon's gate against the
// prompt, and emits an additionalContext injection if any matched
// primitive clears the relevance threshold. Designed to be defensive
// — every error path swallows silently (logging to ~/.claude/lexicon/
// hook.log) so a broken lexicon never blocks a Claude turn.
//
// Wire by adding to ~/.claude/settings.json:
//
//	{
//	  "hooks": {
//	    "UserPromptSubmit": [
//	      {"hooks": [{"type": "command", "command": "lexicon hook"}]}
//	    ]
//	  }
//	}
//
// Skip env vars (parallel to hindcast's convention, github.com/justinstimatze/hindcast):
//   - LEXICON_SKIP=1 — short-circuit before any work
//   - LEXICON_ELEMENTS_DIR=/path — override elements location
//     (required when invoked from arbitrary CWDs, since hooks run from
//     wherever Claude Code happens to be)
//
// V0.5 instrumentation env vars (per surfacing-function-utility-pass-1.md):
//   - LEXICON_HOOK_THRESHOLD=<float> — override hookScoreThreshold at runtime
//     (for threshold-tuning experiments without recompiling). Default 0.95
//     (raised from 0.84 in V12 after fires.jsonl analysis showed the 0.84
//     floor produced ~69% noise; per fires-jsonl-analysis-pass-1.md R3)
//   - LEXICON_CAPTURE_PROMPTS=1 — opt-in to storing full prompt body in
//     fires.jsonl (default off — only the 120-char snippet is stored)
//   - LEXICON_LENS_DISABLED=1 — skip the V12 LLM-backed lens layer
//     (hook falls back to pure lexical gating on the full pool — the
//     pre-V12 behavior). Useful for offline runs and threshold-tuning
//     experiments where the lens noise floor would confound results.
//   - LEXICON_HOOK_TOP_K=<int> — override how many primitives are injected
//     per fire (default 3; clamped to [1, 8]). Set to 1 for sparser
//     injections (per fires-jsonl-analysis-pass-2 open question 3 — the
//     hook fires on ~84% of prompts, denser than slimemold).
//   - LEXICON_LENS_MIN_CONFIDENCE=<float> — skip the fire when the lens's
//     top-confidence pick is below this. Default 0.7. Set to 0 to disable
//     (V12 behavior). Only enforced when the lens actually ran (disabled /
//     errored paths fall back to lexical threshold).
//   - LEXICON_METRICS_DISABLED=1 — skip writing per-call timing data to
//     ~/.claude/lexicon/metrics.jsonl. Default off (metrics are written).
//     `lexicon doctor` reads metrics.jsonl for latency/outcome rollups.
//   - LEXICON_EMBED_GATE_BUDGET_MS=<int> — hard wall-clock cap on the embed
//     stage (default 6000). On expiry the gate fails soft to the full-pool
//     lens. The gate is warm-only (it never builds prototypes on this path —
//     that's `lexicon build-prototypes`, run off-path), so this only bounds
//     the single prompt embed.
//   - LEXICON_LENS_BUDGET_MS=<int> — backstop wall-clock cap on the lens
//     stage (default 12000), above lens's own inner timeout. Guarantees the
//     hook returns even if the lens stalls; on expiry it falls back to
//     lexical full-pool gating.
//   - LEXICON_ELEMENTS_LOAD_BUDGET_MS=<int> — hard wall-clock cap on
//     loader.LoadAll (default 6000). Bounds disk/memory-pressure stalls
//     (observed up to 117s on this host); on expiry the hook exits with
//     outcome "error-loader-timeout", distinct from "error-loader"
//     (a real bad-path/unreadable-dir failure).
//
// V0.5 logs written:
//   - ~/.claude/lexicon/hook.log (existing) — human-readable one-liner
//   - ~/.claude/lexicon/fires.jsonl (NEW) — structured per-fire JSONL with
//     hook_event_id, prompt_hash, top_3 results. Enables retrospective
//     per-fire utility analysis. Pair with `lexicon mark-fire <id> <vibe>`
//     to tag specific fires as useful / mixed / not-useful / autonomous.
type hookInput struct {
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	Prompt         string `json:"prompt"`
	HookEventName  string `json:"hook_event_name"`
	PermissionMode string `json:"permission_mode"`
	TranscriptPath string `json:"transcript_path"`
}

const (
	hookScoreThreshold   = 0.95 // don't inject if top score below this (V12: raised from 0.84; see fires-jsonl-analysis-pass-1.md R3)
	hookTopK             = 3
	hookExampleMaxLen    = 220
	hookPromptSnippetLen = 120  // first-N-chars captured into fires.jsonl
	lensMinConfidence    = 0.7  // V13: skip fire when lens top-conf below this
	hookAmbiguityGap     = 0.15 // V71: if top1-top2 score gap < this, the match is ambiguous — interview instead of asserting one pick
)

// fireRecord is the per-fire structured log line in fires.jsonl. Written
// for every gate run that clears threshold (matches the hook.log
// "injected ..." line exactly). Enables per-fire utility analysis that
// the original hook.log structurally cannot support.
type fireRecord struct {
	Ts            string             `json:"ts"`
	HookEventID   string             `json:"hook_event_id"`
	SessionID     string             `json:"session_id"`
	PromptHash    string             `json:"prompt_hash"`
	PromptSnippet string             `json:"prompt_snippet"`
	PromptFull    string             `json:"prompt_full,omitempty"`
	Threshold     float64            `json:"threshold"`
	VocabSize     int                `json:"vocab_size"`
	TopResults    []fireResultRecord `json:"top_results"`
}

type fireResultRecord struct {
	PrimitiveID    string  `json:"primitive_id"`
	Name           string  `json:"name"`
	Score          float64 `json:"score"`
	LensConfidence float64 `json:"lens_confidence,omitempty"` // V13: per-result lens confidence (0 if lens didn't run / V12-shape fallback)
}

func cmdHook(renderDir string, args []string) {
	// V13 perf instrumentation: every exit path writes one line to
	// metrics.jsonl. The two defers are LIFO — the panic-recover defer
	// is registered LAST so it runs FIRST, recording the outcome before
	// the metrics-write defer fires.
	wallStart := time.Now()
	rec := metricsRecord{Ts: nowTs()}
	defer func() {
		rec.TotalMs = time.Since(wallStart).Milliseconds()
		writeMetricsRecord(rec)
	}()
	defer func() {
		if r := recover(); r != nil {
			if rec.Outcome == "" {
				rec.Outcome = "panic"
			}
			hookLog("hook", "PANIC: %v", r)
		}
	}()
	if os.Getenv("LEXICON_SKIP") == "1" {
		rec.Outcome = "skipped-env"
		return
	}
	var in hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		rec.Outcome = "error-stdin"
		hookLog("hook", "stdin decode: %s", err)
		return
	}
	rec.SessionID = in.SessionID
	if strings.TrimSpace(in.Prompt) == "" {
		rec.Outcome = "skipped-empty-prompt"
		return
	}
	// V54.2: filter Claude Code system events that arrive on
	// UserPromptSubmit but are not user input. replay-fires surfaced 4
	// task-notification events firing at sim ~0.60 on lex-mjav2. These
	// share an XML-ish prefix that's unambiguous; filtering here costs
	// zero real-prompt recall, vs. raising the gate threshold which
	// would.
	if isSystemEventPrompt(in.Prompt) {
		rec.Outcome = "skipped-system-event"
		return
	}

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	tElements := time.Now()
	pool, err := loadElementsWithTimeout(elementsDir, resolveElementsLoadBudget())
	rec.ElementsMs = time.Since(tElements).Milliseconds()
	if err != nil {
		if errors.Is(err, errLoadTimeout) {
			rec.Outcome = "error-loader-timeout"
		} else {
			rec.Outcome = "error-loader"
		}
		hookLog("hook", "loader: %s", err)
		return
	}
	entries := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		entries = append(entries, e)
	}
	rec.PoolSize = len(entries)

	// V71: optionally enrich the match input with recent user-turn text
	// (LEXICON_CONTEXT_TURNS>0) so the match sharpens on the accumulated
	// situation, not just this one prompt. Default off → matchText == prompt.
	// in.Prompt is still used for system-event filtering + fire records.
	matchText := recentUserContext(in.TranscriptPath, in.Prompt, resolveContextTurns())

	// V14 embedding pre-filter: ollama-local cosine vs per-atom prototypes.
	// Cheap-first (github.com/justinstimatze/cupel's pattern): if top-K embed match
	// >= threshold, narrow `entries` to those K candidates before the V12
	// Haiku lens runs — the lens then judges K~20 atoms instead of the whole
	// elements.
	// Fail-soft: on ollama-unreachable / timeout, log + fall through with
	// full pool (pre-V14 behavior). If top sim < threshold, short-circuit
	// to silence (skip lens entirely; cupel's gate-then-trip semantic —
	// see the cheap-first comment above for the link).
	// Hard wall-clock cap on the embed stage. The gate is a cheap-FIRST
	// optimization that embeds only the short prompt and scores it against the
	// PREBAKED prototype cache — it never builds prototypes on this path (that's
	// a minutes-long off-path job; see embedgate.Score). The budget guards the
	// one remaining variable cost: the single prompt embed against a possibly
	// oversubscribed local ollama runner. On a cold cache Score returns
	// ErrColdCache and we fall straight through to the full-pool lens.
	tEmbed := time.Now()
	embedCtx, embedCancel := context.WithTimeout(context.Background(), pkglexicon.ResolveEmbedGateBudget())
	gateRes, gateErr := embedgate.Score(embedCtx, matchText, entries, embedgate.TopK())
	embedCancel()
	rec.EmbedGateMs = time.Since(tEmbed).Milliseconds()
	if errors.Is(gateErr, embedgate.ErrColdCache) {
		rec.Outcome = "embed-gate-cold"
		hookLog("hook", "embed gate: prototype cache cold; using full-pool lens (warm off-path: lexicon build-prototypes)")
	} else if gateErr != nil {
		hookLog("hook", "embed gate: %s (falling back to full-pool lens)", gateErr)
	} else if len(gateRes) > 0 {
		rec.EmbedTopSim = gateRes[0].Score
		narrowed, active, logMsg := pkglexicon.DecideEmbedNarrowing(gateRes, pool, embedgate.Threshold(), rec.PoolSize)
		entries = narrowed
		rec.EmbedActive = active
		hookLog("hook", "%s", logMsg)
	}

	// V12 lens layer: ask Haiku which entries are SEMANTICALLY relevant
	// before the gate scores them. Fail-soft — on timeout or error, the
	// lens returns the full pool so the gate behaves as it did pre-V12.
	// A nil slice from lens means "explicitly nothing relevant" (LLM
	// returned []), which the hook honors by skipping.
	// V14: `entries` may already be narrowed by the embedding pre-filter
	// above; the lens then operates on the embed-top-K rather than all atoms.
	candidatePool := entries
	lensActive := false // true once we've confirmed lens filtered something
	lensDisabled := lens.Disabled()
	var llmClient client.Client
	if !lensDisabled {
		llmClient, err = client.New()
		if err != nil {
			hookLog("hook", "lens: client init: %s (falling back to lexical)", err)
			lensDisabled = true
		}
	}
	rec.LensCalled = !lensDisabled
	// Backstop wall-clock cap on the lens stage. lens.Filter already self-caps
	// at its own (env-tunable) DefaultTimeout, so in the normal case the inner
	// deadline wins; this outer cap is defense-in-depth — it guarantees the hook
	// returns even if the inner timeout is misconfigured high or the SDK retries
	// past it. On expiry lens.Filter's error path falls back to the lexical
	// full-pool flow.
	tLens := time.Now()
	lensCtx, lensCancel := context.WithTimeout(context.Background(), resolveLensBudget())
	lensRes, lensErr := lens.Filter(lensCtx, matchText, entries, llmClient, lensDisabled)
	lensCancel()
	rec.LensMs = time.Since(tLens).Milliseconds()
	rec.InputTokens = lensRes.Usage.InputTokens
	rec.OutputTokens = lensRes.Usage.OutputTokens
	rec.CacheReadTokens = lensRes.Usage.CacheReadTokens
	rec.CacheCreationTokens = lensRes.Usage.CacheCreationTokens
	if lensErr != nil {
		hookLog("hook", "lens: %s (falling back to lexical full-pool)", lensErr)
	} else if !lensDisabled {
		// V13 TASK 8: even if Mode 1 (informed picks) is empty, the
		// lens's stuck/contradiction signals can still trigger Mode 2
		// (tarot) or Mode 3 (contradiction placeholder). So we don't
		// short-circuit here — fall through and check the signals
		// below before deciding to silence.
		if len(lensRes.Entries) > 0 && lensRes.Confidences != nil {
			rec.LensTopConf = lensRes.Confidences[lensRes.Entries[0].ID]
			minConf := resolveLensMinConfidence()
			if rec.LensTopConf < minConf {
				// Mode 1 disqualified by conf gate. Clear Entries so
				// downstream code doesn't surface low-conf picks; but
				// auto-trigger signals (Stuck, Contradiction) still
				// have their say below.
				hookLog("hook", "lens: top conf %.2f below threshold %.2f (top=%s); Mode 1 silenced",
					rec.LensTopConf, minConf, lensRes.Entries[0].ID)
				lensRes.Entries = nil
			}
		}
		if len(lensRes.Entries) > 0 {
			candidatePool = lensRes.Entries
			lensActive = true
			hookLog("hook", "lens: filtered %d -> %d candidates", len(entries), len(candidatePool))
		}
	}

	// V13 TASK 8: Mode 1 emission. When lens is active and has picks,
	// trust lens order (skip gate re-rank since gate's lexical scores
	// would scramble lens-confidence ordering). When lens is
	// disabled/errored, fall back to gate scoring with threshold.
	var results []types.GateResult
	vocab := pkglexicon.ExtractPromptVocab(matchText)
	if lensActive {
		// Lens already chose + ranked; convert to gate-result shape so
		// downstream code is uniform. Score is the lens confidence
		// (0-1) for fires.jsonl auditability.
		topK := resolveHookTopK()
		for i, e := range candidatePool {
			if i >= topK {
				break
			}
			results = append(results, types.GateResult{
				PrimitiveID: e.ID,
				ModeHint:    types.ModeMetaExplanatory,
				Score:       lensRes.Confidences[e.ID],
				Reason:      "lens",
			})
		}
		rec.GateMs = 0
	} else {
		// Fallback path only (lens disabled/errored) — the lensActive
		// branch above already trusts lens order directly and never
		// reaches here. lensRes was assigned unconditionally above, so
		// lensRes.Confidences may still carry entries even when Mode-1
		// picks were cleared by the confidence gate; threading it here is
		// safe — candidatePool is the full pool in this branch, so only
		// the handful of IDs the lens actually scored will hit a non-nil
		// map entry, everything else takes the no-op path.
		fsMap, fsErr := framestatus.Load(renderDir)
		if fsErr != nil {
			hookLog("hook", "frame-status: %s (running without frame down-weighting)", fsErr)
		}
		tGate := time.Now()
		results = gate.Run(gate.Input{
			Pool:         candidatePool,
			Context:      matchText,
			WorkingVocab: vocab,
			TopK:         resolveHookTopK(),
			Confidences:  lensRes.Confidences,
			FrameStatus:  fsMap,
		})
		rec.GateMs = time.Since(tGate).Milliseconds()
		threshold := resolveHookThreshold()
		if len(results) == 0 || results[0].Score < threshold {
			results = nil
		}
	}

	// Quiet-mode gates (V116 f). Applied AFTER lens/gate score, BEFORE
	// formatting. Both default-on; either can be disabled via env var.
	//
	// (A) Require agent-instruction: skip the fire unless at least one
	// matched atom carries an authored agent-instruction. The hook is
	// only useful when it has something operational to say; naming a
	// pattern without a decision rule is noise. As the elements'
	// agent-instruction coverage grows, this gate progressively un-mutes.
	if resolveHookRequireInstruction() && len(results) > 0 && !anyResultHasInstruction(results, pool) {
		rec.Outcome = "silence-no-instruction"
		hookLog("hook", "skip: no agent-instruction across %d match(es)", len(results))
		return
	}
	// (B) Skip-on-lexicon-internal: when the prompt is about lexicon
	// itself (contains `lex-NNNN` references), the hook injection is
	// likely redundant or recursive. Skip.
	if resolveHookSkipInternal() && isLexiconInternalPrompt(in.Prompt) {
		rec.Outcome = "silence-internal-conversation"
		hookLog("hook", "skip: prompt is lexicon-internal")
		return
	}

	// Build the slimemold-shaped output. Mode 1 (informed) emits when
	// `results` is non-empty. Mode 2 (tarot) appends when StuckSignal
	// fired. Mode 3 (contradiction placeholder) appends when
	// ContradictionSignal fired. All three can fire in one injection.
	contextStr := formatHookInjection(results, pool, lensRes)
	if contextStr == "" {
		// Nothing to surface — no Mode 1 fire AND no Mode 2/3 signals.
		if lensActive && len(results) == 0 {
			rec.Outcome = "silence-conf" // already logged above
		} else if !lensActive {
			rec.Outcome = "silence-threshold"
			hookLog("hook", "no result above threshold (top=%.2f)", topScore(results))
		} else {
			rec.Outcome = "silence-empty-context"
		}
		return
	}
	emitHookResponse(contextStr)

	threshold := resolveHookThreshold()
	if lensActive {
		threshold = 0
	}
	eventID := writeFireRecord(in, vocab, results, pool, threshold, lensRes.Confidences)
	rec.Outcome = "fire"
	rec.HookEventID = eventID
	topPrimitive := "(no Mode 1 pick)"
	topScore := 0.0
	if len(results) > 0 {
		topPrimitive = results[0].PrimitiveID
		topScore = results[0].Score
	}
	hookLog("hook", "injected primitives=%d stuck=%t contradiction=%t (top=%s @ %.2f) event=%s",
		len(results), lensRes.StuckSignal, lensRes.ContradictionSignal, topPrimitive, topScore, eventID)
}

// resolveLensMinConfidence honors LEXICON_LENS_MIN_CONFIDENCE if set +
// parseable to a float in [0, 1]. Falls back to compiled-in default
// (lensMinConfidence = 0.7). 0 disables the gate entirely (V12 behavior).
func resolveLensMinConfidence() float64 {
	envC := os.Getenv("LEXICON_LENS_MIN_CONFIDENCE")
	if envC == "" {
		return lensMinConfidence
	}
	v, err := strconv.ParseFloat(envC, 64)
	if err != nil || v < 0 || v > 1 {
		hookLog("hook", "LEXICON_LENS_MIN_CONFIDENCE=%q invalid; using default %.2f", envC, lensMinConfidence)
		return lensMinConfidence
	}
	return v
}

// lexiconInternalRefRe detects lex-NNNN references in prompt text. A
// single match is enough — if the user is talking about an atom by id,
// the hook firing on the same prompt is almost always redundant.
var lexiconInternalRefRe = regexp.MustCompile(`\blex-[23456789abcdefghjkmnpqrstuvwxyz]{5}\b`)

// isLexiconInternalPrompt returns true when the prompt is about lexicon
// itself (currently signaled by any lex-NNNN reference). Kept narrow:
// generic words like "atom" / "elements" / "molecule" are too overloaded
// to use as the signal. Future versions can layer in additional
// signals (mining-pass mentions, drift/reciprocation, etc.) if the false-
// positive rate of plain-prose firing on lexicon-internal topics rises.
func isLexiconInternalPrompt(prompt string) bool {
	return lexiconInternalRefRe.MatchString(prompt)
}

// anyResultHasInstruction returns true when at least one matched atom
// in results carries an authored agent-instruction. Used by gate (A) of
// quiet-mode: if no result has an agent-instruction, the hook has nothing
// operational to inject — silence is preferable to bare naming.
func anyResultHasInstruction(results []types.GateResult, pool map[string]*types.LexEntry) bool {
	for _, r := range results {
		if e := pool[r.PrimitiveID]; e != nil && strings.TrimSpace(e.AgentInstruction) != "" {
			return true
		}
	}
	return false
}

// resolveHookRequireInstruction honors LEXICON_HOOK_REQUIRE_INSTRUCTION
// (default "1" — on). Set to "0" to disable gate (A) and restore the
// pre-quiet-mode firing behavior. Useful for elements-fill sessions
// where the bare-naming surface still has value.
func resolveHookRequireInstruction() bool {
	v := os.Getenv("LEXICON_HOOK_REQUIRE_INSTRUCTION")
	if v == "" {
		return true
	}
	return v != "0"
}

// resolveHookSkipInternal honors LEXICON_HOOK_SKIP_INTERNAL (default "1"
// — on). Set to "0" to disable gate (B) and restore firing on
// lexicon-internal prompts.
func resolveHookSkipInternal() bool {
	v := os.Getenv("LEXICON_HOOK_SKIP_INTERNAL")
	if v == "" {
		return true
	}
	return v != "0"
}

// resolveHookTopK honors LEXICON_HOOK_TOP_K if set + parseable to an int
// in [1, 8]. Falls back to compiled-in default (hookTopK = 3). Out-of-bounds
// is misconfiguration and ignored with a log line.
func resolveHookTopK() int {
	envK := os.Getenv("LEXICON_HOOK_TOP_K")
	if envK == "" {
		return hookTopK
	}
	v, err := strconv.Atoi(envK)
	if err != nil || v < 1 || v > 8 {
		hookLog("hook", "LEXICON_HOOK_TOP_K=%q invalid; using default %d", envK, hookTopK)
		return hookTopK
	}
	return v
}

// Per-stage wall-clock budgets for the hook's two slow stages. A hook must
// never hang a Claude turn; these are the hard ceilings that make that true.
// Defaults are chosen well above observed steady-state latency (p99 total ~6s)
// so normal traffic never trips them, but far below the harness's 30s hook cap
// so a transient slow call (the single prompt embed against an oversubscribed
// ollama runner, or a slow Haiku response) is cut off and falls back gracefully
// instead of being killed by the harness and discarded. The prototype BUILD is
// no longer on this path (Score is warm-only), so the prior multi-minute tail
// is gone regardless of these budgets — they guard the remaining variable costs.
const embedGateBudget = 6 * time.Second // local-embed stage (ollama)
const lensBudget = 12 * time.Second     // Haiku lens stage; backstop above lens's own inner timeout
// elementsLoadBudget bounds loader.LoadAll — a plain sequential
// ReadDir+ReadFile+YAML-parse loop that should run well under a second
// locally but has been observed taking 20-117s on this host under disk
// or memory pressure (metrics.jsonl: t_substrate_ms up to 116985 on
// otherwise-unremarkable calls). loader.LoadAll has no context of its
// own to cancel, so this is enforced via loadElementsWithTimeout below.
//
// 6s, not a more generous value: metrics.jsonl's real p99.9 load time is
// 8407ms, so a 8s+ budget mostly just delays killing the calls that were
// never going to matter anyway — the settings.json hook wiring's own
// 10s outer timeout has already discarded the whole invocation by the
// time loader+embed+lens could plausibly finish past that point (p50
// load is 499ms; the embed gate and lens stages still need their own
// few seconds after it). Matches embedGateBudget's value on the same
// reasoning. This budget stops the pathological (20s+) tail from
// burning CPU/disk in the background after Claude Code has already
// moved on — it isn't trying to rescue borderline-slow-but-real loads,
// since those were already lost to the outer timeout regardless.
const elementsLoadBudget = 6 * time.Second

func resolveElementsLoadBudget() time.Duration {
	return resolveDurationMs("LEXICON_ELEMENTS_LOAD_BUDGET_MS", elementsLoadBudget)
}

// loadElementsWithTimeout runs loader.LoadAll on a goroutine and gives up
// after budget rather than blocking the turn indefinitely. On timeout the
// goroutine is left to finish on its own (Go can't interrupt a blocking
// os.ReadFile mid-syscall) — its result lands in the buffered channel and
// is simply never read.
// errLoadTimeout is a sentinel so the caller can tell "the elements dir
// couldn't be read at all" (error-loader — a real config problem, e.g.
// the stale-binary/wrong-path bug this budget was added alongside) apart
// from "it was still loading when the budget ran out" (error-loader-
// timeout — a transient disk/memory-pressure symptom). Conflating the two
// in one outcome bucket is exactly what made the former hard to spot: it
// took reading raw hook.log error text, not the structured Outcome field,
// to find it.
var errLoadTimeout = errors.New("elements load exceeded budget")

func loadElementsWithTimeout(dir string, budget time.Duration) (map[string]*types.LexEntry, error) {
	type result struct {
		pool map[string]*types.LexEntry
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		pool, err := loader.LoadAll(dir)
		ch <- result{pool, err}
	}()
	select {
	case r := <-ch:
		return r.pool, r.err
	case <-time.After(budget):
		return nil, fmt.Errorf("%w: %s (dir: %s)", errLoadTimeout, budget, dir)
	}
}

// resolveLensBudget honors LEXICON_LENS_BUDGET_MS if set + parseable to a
// positive int. Default lensBudget.
func resolveLensBudget() time.Duration {
	return resolveDurationMs("LEXICON_LENS_BUDGET_MS", lensBudget)
}

// resolveDurationMs reads a millisecond duration from env, falling back to def
// on absence or invalid input (logged once).
func resolveDurationMs(envKey string, def time.Duration) time.Duration {
	v := os.Getenv(envKey)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		hookLog("hook", "%s=%q invalid; using default %s", envKey, v, def)
		return def
	}
	return time.Duration(n) * time.Millisecond
}

const hookContextTurns = 0 // V71: default OFF; LEXICON_CONTEXT_TURNS>0 enriches the match input with that many recent user turns

// resolveContextTurns honors LEXICON_CONTEXT_TURNS if set + parseable to
// an int in [0, 20]. Default 0 (off) — the match runs on the single
// current prompt, as it always has. >0 prepends that many recent
// user-turn texts so the match sharpens on the accumulated situation.
// Default-off because feeding multi-turn text shifts the embed/lens/gate
// scores those thresholds were calibrated on; opt in to validate first.
func resolveContextTurns() int {
	envT := os.Getenv("LEXICON_CONTEXT_TURNS")
	if envT == "" {
		return hookContextTurns
	}
	v, err := strconv.Atoi(envT)
	if err != nil || v < 0 || v > 20 {
		hookLog("hook", "LEXICON_CONTEXT_TURNS=%q invalid; using default %d", envT, hookContextTurns)
		return hookContextTurns
	}
	return v
}

// recentUserContext enriches the match input with recent user-turn text
// from the transcript — the deeper fix for "a single prompt is too thin
// to match patterns." Reads up to `turns` most-recent genuine user
// messages (content is a plain string; tool-result/image turns, whose
// content is a list, are skipped, as are system events) and prepends them
// to the current prompt (current prompt last, so it dominates lexical /
// embedding weight). Fail-soft: any error (turns<=0, no path, unreadable,
// unparseable) returns currentPrompt unchanged. V71.
func recentUserContext(transcriptPath, currentPrompt string, turns int) string {
	if turns <= 0 || transcriptPath == "" {
		return currentPrompt
	}
	f, err := os.Open(transcriptPath)
	if err != nil {
		return currentPrompt
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return currentPrompt
	}

	// Walk backward and stop as soon as `turns` messages are in hand, so the
	// cost is proportional to what is wanted rather than to session length.
	// This runs on the blocking UserPromptSubmit path and transcripts on a
	// long-running host reach several GB; the forward scan this replaced read
	// every line of one to keep the last few.
	trimmedCurrent := strings.TrimSpace(currentPrompt)
	var newestFirst []string
	sawNewest := false
	// A read error (only reachable via an oversized single line) leaves
	// whatever was collected in place rather than failing the turn, matching
	// the unchecked sc.Err() of the scanner this replaced.
	_ = eachLineBackward(f, fi.Size(), func(line []byte) bool {
		var rec struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.Type != "user" {
			return true
		}
		var s string
		if json.Unmarshal(rec.Message.Content, &s) != nil { // list content (tool-result/image) → skip
			return true
		}
		if s = strings.TrimSpace(s); s == "" || isSystemEventPrompt(s) {
			return true
		}
		// Drop a trailing duplicate of the current prompt (it's usually the
		// last user line in the transcript) so it isn't counted twice. Only
		// the newest qualifying message is eligible, which is what the
		// forward scan's last-element check amounted to.
		if !sawNewest {
			sawNewest = true
			if s == trimmedCurrent {
				return true
			}
		}
		newestFirst = append(newestFirst, s)
		return len(newestFirst) < turns
	})

	if len(newestFirst) == 0 {
		return currentPrompt
	}
	userTexts := make([]string, 0, len(newestFirst)+1)
	for i := len(newestFirst) - 1; i >= 0; i-- { // back to file order
		userTexts = append(userTexts, newestFirst[i])
	}
	return strings.Join(append(userTexts, currentPrompt), "\n")
}

// tailChunk is how much of a transcript eachLineBackward pulls per step, and
// maxCarry bounds a single line spanning chunk boundaries — the same ceiling
// the bufio.Scanner in recentUserContext used before it read backward.
const (
	tailChunk = 256 << 10
	maxCarry  = 8 << 20
)

// eachLineBackward walks the lines of f from the end toward the start, calling
// fn on each and stopping when fn returns false. A line straddling a chunk
// boundary is carried down and rejoined, so fn never sees a fragment.
//
// Adapted from cope/internal/transcript/transcript.go, which grew the same
// reader for the same reason.
func eachLineBackward(f *os.File, size int64, fn func(line []byte) bool) error {
	var carry []byte
	for off := size; off > 0; {
		n := int64(tailChunk)
		if n > off {
			n = off
		}
		off -= n

		buf := make([]byte, n, n+int64(len(carry)))
		if _, err := f.ReadAt(buf, off); err != nil {
			return err
		}
		buf = append(buf, carry...)

		lines := bytes.Split(buf, []byte{'\n'})
		// lines[0] is a whole line only when the window reaches the start of
		// the file. Anywhere else it is a fragment to carry down.
		first := 1
		if off == 0 {
			first = 0
		}
		for i := len(lines) - 1; i >= first; i-- {
			if !fn(lines[i]) {
				return nil
			}
		}
		if off == 0 {
			return nil
		}
		carry = lines[0]
		if len(carry) > maxCarry {
			return fmt.Errorf("a single transcript line exceeds %d bytes", maxCarry)
		}
	}
	return nil
}

// resolveAmbiguityGap honors LEXICON_AMBIGUITY_GAP if set + parseable to
// a float in [0, 1]. Falls back to the compiled-in default
// (hookAmbiguityGap = 0.15). The gap is the minimum score-separation
// between the #1 and #2 match below which the hook treats the match as
// ambiguous and asks the agent to interview rather than assert.
func resolveAmbiguityGap() float64 {
	envG := os.Getenv("LEXICON_AMBIGUITY_GAP")
	if envG == "" {
		return hookAmbiguityGap
	}
	v, err := strconv.ParseFloat(envG, 64)
	if err != nil || v < 0 || v > 1 {
		hookLog("hook", "LEXICON_AMBIGUITY_GAP=%q invalid; using default %.2f", envG, hookAmbiguityGap)
		return hookAmbiguityGap
	}
	return v
}

// ambiguityInterview returns an interview directive when the top two
// matches are too close to assert a single winner (score gap below the
// tunable threshold). It tells the in-loop agent to bisect — ask one
// distinguishing question and let the next turn re-match — rather than
// assert a possibly-wrong top pick on thin single-prompt input (the "a
// sentence is too thin to match" problem). Returns "" when one candidate
// dominates (assert as normal) or there's only one match. Strictly
// additive to the existing injection. V71.
func ambiguityInterview(results []types.GateResult, pool map[string]*types.LexEntry, gap float64) string {
	if len(results) < 2 {
		return ""
	}
	delta := results[0].Score - results[1].Score
	if delta >= gap {
		return ""
	}
	a, aok := pool[results[0].PrimitiveID]
	bb, bok := pool[results[1].PrimitiveID]
	if !aok || !bok {
		return ""
	}
	return fmt.Sprintf(
		"\nAmbiguous — top matches are close (Δ=%.2f < %.2f), so a one-shot pick on this much context is unreliable. Prefer to INTERVIEW: ask the user one question that would distinguish **%s** (%s) from **%s** (%s), then let the next turn re-match. Don't assert a single pattern yet.\n",
		delta, gap, a.Name, a.ID, bb.Name, bb.ID,
	)
}

// resolveHookThreshold honors LEXICON_HOOK_THRESHOLD if set + parseable
// to a sane positive float. Falls back to the compiled-in default
// (hookScoreThreshold). Sane bounds: (0, 10) — anything outside is treated
// as misconfiguration and ignored.
func resolveHookThreshold() float64 {
	envT := os.Getenv("LEXICON_HOOK_THRESHOLD")
	if envT == "" {
		return hookScoreThreshold
	}
	v, err := strconv.ParseFloat(envT, 64)
	if err != nil || v <= 0 || v >= 10 {
		hookLog("hook", "LEXICON_HOOK_THRESHOLD=%q invalid; using default %.2f",
			envT, hookScoreThreshold)
		return hookScoreThreshold
	}
	return v
}

// writeFireRecord appends one structured line to ~/.claude/lexicon/fires.jsonl.
// Returns the generated hook_event_id so it can be referenced in the
// human-readable hook.log line (so a user reading either log can find
// the corresponding fires.jsonl entry). V13: also records per-result
// lens confidence when available (zero when lens didn't run or fell
// back to V12 string-array shape).
func writeFireRecord(in hookInput, vocab []string, results []types.GateResult, pool map[string]*types.LexEntry, threshold float64, lensConfidences map[string]float64) string {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := makeFireEventID(in.SessionID, ts, in.Prompt)
	rec := fireRecord{
		Ts:            ts,
		HookEventID:   eventID,
		SessionID:     in.SessionID,
		PromptHash:    sha256Hex(in.Prompt, 16),
		PromptSnippet: snippet(in.Prompt, hookPromptSnippetLen),
		Threshold:     threshold,
		VocabSize:     len(vocab),
		TopResults:    make([]fireResultRecord, 0, len(results)),
	}
	if os.Getenv("LEXICON_CAPTURE_PROMPTS") == "1" {
		rec.PromptFull = in.Prompt
	}
	for _, r := range results {
		name := r.PrimitiveID
		if entry, ok := pool[r.PrimitiveID]; ok {
			name = entry.Name
		}
		rec.TopResults = append(rec.TopResults, fireResultRecord{
			PrimitiveID:    r.PrimitiveID,
			Name:           name,
			Score:          r.Score,
			LensConfidence: lensConfidences[r.PrimitiveID],
		})
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return eventID
	}
	dir := filepath.Join(home, ".claude", "lexicon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return eventID
	}
	path := filepath.Join(dir, "fires.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		hookLog("hook", "fires.jsonl open: %s", err)
		return eventID
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		hookLog("hook", "fires.jsonl marshal: %s", err)
		return eventID
	}
	fmt.Fprintln(f, string(data))
	return eventID
}

func makeFireEventID(sessionID, ts, prompt string) string {
	h := sha256.New()
	h.Write([]byte(sessionID))
	h.Write([]byte(ts))
	h.Write([]byte(prompt))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func sha256Hex(s string, n int) string {
	sum := sha256.Sum256([]byte(s))
	enc := hex.EncodeToString(sum[:])
	if n > 0 && n < len(enc) {
		return enc[:n]
	}
	return enc
}

func snippet(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// isSystemEventPrompt detects Claude Code system events that arrive on
// UserPromptSubmit but are not user input. The hook should silence them
// before any embed/lens work, since they're noise that the gate would
// otherwise score against the user-prompt-tuned prototype corpus.
//
// Current signatures (extend as new system event shapes appear):
//   - <task-notification> ... </task-notification>
//     (background TaskCreate completions; observed firing lex-mjav2 at sim ~0.60)
func isSystemEventPrompt(prompt string) bool {
	t := strings.TrimSpace(prompt)
	return strings.HasPrefix(t, "<task-notification>")
}

// formatHookInjection builds the V13 slimemold-shaped output.
//
// Three sections, any can be empty:
//  1. Mode 1 (informed): Priority match + suggested mention + additional
//     bullets. Fires when lens has high-conf picks.
//  2. Mode 2 (tarot): random N primitives framed as oblique provocations.
//     Fires when lens detected a stuck-signal in the prompt.
//  3. Mode 3 (contradiction placeholder): names the contradiction the
//     lens detected; gestures at TRIZ-style decomposition. Full Mode 3
//     output deferred to TASK 9.
//
// Voicing throughout mirrors slimemold register: conversational,
// modest, soft modals, action-pointing, not upbeat.
func formatHookInjection(results []types.GateResult, pool map[string]*types.LexEntry, lensRes lens.Result) string {
	hasMode1 := len(results) > 0
	hasMode2 := lensRes.StuckSignal
	hasMode3 := lensRes.ContradictionSignal
	if !hasMode1 && !hasMode2 && !hasMode3 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Lexicon elements hit. Surface only if it actually applies. Run `lexicon render <id> --mode introspection` to inspect any pick.\n\n")

	// Mode 1 — informed/compositional
	if hasMode1 {
		top := results[0]
		topEntry, ok := pool[top.PrimitiveID]
		if ok {
			fmt.Fprintf(&b, "Priority match: **%s** (%s, %s → %s, %s, lens conf=%.2f)\n",
				topEntry.Name, topEntry.ID, topEntry.TypeIn, topEntry.TypeOut, topEntry.Tier, top.Score)
			if lensRes.TopSuggestedMention != "" {
				fmt.Fprintf(&b, "Suggested mention: %q\n", lensRes.TopSuggestedMention)
			}
			if len(topEntry.CanonicalInstances) > 0 {
				fmt.Fprintf(&b, "Canonical: %s\n", trimExample(topEntry.CanonicalInstances[0]))
			}
			if len(results) > 1 {
				b.WriteString("\nAdditional matches (only if more relevant than the priority):\n")
				for _, r := range results[1:] {
					if entry, ok := pool[r.PrimitiveID]; ok {
						fmt.Fprintf(&b, "- **%s** (%s, lens conf=%.2f) — %s\n",
							entry.Name, entry.ID, r.Score, briefMechanism(entry))
					}
				}
			}
			// V71: surface predict/intervene steering for the top firing
			// reaction (if any among the matches). The in-loop agent
			// translates it to plain language; end-users don't see the
			// reactant/catalyst framing (feedback: interpret-dont-expose).
			for _, r := range results {
				if re, ok := pool[r.PrimitiveID]; ok && re.Tier == "reaction" {
					b.WriteString(reactionSteering(re, pool))
					break
				}
			}
			b.WriteString(ambiguityInterview(results, pool, resolveAmbiguityGap()))
			b.WriteString("\n")
		}
	}

	// Mode 2 — tarot (stuck-signal triggered)
	if hasMode2 {
		picks := tarotDraw(pool, 3)
		if len(picks) > 0 {
			b.WriteString("Oblique provocation (lens flagged stuck-signal — random draw, treat as Eno cards):\n")
			for _, e := range picks {
				fmt.Fprintf(&b, "- **%s** (%s) — %s\n", e.Name, e.ID, briefMechanism(e))
			}
			// Closing question depends on whether Mode 1 also fired —
			// the comparison line only makes sense if there's a priority
			// match to compare against.
			if hasMode1 {
				b.WriteString("Worth asking: what does the priority match miss that one of these would catch?\n\n")
			} else {
				b.WriteString("Worth asking: which of these, taken literally, opens a path you weren't considering?\n\n")
			}
		}
	}

	// Mode 3 — TRIZ-style contradiction body (V14 TASK C)
	if hasMode3 {
		b.WriteString(mode3Body(lensRes.ContradictionPhrasing, pool))
	}

	return b.String()
}

// contradictionPatterns maps common two-sided contradiction shapes to
// the elements primitives + resolution-pattern hint Mode 3 should
// surface. Each entry needs at least one keyword from sideA AND one
// from sideB present in the lens-extracted contradiction_phrasing
// (lowercased) to fire. First match wins. Primitives that aren't in
// the live pool are skipped silently — keeps the table robust to
// renames/removals without runtime errors.
//
// V14 TASK C scope: 7 common contradictions covering the bulk of
// software/engineering trade-off framings. Generic fallback (parameter-
// decomposition pointer) handles everything not covered.
var contradictionPatterns = []struct {
	name       string
	sideA      []string
	sideB      []string
	primitives []string
	resolution string
}{
	{
		name:       "speed-vs-correctness",
		sideA:      []string{"fast", "quick", "speed", "performant", "real-time", "low-latency", "low latency"},
		sideB:      []string{"accurate", "correct", "exact", "precise", "rigorous", "thorough"},
		primitives: []string{"lex-spk4s"},
		resolution: "Fast-path / slow-path: cheap heuristic for the 95% case, exact computation for the rest. Two paths share an interface; each optimizes for its own cost.",
	},
	{
		name:       "simple-vs-powerful",
		sideA:      []string{"simple", "minimal", "ergonomic", "easy", "approachable", "beginner"},
		sideB:      []string{"powerful", "flexible", "expressive", "feature-rich", "advanced", "extensible", "configurable"},
		primitives: []string{"lex-spk4s", "lex-wdeje"},
		resolution: "Progressive disclosure: defaults stay simple; advanced surfaces appear only when reached for. Asymmetric exposure by user-role / depth-of-engagement.",
	},
	{
		name:       "fresh-vs-stable",
		sideA:      []string{"fresh", "new", "latest", "cutting-edge", "experimental", "innovative", "bleeding-edge"},
		sideB:      []string{"stable", "reliable", "tested", "production", "battle-tested", "safe", "proven"},
		primitives: []string{"lex-spk4s"},
		resolution: "Channel separation: stable + experimental versions side-by-side. Users opt into the experimental edge; production stays on the stable channel.",
	},
	{
		name:       "decoupled-vs-simple",
		sideA:      []string{"decoupled", "modular", "loose", "independent", "separate", "abstracted"},
		sideB:      []string{"simple", "minimal", "few", "single", "unified", "small"},
		primitives: []string{"lex-spk4s"},
		resolution: "Bounded-context: hard invariants AT the boundary, freedom INSIDE. The decoupling seam is the only interface that matters; everything else can stay tightly cohesive.",
	},
	{
		name:       "secure-vs-convenient",
		sideA:      []string{"secure", "safe", "locked", "controlled", "auth", "permission", "guarded"},
		sideB:      []string{"convenient", "frictionless", "smooth", "ergonomic", "easy"},
		primitives: []string{"lex-wdeje"},
		resolution: "Asymmetric friction: friction scales with risk. Low-stakes paths skip the gate; high-stakes paths step up. Just-in-time auth, not blanket-friction.",
	},
	{
		name:       "specific-vs-general",
		sideA:      []string{"specific", "narrow", "focused", "particular", "this case", "this instance"},
		sideB:      []string{"general", "broad", "generic", "universal", "all cases", "every case", "all instances"},
		primitives: []string{"lex-xcaqj"},
		resolution: "Solve the general, instantiate yours: lift to a parameterized family; solve at that level; specialize back. The general solution often falls out cleaner than the special case.",
	},
	{
		name:       "performant-vs-readable",
		sideA:      []string{"performant", "fast", "optimized", "efficient", "tight", "hot path"},
		sideB:      []string{"readable", "maintainable", "clean", "clear", "obvious"},
		primitives: []string{"lex-spk4s"},
		resolution: "Isolate the hot path: clean code everywhere except the measured-hot 3%. That 3% is allowed to be ugly because it's contained, named, and benchmarked.",
	},
}

// mode3Body produces the Mode 3 (contradiction) injection body. It
// matches the lens-extracted contradiction phrasing against the
// contradictionPatterns table; first two-sided match wins. Recommended
// primitives are looked up in the live pool (so renamed/removed
// entries become quiet skips). Falls back to the generic parameter-
// decomposition pointer when nothing matches — preserves the V13
// behavior for unrecognized contradictions.
func mode3Body(phrasing string, pool map[string]*types.LexEntry) string {
	display := phrasing
	if display == "" {
		display = "the prompt articulates opposing requirements"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Contradiction surfaced: %s\n", display)

	lower := strings.ToLower(phrasing)
	for _, p := range contradictionPatterns {
		if !anyContains(lower, p.sideA) || !anyContains(lower, p.sideB) {
			continue
		}
		var picks []*types.LexEntry
		for _, id := range p.primitives {
			if e, ok := pool[id]; ok {
				picks = append(picks, e)
			}
		}
		if len(picks) == 0 {
			continue
		}
		fmt.Fprintf(&b, "Pattern: %s. %s\n", p.name, p.resolution)
		b.WriteString("Relevant primitives:\n")
		for _, e := range picks {
			fmt.Fprintf(&b, "- **%s** (%s)\n", e.Name, e.ID)
		}
		return b.String()
	}

	// Default: gesture at parameter-decomposition without naming a
	// specific resolution pattern.
	b.WriteString("This is the shape parameter-decomposition (lex-mnxhs) and separation-by-aspect address. Worth naming which axis varies before picking a side.\n")
	return b.String()
}

func anyContains(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// tarotDraw returns up to n active entries from the pool, in
// random order. Pure shuffle — no LLM, no filtering by relevance.
// Matches `lexicon shuffle --filter status=active` semantics.
func tarotDraw(pool map[string]*types.LexEntry, n int) []*types.LexEntry {
	candidates := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		if e.Status == "active" {
			candidates = append(candidates, e)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	if n > len(candidates) {
		n = len(candidates)
	}
	return candidates[:n]
}

func trimExample(s string) string {
	if len(s) > hookExampleMaxLen {
		return s[:hookExampleMaxLen-3] + "..."
	}
	return s
}

// briefMechanism extracts a one-line mechanism gloss for an entry —
// used in additional-matches bullets and tarot picks. Prefers the
// first canonical-instance trimmed to ~150 chars.
func briefMechanism(e *types.LexEntry) string {
	if len(e.CanonicalInstances) == 0 {
		return ""
	}
	s := strings.ReplaceAll(e.CanonicalInstances[0], "\n", " ")
	if len(s) > 150 {
		s = s[:147] + "..."
	}
	return s
}

// reactionSteering renders the compact predict/intervene view for a
// firing reaction: where it's heading, what accelerates it (the lever to
// watch), what blocks it (the intervention point). Emitted into the hook
// injection so the in-loop agent can translate it into plain language —
// end-users shouldn't see the reactant/catalyst chemistry framing (V71;
// feedback memory interpret-dont-expose-chemistry). joinSlots/truncateGreedy
// are shared from cmd_what_if.go (same package).
func reactionSteering(e *types.LexEntry, pool map[string]*types.LexEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nReaction steering — if **%s** (%s) is what's firing, interpret it for the user in plain language (don't expose the reactant/catalyst framing):\n", e.Name, e.ID)
	if len(e.Products) > 0 {
		fmt.Fprintf(&b, "- heading toward: %s\n", truncateGreedy(joinSlots(e.Products, pool), 200))
	}
	if len(e.Catalysts) > 0 {
		fmt.Fprintf(&b, "- accelerated by (the lever to watch): %s\n", truncateGreedy(joinSlots(e.Catalysts, pool), 200))
	}
	if len(e.Inhibitors) > 0 {
		fmt.Fprintf(&b, "- blocked by (the intervention): %s\n", truncateGreedy(joinSlots(e.Inhibitors, pool), 200))
	}
	return b.String()
}

func emitHookResponse(context string) {
	resp := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "UserPromptSubmit",
			"additionalContext": context,
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		hookLog("hook", "marshal response: %s", err)
		return
	}
	fmt.Println(string(data))
}

func topScore(rs []types.GateResult) float64 {
	if len(rs) == 0 {
		return 0
	}
	return rs[0].Score
}

func hookLog(name, format string, args ...any) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".claude", "lexicon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	path := filepath.Join(dir, "hook.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s [%s] %s\n",
		time.Now().UTC().Format(time.RFC3339),
		name,
		fmt.Sprintf(format, args...),
	)
}
