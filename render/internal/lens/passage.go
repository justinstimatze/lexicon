package lens

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// FilterPassage is the document-trace variant of Filter. Filter was built
// for the hook: a short user prompt, a sub-second latency budget, an index
// line of 90 characters per atom, and an output shape that carries
// hook-only signals (stuck, contradiction, a voiced suggested_mention).
// Reusing it on a 300-word paragraph of published prose asked the wrong
// question in the wrong register and gave the model too little per atom
// to decide with. This call:
//
//   - frames the input as a passage from a named text, not a user prompt;
//   - gives each candidate its mechanism statement (the opening of its
//     agent-instruction), not a 90-char example;
//   - asks for a verbatim evidence span per pick, so the frontend can
//     highlight the words that carry the match rather than the whole
//     paragraph, and so a pick that cannot point at its own evidence is
//     visibly weaker;
//   - allows — and expects — an empty answer. Most exposition instantiates
//     nothing, and the honest trace of a document has gaps.
//
// There is no lexical path here. The caller ranks by Confidence only.

// PassageMeta names the text a passage comes from. It goes into the
// prompt so the model reads the passage as prose with an author, which
// matters for rhetorical patterns (irony in Swift reads differently from
// the same sentence in a memo).
type PassageMeta struct {
	Title  string
	Author string
	Year   int
}

// PassagePick is one pattern the lens judged present in the passage.
type PassagePick struct {
	ID         string
	Confidence float64
	// Evidence is the model's verbatim quote of the words carrying the
	// mechanism. LocateEvidence maps it back to offsets in the passage;
	// a pick whose evidence cannot be found is still returned, with the
	// text and no offsets, so the caller can decide how much to trust it.
	Evidence string
	// Why is one sentence on how this passage does what the pattern names.
	Why string
}

// PassageResult is FilterPassage's output. Picks are sorted by Confidence
// descending; Raw carries the model's text for diagnostics.
type PassageResult struct {
	Picks []PassagePick
	Usage Usage
	Raw   string
}

// DefaultPassageTimeout is the per-call ceiling for FilterPassage. This
// is a batch precompute, not a hook — a slow answer costs nothing and a
// timed-out one costs the whole passage. Override with
// LEXICON_LENS_TIMEOUT_MS as with Filter.
const DefaultPassageTimeout = 90 * time.Second

// MaxPassagePicks caps how many picks a single passage can carry. Three is
// already generous for one paragraph; the point of the cap is to stop a
// model from listing everything that is merely nearby.
const MaxPassagePicks = 3

const passageSystemPrompt = `You read a passage from a published text against a small catalog of named patterns — recurring moves in reasoning, rhetoric, strategy, and social mechanics. Your job is to say which patterns the passage genuinely instantiates: the author performs the move, argues for it, or describes a situation whose working mechanism is the pattern's mechanism.

Catalog lines: id | name | type-in -> type-out | mechanism

Output ONE JSON object and nothing else:
{"picks":[{"id":"lex-xxxxx","confidence":0.0,"evidence":"...","why":"..."}]}

Rules:
- At most 3 picks, strongest first. An empty array is a correct and common answer: most passages of ordinary exposition, narration, or transition instantiate nothing in the catalog. Do not fill slots.
- confidence is your estimate that the pattern's MECHANISM is present, not that its vocabulary is. 0.85 or above only when a reader who knows the pattern would name it unprompted from this passage alone. 0.65 to 0.85 when the mechanism is there but partial, implicit, or one of several fair readings. Below 0.6, omit the pick.
- evidence is the exact words from the passage that carry the mechanism: one contiguous span of 5 to 30 words, copied character for character. Never paraphrase, never merge two places, never quote the pattern's name. This string is matched back against the passage; if you cannot point at a span, the pick does not belong.
- why is one plain sentence, at most 25 words, saying how THIS passage does what the pattern names. Specific to the passage's content, not a restatement of the pattern's name or mechanism.
- Shared vocabulary is not a match. A passage that uses the word "faction" does not instantiate a pattern with "faction" in its name unless the mechanism is present.
- A pattern that describes the general subject the passage is about, without the passage doing or exhibiting the move, is not a match.`

// passageIndexLine renders one candidate: id | name | in -> out |
// mechanism. The mechanism is the opening of the agent-instruction (the
// field that states what the move is and when it applies), cut at the
// first sentence boundary past 80 characters or at 240 characters on a
// word boundary. Falls back to the first canonical-instance when an atom
// has no agent-instruction.
func passageIndexLine(e *types.LexEntry) string {
	mech := mechanismBrief(e)
	return fmt.Sprintf("%s | %s | %s -> %s | %s\n", e.ID, e.Name, e.TypeIn, e.TypeOut, mech)
}

const (
	mechanismMinCut = 80
	mechanismMaxCut = 240
)

func mechanismBrief(e *types.LexEntry) string {
	src := strings.TrimSpace(e.AgentInstruction)
	if src == "" && len(e.CanonicalInstances) > 0 {
		src = strings.TrimSpace(e.CanonicalInstances[0])
	}
	src = strings.Join(strings.Fields(src), " ")
	if len(src) <= mechanismMaxCut {
		return src
	}
	// First sentence boundary after the minimum — agent-instructions
	// often open with "When X, do Y -- because Z", and the clause before
	// the first full stop is the mechanism statement.
	if i := strings.Index(src[mechanismMinCut:], ". "); i >= 0 && mechanismMinCut+i+1 <= mechanismMaxCut {
		return src[:mechanismMinCut+i+1]
	}
	cut := mechanismMaxCut
	for i := mechanismMaxCut; i > mechanismMaxCut-40; i-- {
		if src[i] == ' ' {
			cut = i
			break
		}
	}
	return src[:cut] + "…"
}

// buildPassageIndex renders the candidate lines in ID order. The order
// carries no ranking information on purpose: the embed gate's cosine
// order is a retrieval artefact, and presenting it as a list order would
// bias the model toward the top of the list.
func buildPassageIndex(pool []*types.LexEntry) string {
	var b strings.Builder
	for _, e := range sortedIndexable(pool) {
		b.WriteString(passageIndexLine(e))
	}
	return b.String()
}

// FilterPassage asks the model which candidates the passage instantiates.
// model may be empty (client default). maxPicks <= 0 means
// MaxPassagePicks. Picks whose id is not in pool are dropped; confidences
// are clamped to [0,1]; the result is sorted by confidence descending.
func FilterPassage(ctx context.Context, passage string, meta PassageMeta, pool []*types.LexEntry, c client.Client, model string, maxPicks int) (PassageResult, error) {
	if c == nil {
		return PassageResult{}, fmt.Errorf("lens: passage: nil client")
	}
	if len(pool) == 0 {
		return PassageResult{}, nil
	}
	if maxPicks <= 0 || maxPicks > MaxPassagePicks {
		maxPicks = MaxPassagePicks
	}

	timeout := DefaultPassageTimeout
	if v := os.Getenv("LEXICON_LENS_TIMEOUT_MS"); v != "" {
		if ms, err := time.ParseDuration(v + "ms"); err == nil && ms > 0 {
			timeout = ms
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var user strings.Builder
	user.WriteString("Text: ")
	user.WriteString(meta.Title)
	if meta.Author != "" {
		user.WriteString(", ")
		user.WriteString(meta.Author)
	}
	if meta.Year != 0 {
		fmt.Fprintf(&user, " (%d)", meta.Year)
	}
	user.WriteString("\n\nPassage:\n")
	user.WriteString(strings.TrimSpace(passage))
	user.WriteString("\n\nCatalog:\n")
	user.WriteString(buildPassageIndex(pool))
	user.WriteString("\nReturn the JSON object.")

	resp, err := c.CreateMessage(ctx, client.MessageRequest{
		System:    passageSystemPrompt,
		UserText:  user.String(),
		MaxTokens: 800,
		Model:     model,
	})
	out := PassageResult{
		Usage: Usage{
			InputTokens:         resp.InputTokens,
			OutputTokens:        resp.OutputTokens,
			CacheReadTokens:     resp.CacheReadTokens,
			CacheCreationTokens: resp.CacheCreationTokens,
		},
		Raw: resp.Text,
	}
	if err != nil {
		return out, fmt.Errorf("lens: passage: llm: %w", err)
	}
	picks, err := parsePassageResponse(resp.Text)
	if err != nil {
		return out, fmt.Errorf("lens: passage: parse: %w", err)
	}

	inPool := make(map[string]bool, len(pool))
	for _, e := range pool {
		inPool[e.ID] = true
	}
	for _, p := range picks {
		if !inPool[p.ID] {
			continue
		}
		if p.Confidence < 0 {
			p.Confidence = 0
		}
		if p.Confidence > 1 {
			p.Confidence = 1
		}
		out.Picks = append(out.Picks, p)
	}
	sort.SliceStable(out.Picks, func(i, j int) bool { return out.Picks[i].Confidence > out.Picks[j].Confidence })
	if len(out.Picks) > maxPicks {
		out.Picks = out.Picks[:maxPicks]
	}
	return out, nil
}

// parsePassageResponse reads {"picks":[{id,confidence,evidence,why}]}
// with the same tolerance as parseLensResponse: first '{' to its
// matching '}', trailing commas repaired. Duplicate ids keep the first.
func parsePassageResponse(text string) ([]PassagePick, error) {
	start := strings.Index(text, "{")
	if start < 0 {
		return nil, fmt.Errorf("no JSON object in response: %q", text)
	}
	end, err := findMatchingClose(text, start, '{', '}')
	if err != nil {
		return nil, fmt.Errorf("malformed JSON object in response: %w: %q", err, text)
	}
	var doc struct {
		Picks []struct {
			ID         string  `json:"id"`
			Confidence float64 `json:"confidence"`
			Evidence   string  `json:"evidence"`
			Why        string  `json:"why"`
		} `json:"picks"`
	}
	if err := unmarshalWithRepair(text[start:end+1], &doc); err != nil {
		return nil, fmt.Errorf("unmarshal %q: %w", text[start:end+1], err)
	}
	seen := make(map[string]bool, len(doc.Picks))
	out := make([]PassagePick, 0, len(doc.Picks))
	for _, p := range doc.Picks {
		id := strings.TrimSpace(p.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, PassagePick{
			ID:         id,
			Confidence: p.Confidence,
			Evidence:   strings.TrimSpace(p.Evidence),
			Why:        strings.TrimSpace(p.Why),
		})
	}
	return out, nil
}

// LocateEvidence finds evidence inside passage and returns the span as
// rune offsets [start, end) into passage. Matching ignores case, drops
// quotation marks (a model writes 'x' where the text has "x"), folds
// dashes to ASCII, and treats any run of whitespace as one space — the
// source texts are hard-wrapped at ~70 columns with CRLF line ends, and
// the model quotes them unwrapped. If the full span is not found, the
// leading words are tried in decreasing counts (8, 6, 4), then the
// trailing words, so a quote the model mis-copied at one end still
// anchors to the other; partial is true in that case. ok is false when
// nothing anchors — typically a quote spliced from two places.
func LocateEvidence(passage, evidence string) (start, end int, partial, ok bool) {
	normP, mapP := normalizeForMatch(passage)
	// A model quoting verse writes " / " for a line break the source has
	// as a newline. Only the evidence is rewritten — the passage's rune map
	// must stay exact.
	normE, _ := normalizeForMatch(strings.ReplaceAll(evidence, " / ", " "))
	normE = strings.TrimSpace(normE)
	if normE == "" {
		return 0, 0, false, false
	}
	n := len([]rune(passage))
	if s, e, found := findNormalized(normP, mapP, normE, n); found {
		return s, e, false, true
	}
	words := strings.Fields(normE)
	for _, k := range []int{8, 6, 4} {
		if len(words) <= k {
			continue
		}
		if s, e, found := findNormalized(normP, mapP, strings.Join(words[:k], " "), n); found {
			return s, e, true, true
		}
	}
	for _, k := range []int{8, 6, 4} {
		if len(words) <= k {
			continue
		}
		if s, e, found := findNormalized(normP, mapP, strings.Join(words[len(words)-k:], " "), n); found {
			return s, e, true, true
		}
	}
	return 0, 0, false, false
}

// findNormalized locates needle in the normalized haystack and maps the
// hit back to original rune offsets via mapP (normalized rune index ->
// original rune index). end maps to one past the last matched rune.
func findNormalized(normP string, mapP []int, needle string, passageRunes int) (int, int, bool) {
	i := strings.Index(normP, needle)
	if i < 0 {
		return 0, 0, false
	}
	// strings.Index returns a byte offset; convert to rune index.
	ri := len([]rune(normP[:i]))
	rj := ri + len([]rune(needle))
	if ri >= len(mapP) {
		return 0, 0, false
	}
	start := mapP[ri]
	end := passageRunes
	if rj-1 < len(mapP) {
		end = mapP[rj-1] + 1
	}
	return start, end, true
}

// normalizeForMatch lowercases, drops quotation marks, folds dashes, and
// collapses whitespace, returning the normalized string and a map from
// each normalized rune's index to the original rune index it came from
// (a collapsed whitespace run maps to its first rune). Quotes are
// dropped rather than folded because the model's choice of single or
// double is unrelated to the text's, and a span that starts or ends on a
// quote mark should anchor either way.
func normalizeForMatch(s string) (string, []int) {
	var b strings.Builder
	var m []int
	inSpace := false
	for i, r := range []rune(s) {
		switch r {
		case '\'', '"', '‘', '’', '‚', '′', '“', '”', '„', '″':
			continue
		case '‐', '‑', '‒', '–', '—', '―':
			r = '-'
		}
		if unicode.IsSpace(r) {
			if inSpace {
				continue
			}
			inSpace = true
			b.WriteRune(' ')
			m = append(m, i)
			continue
		}
		inSpace = false
		b.WriteRune(unicode.ToLower(r))
		m = append(m, i)
	}
	return b.String(), m
}
