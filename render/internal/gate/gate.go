// Package gate is the deterministic filter+rank step between elements
// and reasoner (per hybrid's lens/elements/gate/reasoner/action loop).
// No LLM calls live here. v0 uses heuristics; v1+ may add learned
// ranking once there's enough deployment data to justify.
//
// The load-bearing call this package makes: tier-base scoring is
// CONTEXT-DEPENDENT. In deployment contexts (mid-bind, stuck-on,
// trying-to, or no-context-given) molecules outrank atoms because the
// user wants the named-pattern they can deploy now. In design contexts
// (design, explain, decompose) atoms outrank molecules because the
// user is inspecting the elements' structure. Invariant: "user
// mid-bind on expert claims" must rank lex-kebfa first; "design
// conversation about decomposition" must rank atoms first.
package gate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/framestatus"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// ContextStance is the gate's classification of "what kind of help
// does the user want right now." Either deployment-mode (apply a
// pattern) or design-mode (inspect the elements).
type ContextStance string

const (
	StanceDeployment ContextStance = "deployment"
	StanceDesign     ContextStance = "design"
)

// DefaultTopK is the cap when the caller doesn't specify. Per Q4 +
// hybrid GATE spec: top-3 typical for design conversations, top-1 for
// deployment-mode renders.
const DefaultTopK = 3

// designCues / deploymentCues are substring matches against the
// lowercased context string. Design takes precedence: if both kinds
// of cues appear, the user is in a design conversation about a
// deployment situation, and the elements' structure is what they
// need.
var (
	designCues = []string{
		"design", "explain", "decompose", "decomposition",
		"architect", "structure", "explore", "review",
	}
	deploymentCues = []string{
		"mid-bind", "stuck on", "trying to",
		"in the moment", "right now", "help me",
	}
)

// tierBaseDeployment / tierBaseDesign are the per-tier base scores
// before status multiplier and vocab-match boost. The asymmetry between
// the two tables IS the resolution of the original v0 smoke-test
// finding (atoms-outrank-molecules-by-default was operationally wrong
// for deployment contexts).
var (
	tierBaseDeployment = map[string]float64{
		"sub-atomic": 0.2,
		"atomic":     0.7,
		"molecule":   1.0,
		"compound":   0.9,
	}
	tierBaseDesign = map[string]float64{
		"sub-atomic": 0.2,
		"atomic":     1.0,
		"molecule":   0.7,
		"compound":   0.6,
	}
	statusMult = map[string]float64{
		"active":       1.0,
		"under-review": 0.7,
		"deprecated":   0.0,
	}
)

// confidenceFloor is the weight applied at confidence=0.0 — deliberately
// nonzero so a single bad lens judgment can't fully zero out an otherwise
// tier-appropriate atom. This is an empirically-calibrated constant (like
// tierBase/vocab-boost above), not an analytically-derived one: a naive
// floor of 0.5 was checked against the motivating car-wash case and found
// too weak to move rankings, since lens.go's own prompt already excludes
// anything below ~0.5 confidence from ever reaching the gate, collapsing
// the achievable weight spread. 0.25 leaves real headroom. Retune against
// replayed fires.jsonl confidence values once real traffic is observed.
const confidenceFloor = 0.25

// constitutiveDownweight applies when an atom is classified constitutive
// (Principle 10 — an offered lens, not a finding) in the oracle-risk
// register. 0.85 sits between statusMult's under-review (0.7) and active
// (1.0), consistent with this file's existing calibration scale. This is
// a down-weight, not an exclusion — a strongly-relevant constitutive atom
// can still win.
const constitutiveDownweight = 0.85

// confidenceWeight maps a lens confidence in [0,1] to a score multiplier
// in [confidenceFloor, 1.0], linear between the two endpoints. Values
// outside [0,1] are clamped first — lens.parseLensResponse does not
// currently validate/clamp raw model JSON, so a malformed or hallucinated
// confidence value must not produce an out-of-range multiplier here.
func confidenceWeight(conf float64) float64 {
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}
	return confidenceFloor + (1-confidenceFloor)*conf
}

// Input bundles the gate's parameters. Pool is the LENS output (LLM
// match against elements); v0 lets the caller stuff all entries in
// and lets the gate filter.
type Input struct {
	Pool         []*types.LexEntry
	Context      string
	WorkingVocab []string
	TopK         int

	// Confidences is optional per-ID lens confidence (0.0-1.0): the LLM's
	// judgment that the primitive's MECHANISM applies (lens.go's "lexical
	// overlap is NOT relevance" instruction), not that it merely surfaced.
	// nil, or an ID absent from the map, is a no-op (weight 1.0) — this is
	// REQUIRED for byte-identical backward compatibility with every caller
	// that never ran the lens. Values outside [0,1] are clamped.
	Confidences map[string]float64

	// FrameStatus is optional per-ID classification from the oracle-risk
	// register (internal/framestatus, elements-design Principle 10). Only
	// framestatus.Constitutive triggers a discount ("offered lens, never a
	// finding" — a merely-interpretive atom shouldn't casually outrank a
	// checkable/corrective one of similar score). Navigational, Mixed, and
	// unclassified atoms are no-ops. This is a DIFFERENT axis than
	// types.LexEntry.Status (active/under-review/deprecated lifecycle) —
	// don't conflate the two.
	FrameStatus framestatus.Map
}

// ClassifyContext picks deployment vs design from the (optionally
// empty) context string. Empty string → deployment (the most-common
// case for a mid-conversation "what move applies here" query).
func ClassifyContext(context string) ContextStance {
	if context == "" {
		return StanceDeployment
	}
	lower := strings.ToLower(context)
	for _, cue := range designCues {
		if strings.Contains(lower, cue) {
			return StanceDesign
		}
	}
	for _, cue := range deploymentCues {
		if strings.Contains(lower, cue) {
			return StanceDeployment
		}
	}
	return StanceDeployment
}

// Run filters the pool (drops sub-atomic and deprecated), scores each
// remaining entry, sorts descending, and returns the top-k.
func Run(in Input) []types.GateResult {
	topK := in.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}
	stance := ClassifyContext(in.Context)
	results := make([]types.GateResult, 0, len(in.Pool))
	for _, entry := range in.Pool {
		if entry.Tier == "sub-atomic" || entry.Status == "deprecated" {
			continue
		}
		score, reason := scoreEntry(entry, stance, in.WorkingVocab, in.Confidences, in.FrameStatus)
		results = append(results, types.GateResult{
			PrimitiveID: entry.ID,
			ModeHint:    pickModeHint(entry, in.Context, stance),
			Score:       score,
			Reason:      reason,
		})
	}
	// Descending sort by score, ties broken by PrimitiveID ascending.
	// This is a TOTAL order: the result is independent of input order, so
	// callers passing a Go-map-iterated pool (random order per run) still
	// get identical output run-to-run. Tier×status scoring produces large
	// tie groups, so without an ID tiebreak the map's random iteration
	// order leaked straight into the ranking (observed: byte-identical
	// input → different top-K every run; reported by costean 2026-06-30).
	sortRankDesc(results)
	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

func scoreEntry(
	e *types.LexEntry,
	stance ContextStance,
	vocab []string,
	confidences map[string]float64,
	frameStatus framestatus.Map,
) (float64, string) {
	tierTable := tierBaseDeployment
	if stance == StanceDesign {
		tierTable = tierBaseDesign
	}
	score := tierTable[e.Tier]
	if score == 0 {
		score = 0.5 // unknown tier — give it a chance, log via reason
	}
	mult, ok := statusMult[e.Status]
	if !ok {
		mult = 0.5
	}
	score *= mult
	reasons := []string{
		"tier:" + e.Tier,
		"status:" + e.Status,
		"stance:" + string(stance),
	}
	if len(vocab) > 0 {
		vocabSet := map[string]bool{}
		for _, w := range vocab {
			vocabSet[strings.ToLower(w)] = true
		}
		// Token-aware matching: lexicon entry names are typically
		// hyphenated multi-word handles ("argument-from-expert-opinion",
		// "decoupling-via-indirection"), while prompt vocab is usually
		// single words ("expert", "decoupling"). Split on `-` and check
		// for any-token overlap. Old exact-match behavior is subsumed:
		// an exact-match prompt-word against a single-word entry name
		// still matches as a token.
		// Boost magnitudes calibrated so a name-token vocab match on an
		// atom (0.49 deployment under-review base) beats an unmatched
		// molecule (0.70): 0.49 × 1.6 = 0.784. Strong vocab signal
		// genuinely overrides the tier asymmetry, which matters for the
		// hook-inject use case where the user's prompt is the relevance
		// signal. Evokes-token match is weaker (more entries trip it),
		// so scaled to NOT override tier — molecule unmatched (0.70) >
		// atom with evokes-token match (0.49 × 1.2 = 0.588).
		if entryHasVocabToken(e.Name, vocabSet) {
			score *= 1.6
			reasons = append(reasons, "name-token-in-vocab")
		} else {
			for _, ev := range e.Evokes {
				if entryHasVocabToken(ev, vocabSet) {
					score *= 1.2
					reasons = append(reasons, "evokes-token-in-vocab")
					break
				}
			}
		}
	}
	if conf, ok := confidences[e.ID]; ok {
		w := confidenceWeight(conf)
		score *= w
		reasons = append(reasons, fmt.Sprintf("lens-confidence:%.2f(w=%.2f)", conf, w))
	}
	if fs, ok := frameStatus.Lookup(e.ID); ok && fs.Status == framestatus.Constitutive {
		score *= constitutiveDownweight
		reasons = append(reasons, "constitutive-downweight")
	}
	return score, strings.Join(reasons, ",")
}

// commonTokens are length-4+ English tokens that appear inside lexicon
// entry handles (especially in evokes lists like "the-tribe-defends-the-
// claim") AND in casual prompts. Without filtering, these produce
// false-positive name/evokes matches. List derived empirically from V12
// fires.jsonl analysis (fires-jsonl-analysis-pass-1.md): tokens that
// drove the most false fires in production. Conservative — does NOT
// include domain-loaded tokens like "state", "model", "stack",
// "system", "data" even when noisy, because they carry semantic content
// for the relevant atoms.
var commonTokens = map[string]bool{
	// determiners / pronouns / connectives (length 4+)
	"that": true, "this": true, "these": true, "those": true,
	"they": true, "them": true, "then": true, "than": true,
	"with": true, "from": true, "into": true, "onto": true,
	"here": true, "there": true, "where": true, "when": true,
	"what": true, "which": true, "like": true, "such": true,
	// generic verbs that mean little on their own
	"make": true, "made": true, "take": true, "taken": true,
	"give": true, "given": true, "come": true, "came": true,
	"goes": true, "went": true, "does": true, "done": true,
	"look": true, "seem": true, "feel": true, "felt": true,
	"keep": true, "kept": true, "find": true, "found": true,
	"show": true, "shown": true, "tell": true, "told": true,
	"said": true, "says": true, "gets": true, "know": true,
	"knew": true, "want": true, "need": true, "tried": true,
	"used": true, "uses": true, "have": true, "having": true,
	"wanted": true, "wants": true,
	// sequence / direction
	"first": true, "second": true, "third": true,
	"next": true, "last": true, "back": true, "forward": true,
	"again": true, "also": true, "before": true, "after": true,
	// quantity / quality
	"more": true, "less": true, "much": true, "many": true,
	"some": true, "most": true, "well": true, "even": true,
	"down": true, "over": true, "under": true,
	"right": true, "left": true,
	"better": true, "worse": true, "good": true,
	// generic nouns
	"things": true, "thing": true,
	"version": true, "versions": true,
	"group": true, "groups": true,
	"point": true, "part": true,
	"item": true, "items": true,
	"changed": true, "changes": true,
	// connectives / directionals missed in first pass (V12 replay surfaced)
	"through": true, "throughout": true,
	"backwards": true, "forwards": true,
	// number-names (e.g. "cialdini-three-lever" tokenizes "three";
	// false-positive on "three.min.js" prompts). Numbers as count
	// modifiers in entry handles are never the load-bearing token.
	"two": true, "three": true, "four": true, "five": true,
	"six": true, "seven": true, "eight": true, "nine": true,
	"ten": true,
}

// entryHasVocabToken returns true iff any hyphen-separated token of
// the entry handle (length ≥ 4, not in commonTokens stopword set)
// appears in vocabSet.
//
// V12: length guard was 3, raised to 4 after fires.jsonl analysis
// found 22 false-positive matches on the length-3 token "the" alone
// (matched inside evokes phrases like "the-tribe-defends-the-claim").
// commonTokens stopword set added at same time for length-4+ tokens
// that drive noise (look, first, things, drop, etc).
func entryHasVocabToken(handle string, vocabSet map[string]bool) bool {
	for _, tok := range strings.Split(strings.ToLower(handle), "-") {
		if len(tok) >= 4 && !commonTokens[tok] && vocabSet[tok] {
			return true
		}
	}
	return false
}

func pickModeHint(e *types.LexEntry, context string, stance ContextStance) types.RenderMode {
	lower := strings.ToLower(context)
	if strings.Contains(lower, "debug") || strings.Contains(lower, "introspect") {
		return types.ModeIntrospection
	}
	if stance == StanceDesign {
		return types.ModeMetaExplanatory
	}
	if e.Tier == "molecule" || e.Tier == "compound" {
		return types.ModeNarrative
	}
	return types.ModeMetaExplanatory
}

// sortRankDesc sorts in-place by Score descending, breaking ties on
// PrimitiveID ascending. The ID tiebreak makes the order a total function
// of the result set alone — NOT of input/iteration order — so the same
// pool ranks identically every run regardless of how it was enumerated
// (e.g. Go map iteration, which is randomized). N here is the full
// candidate pool on the no-lens path (hundreds of atoms), so sort.Slice's
// O(n log n) is the right call over the previous O(n²) insertion sort.
func sortRankDesc(rs []types.GateResult) {
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].Score != rs[j].Score {
			return rs[i].Score > rs[j].Score
		}
		return rs[i].PrimitiveID < rs[j].PrimitiveID
	})
}

// FormatResult is a small convenience for CLI tab-output.
func FormatResult(r types.GateResult) string {
	return fmt.Sprintf("%s\t%s\t%.3f\t%s", r.PrimitiveID, r.ModeHint, r.Score, r.Reason)
}
