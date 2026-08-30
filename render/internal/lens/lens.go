// Package lens is the LLM-backed semantic-relevance filter that sits
// BETWEEN the elements-load step and the deterministic gate (per
// hybrid-cross-pollination.md's lens → elements → gate → reasoner →
// action pipeline). Until V12 the lens was bypassed and the gate did
// pure lexical scoring on the entire pool, which produced ~95%
// false-positive rate in production (see fires-jsonl-analysis-pass-1.md).
//
// V12 lens implementation: builds a compact one-line index of the
// elements, asks a fast model (Haiku 4.5) which entries are
// SEMANTICALLY relevant to the prompt, returns the filtered subset for
// the gate to score. Fail-soft: on timeout or error, returns the full
// pool so the gate sees what it would have seen in the pre-V12 world.
package lens

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// DefaultTimeout is how long the lens waits for the LLM before
// falling back to the full pool. Hooks must not block prompt
// submission for too long; 8s is the practical upper bound (Haiku
// 4.5 typically returns in 1-3s on a ~16KB elements index, but
// tail-latency can hit 5-7s under load). Override with
// LEXICON_LENS_TIMEOUT_MS=<int>.
const DefaultTimeout = 8 * time.Second

// MaxCandidates caps the LLM's output. Even if the pool has 100+
// entries, the gate only re-ranks a handful; asking for too many
// dilutes the lens's filtering value.
const MaxCandidates = 8

// systemPrompt instructs the lens. V13 expanded the output to enable
// auto-triggered modes (informed, tarot, contradiction): one LLM call
// produces picks + per-pick confidence + per-top-pick suggested_mention
// + stuck_signal + contradiction_signal. The hook dispatches modes
// based on those signals so the user gets ambient surfacing without
// invoking any subcommand (mirrors slimemold's auto-detection pattern).
//
// The parser is tolerant of legacy shapes (V12 string-array; V13
// {id,confidence}-only) so older replay scripts still work.
const systemPrompt = `You read a user prompt against a set of cognitive primitives. Produce ONE JSON object:

  picks:                  array of {id, confidence, suggested_mention?} — most-relevant primitives ranked. At most 8. Empty if nothing fits.
  stuck_signal:           true if the prompt shows the user is stuck/brainstorming/lost and wants lateral provocation rather than crisp classification.
  contradiction_signal:   true if the prompt articulates opposing requirements ("X must be both A and not-A", "tradeoff between X and Y").
  contradiction_phrasing: when contradiction_signal is true, the user's actual phrasing (one sentence, paraphrased lightly).

Elements index lines: id | name | type-in -> type-out | tier | brief.

picks rules:
- Confidence (0.0-1.0) is your estimate that the primitive's mechanism actually applies. 0.8+ only when the mechanism clearly fits; 0.6-0.8 plausible-but-uncertain; below 0.5 just exclude.
- Lexical token overlap is NOT relevance. If a primitive matches by word but not by mechanism, exclude it.
- suggested_mention: ONLY on the top pick (rank 0). ONE SENTENCE, max ~25 words. Claude can say it verbatim. Specific to BOTH the primitive's mechanism AND the user's actual prompt content.

VOICING for suggested_mention — mirror slimemold's register:
- Conversational, modest, soft modals (looks like / might / seems). Not peppy ("Great point!"), not assertive ("You should X"), not declarative ("X is the answer").
- End in a short question or hook, not a declaration ("worth flagging?", "want to lean into that?").
- Form: "<opener> — this <looks like/has the shape of> <name> <mechanism gloss tied to user's prompt>. <soft action question>?"
- Good: "Worth flagging — this looks like contradiction-resolution-via-parameter-decomposition: the 'thin AND thick wall' shape. Want to walk through which axis to vary?"

stuck_signal — DEFAULT IS FALSE. Only true when the prompt EXPLICITLY asks for lateral input or signals a perception-block. The bar is high; over-firing pollutes output with unwanted provocations.
  true: "I'm stuck on...", "what am I missing?", "let me brainstorm", "I don't see how", "any ideas?", "feels blocked"
  FALSE (despite surface similarity):
  - "keep going", "continue", "yes do that" (forward-momentum directives)
  - "they keep pushing back", "build keeps failing" (situational friction, not cognitive stuckness)
  - "this is hard", "ugh", "frustrating" (acknowledging difficulty)
  - "let me think", "give me a sec" (active processing)
  - "implement X", "fix this bug", "what's next?" (concrete task / sequence asks)
  - "do we want X? maybe?", "should we go with Y or Z?", "thinking through whether to..." (hedged weighing of options — actively reasoning, NOT stuck)
  - questions with a clear answerable shape, even if hard

contradiction_signal:
  true: "must be fast but also accurate", "tension between X and Y", "wall has to be thin AND thick"
  false: single-requirement asks, sequential steps, classification questions

Output ONLY the JSON object. No commentary.

Example: {"picks":[{"id":"lex-mnxhs","confidence":0.85,"suggested_mention":"This has the shape of contradiction-resolution-via-parameter-decomposition — varying which property dominates per region. Want to walk that decomposition?"},{"id":"lex-bpr6b","confidence":0.6}],"stuck_signal":false,"contradiction_signal":true,"contradiction_phrasing":"wall must be thin (thermal) and thick (strength)"}`

// Usage is the V13 prompt-cache accounting returned alongside the
// filter result. Zero-valued when the lens didn't run.
type Usage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
}

// Result bundles everything the lens produces in its single LLM call:
// the filtered+ranked entries, per-ID confidence, the top pick's
// pre-baked surfacing sentence, the auto-trigger signals (stuck /
// contradiction), and prompt-cache usage. Fields are zero-valued when
// the lens didn't run or didn't surface them.
type Result struct {
	Entries               []*types.LexEntry
	Confidences           map[string]float64
	TopSuggestedMention   string
	StuckSignal           bool
	ContradictionSignal   bool
	ContradictionPhrasing string
	Usage                 Usage
}

// Filter asks the LLM which entries are semantically relevant + which
// auto-trigger modes apply (stuck, contradiction). On any error or
// timeout, returns a Result with Entries=pool, zero signals, err set —
// caller decides whether to proceed with degraded (lexical-only) gating.
//
// disabled short-circuits (Entries=pool, no signals, no usage, no err).
// Use when ANTHROPIC_API_KEY isn't set or for explicit opt-out.
//
// When the lens returns an empty picks array, Filter returns
// Entries=nil, signals as parsed, no err — the hook caller treats nil
// Entries (post-lens-active) as "no Mode 1 fire" but may still emit
// Mode 2/3 if their signals fired.
func Filter(ctx context.Context, prompt string, pool []*types.LexEntry, c client.Client, disabled bool) (Result, error) {
	if disabled || c == nil {
		return Result{Entries: pool}, nil
	}
	if len(pool) == 0 {
		return Result{Entries: pool}, nil
	}

	index := buildIndex(pool)
	// V13: elements index moves into CachedSystem so it hits the 5min
	// ephemeral prompt cache. The user message is now JUST the prompt —
	// stable cache key across calls in a session. First call in a
	// session writes the cache (~1.25x input cost); subsequent calls
	// read the cache (~10% input cost). Net: ~4x cost reduction at
	// typical 50-calls-per-session burstiness.
	cachedSystem := "Elements (cached):\n" + index
	userText := "Prompt:\n" + prompt

	timeout := DefaultTimeout
	if v := os.Getenv("LEXICON_LENS_TIMEOUT_MS"); v != "" {
		if ms, err := time.ParseDuration(v + "ms"); err == nil && ms > 0 {
			timeout = ms
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := c.CreateMessage(ctx, client.MessageRequest{
		System:       systemPrompt,
		CachedSystem: cachedSystem,
		UserText:     userText,
		MaxTokens:    600, // V13: object shape with picks + signals + suggested_mention; ~250-400 tokens typical, 600 is cushion
		Model:        client.LensModel,
	})
	out := Result{
		Usage: Usage{
			InputTokens:         resp.InputTokens,
			OutputTokens:        resp.OutputTokens,
			CacheReadTokens:     resp.CacheReadTokens,
			CacheCreationTokens: resp.CacheCreationTokens,
		},
	}
	if err != nil {
		out.Entries = pool
		return out, fmt.Errorf("lens: llm: %w", err)
	}

	parsed, err := parseLensResponse(resp.Text)
	if err != nil {
		out.Entries = pool
		return out, fmt.Errorf("lens: parse: %w", err)
	}

	out.Confidences = parsed.confidences
	out.TopSuggestedMention = parsed.topSuggestedMention
	out.StuckSignal = parsed.stuckSignal
	out.ContradictionSignal = parsed.contradictionSignal
	out.ContradictionPhrasing = parsed.contradictionPhrasing

	// Empty picks = "nothing relevant" for Mode 1. Caller still sees
	// the StuckSignal / ContradictionSignal so Mode 2/3 can fire on
	// their own merit even when no informed pick exists.
	if len(parsed.ids) == 0 {
		return out, nil
	}

	// Map IDs back to entries in lens-returned order.
	byID := make(map[string]*types.LexEntry, len(pool))
	for _, e := range pool {
		byID[e.ID] = e
	}
	for _, id := range parsed.ids {
		if e, ok := byID[id]; ok {
			out.Entries = append(out.Entries, e)
		}
	}
	return out, nil
}

// indexable reports whether an entry gets an index line at all (gate
// would drop sub-atomic/deprecated entries anyway, so buildIndex and
// ChunkPool both skip them up front rather than paying for a line
// neither will use).
func indexable(e *types.LexEntry) bool {
	return e.Tier != "sub-atomic" && e.Status != "deprecated"
}

// indexLine renders one entry's index row: fields delimited by " | ".
// Brief is the first canonical-instance trimmed to ~90 chars — enough
// for the LLM to distinguish similar atoms by their concrete example.
// Shared by buildIndex and ChunkPool's size accounting so the two can
// never silently drift out of sync with each other.
func indexLine(e *types.LexEntry) string {
	brief := ""
	if len(e.CanonicalInstances) > 0 {
		brief = trimBrief(e.CanonicalInstances[0], 90)
	}
	return fmt.Sprintf("%s | %s | %s -> %s | %s | %s\n",
		e.ID, e.Name, e.TypeIn, e.TypeOut, e.Tier, brief)
}

// sortedIndexable returns the subset of pool that gets an index line,
// sorted by ID. V13 TASK 4: sorting makes the index byte-identical
// across invocations — critical for prompt-cache hits, since Go map
// iteration is non-deterministic and callers passing entries in
// pool-order would otherwise produce a different cache-key prefix
// every call. ChunkPool reuses this ordering so each chunk's cache
// prefix is likewise stable call-to-call within a session.
func sortedIndexable(pool []*types.LexEntry) []*types.LexEntry {
	sorted := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		if indexable(e) {
			sorted = append(sorted, e)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	return sorted
}

// buildIndex flattens the pool to a single newline-separated text block,
// one line per entry (see indexLine), sorted by ID (see sortedIndexable).
func buildIndex(pool []*types.LexEntry) string {
	var b strings.Builder
	for _, e := range sortedIndexable(pool) {
		b.WriteString(indexLine(e))
	}
	return b.String()
}

// charsPerTokenEstimate is a deliberately conservative (i.e. token-
// overestimating) chars-per-token ratio, used only to size chunks
// safely below a model's context ceiling with no local tokenizer
// available. Calibrated against the real failure this exists to
// prevent: a 3512-atom buildIndex output was rejected by the API at
// "200151 tokens > 200000 maximum" -- an index that size runs ~3.4
// chars/token, so 3.5 rounds toward MORE estimated tokens (smaller,
// safer chunks) rather than fewer.
const charsPerTokenEstimate = 3.5

// EstimateTokens gives a conservative (over-)estimate of how many
// tokens s will cost a Claude model. Not exact -- there is no local
// tokenizer here -- but exact isn't the goal; staying safely under a
// context ceiling is, and overestimating errs the right direction.
func EstimateTokens(s string) int {
	return int(float64(len(s))/charsPerTokenEstimate) + 1
}

// ChunkPool splits pool into groups whose combined buildIndex text
// each stays under an estimated maxTokens. Chunk COUNT is not fixed --
// it grows with the pool -- which is the entire point: this exists so
// a single-call full-pool index can never again silently overflow a
// model's context window as the corpus grows past whatever size it was
// last measured at. Order is stable (sortedIndexable, ID-ascending) so
// repeated calls in one mining session produce the same chunk
// boundaries and each chunk's prompt-cache prefix stays stable.
//
// An atom whose own line somehow exceeds maxTokens still gets its own
// single-atom chunk rather than being dropped -- oversized-but-present
// beats silently excluded from ever being seen by the lens.
func ChunkPool(pool []*types.LexEntry, maxTokens int) [][]*types.LexEntry {
	sorted := sortedIndexable(pool)
	if len(sorted) == 0 {
		return nil
	}
	var chunks [][]*types.LexEntry
	var cur []*types.LexEntry
	curTokens := 0
	for _, e := range sorted {
		lineTokens := EstimateTokens(indexLine(e))
		if len(cur) > 0 && curTokens+lineTokens > maxTokens {
			chunks = append(chunks, cur)
			cur = nil
			curTokens = 0
		}
		cur = append(cur, e)
		curTokens += lineTokens
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

func trimBrief(s string, n int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// parsedLens is the internal scratch shape for one parsed lens
// response. parseLensResponse fills it from the model's JSON.
type parsedLens struct {
	ids                   []string
	confidences           map[string]float64
	topSuggestedMention   string
	stuckSignal           bool
	contradictionSignal   bool
	contradictionPhrasing string
}

// parseLensResponse extracts the V13 object-shaped lens output. Falls
// back to V13a array shape (just picks, no signals) and V12 string
// array (just IDs) for safety — older deploys / model regressions
// still produce something usable for Mode 1, just no auto-trigger
// data for Modes 2/3.
//
// Tolerant of stray prose: locates the first '{' or '[' and uses
// findMatchingClose to find ITS matching closer (bracket-depth-tracked,
// string-literal-aware), not just the last '}'/']' in the whole text —
// a naive last-index would mis-slice if the model's wrapping prose
// happens to contain any other brace/bracket character. On a parse
// failure of that span, retries once with trailing commas stripped
// (a common small LLM JSON malformation) before giving up.
func parseLensResponse(text string) (parsedLens, error) {
	out := parsedLens{}
	objStart := strings.Index(text, "{")
	arrStart := strings.Index(text, "[")
	// Prefer object form (V13) if it appears before any array.
	if objStart >= 0 && (arrStart < 0 || objStart < arrStart) {
		end, ferr := findMatchingClose(text, objStart, '{', '}')
		if ferr != nil {
			return out, fmt.Errorf("malformed JSON object in response: %w: %q", ferr, text)
		}
		jsonText := text[objStart : end+1]
		type lensItem struct {
			ID               string  `json:"id"`
			Confidence       float64 `json:"confidence"`
			SuggestedMention string  `json:"suggested_mention"`
		}
		var doc struct {
			Picks                 []lensItem `json:"picks"`
			StuckSignal           bool       `json:"stuck_signal"`
			ContradictionSignal   bool       `json:"contradiction_signal"`
			ContradictionPhrasing string     `json:"contradiction_phrasing"`
		}
		if err := unmarshalWithRepair(jsonText, &doc); err != nil {
			return out, fmt.Errorf("unmarshal %q: %w", jsonText, err)
		}
		out.confidences = make(map[string]float64, len(doc.Picks))
		for i, it := range doc.Picks {
			if it.ID == "" {
				continue
			}
			out.ids = append(out.ids, it.ID)
			out.confidences[it.ID] = it.Confidence
			if i == 0 {
				out.topSuggestedMention = it.SuggestedMention
			}
		}
		if len(out.ids) > MaxCandidates {
			out.ids = out.ids[:MaxCandidates]
		}
		out.stuckSignal = doc.StuckSignal
		out.contradictionSignal = doc.ContradictionSignal
		out.contradictionPhrasing = doc.ContradictionPhrasing
		return out, nil
	}
	// Fallback: V13a/V12 array form (just picks, no signals).
	if arrStart < 0 {
		return out, fmt.Errorf("no JSON in response: %q", text)
	}
	end, ferr := findMatchingClose(text, arrStart, '[', ']')
	if ferr != nil {
		return out, fmt.Errorf("malformed JSON array in response: %w: %q", ferr, text)
	}
	jsonText := text[arrStart : end+1]
	type lensItemArr struct {
		ID         string  `json:"id"`
		Confidence float64 `json:"confidence"`
	}
	var items []lensItemArr
	if err := unmarshalWithRepair(jsonText, &items); err == nil && (len(items) == 0 || items[0].ID != "") {
		out.confidences = make(map[string]float64, len(items))
		for _, it := range items {
			if it.ID == "" {
				continue
			}
			out.ids = append(out.ids, it.ID)
			out.confidences[it.ID] = it.Confidence
		}
		if len(out.ids) > MaxCandidates {
			out.ids = out.ids[:MaxCandidates]
		}
		return out, nil
	}
	// Final fallback: V12 string array.
	var stringIDs []string
	if err := unmarshalWithRepair(jsonText, &stringIDs); err != nil {
		return out, fmt.Errorf("unmarshal %q: %w", jsonText, err)
	}
	out.ids = stringIDs
	if len(out.ids) > MaxCandidates {
		out.ids = out.ids[:MaxCandidates]
	}
	return out, nil
}

// findMatchingClose scans text starting at the index of an opening
// bracket (open) for its matching closing bracket (close), tracking
// nesting depth and skipping over content inside double-quoted string
// literals (respecting backslash escapes) so braces/brackets that
// appear inside a string value don't perturb the count. Returns the
// index of the matching closer, or an error if the text ends before
// depth returns to zero (truncated/unterminated JSON).
func findMatchingClose(text string, start int, open, close byte) (int, error) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		c := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("no matching %q for %q at byte %d (likely truncated)", string(close), string(open), start)
}

// trailingCommaRe matches a comma followed by only whitespace before a
// closing brace or bracket — the single most common small malformation
// in LLM-generated JSON (a dangling comma after the last array/object
// element).
var trailingCommaRe = regexp.MustCompile(`,(\s*[}\]])`)

// unmarshalWithRepair tries json.Unmarshal as-is first (the common
// case, zero overhead); on failure, retries once with trailing commas
// stripped. Returns the ORIGINAL unmarshal error if the repaired text
// still doesn't parse, since that error is more informative about the
// real malformation than a second, repair-confused one would be.
func unmarshalWithRepair(jsonText string, v interface{}) error {
	firstErr := json.Unmarshal([]byte(jsonText), v)
	if firstErr == nil {
		return nil
	}
	repaired := trailingCommaRe.ReplaceAllString(jsonText, "$1")
	if repaired == jsonText {
		return firstErr // nothing to repair, don't retry an identical parse
	}
	if err := json.Unmarshal([]byte(repaired), v); err == nil {
		return nil
	}
	return firstErr
}

// Disabled returns true iff the lens should be skipped (env var or
// missing API key). The hook calls this BEFORE constructing a client,
// so a missing key doesn't error-spam the log on every prompt.
func Disabled() bool {
	if os.Getenv("LEXICON_LENS_DISABLED") == "1" {
		return true
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return true
	}
	return false
}
