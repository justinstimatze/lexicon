package lexicon

import (
	"regexp"
	"strings"
)

// ExtractPromptVocab tokenizes prompt into lowercase, deduplicated,
// stop-word-filtered terms of length >= 3. Feeding this as gate.Input's
// WorkingVocab is what lets a caller's own name/evokes tokens lift
// relevant atoms above the tier×status tie pool -- without it, scoring
// falls back to tier×status alone and ties on an ID-order tiebreak.
func ExtractPromptVocab(prompt string) []string {
	splitter := regexp.MustCompile(`[^a-zA-Z\-]+`)
	tokens := splitter.Split(strings.ToLower(prompt), -1)
	out := make([]string, 0, len(tokens))
	seen := map[string]bool{}
	for _, t := range tokens {
		if len(t) < 3 || stopWords[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// Stop-word list — cuts common-word noise from vocab matching. Two
// classes: (1) English function words; (2) generic content tokens that
// recur across most coding/conversation prompts and would trigger
// spurious name/evokes matches against elements entries. The class-2
// expansion (V7) addresses the V6-empirical lex-bpr6b dominance: its name
// `polya-work-backwards-via-related-problem` was matching ~15% of all
// prompts via the generic tokens work/related/problem/via — these now
// fall out of working vocab and stop triggering the 1.6x name boost.
var stopWords = map[string]bool{
	// class 1: English function words
	"the": true, "and": true, "for": true, "you": true, "are": true,
	"but": true, "not": true, "this": true, "that": true, "with": true,
	"have": true, "what": true, "all": true, "can": true, "use": true,
	"how": true, "from": true, "your": true, "any": true, "out": true,
	"its": true, "one": true, "more": true, "should": true, "want": true,
	"need": true, "into": true, "now": true, "they": true, "them": true,
	"who": true, "why": true, "when": true, "where": true, "which": true,
	"will": true, "would": true, "could": true, "may": true, "might": true,
	"about": true, "just": true, "like": true, "than": true, "then": true,
	// class 2: generic content tokens (V7 expansion)
	"work": true, "related": true, "problem": true, "method": true,
	"pattern": true, "anchor": true, "strategy": true, "plan": true,
	"via": true, "find": true, "make": true, "way": true, "thing": true,
	"here": true, "there": true, "also": true, "well": true, "very": true,
	"some": true, "such": true, "only": true, "much": true, "many": true,
	"each": true, "every": true, "other": true, "another": true,
}
