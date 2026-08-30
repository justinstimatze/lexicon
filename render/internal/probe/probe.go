// Package probe generates disambiguation questions for an ambiguous
// situation, based on the top-K elements atoms surfaced by the gate.
//
// V0 design (V36): probes come in two flavors:
//
//   - generic: fortune-teller staples (decision-vs-posture, actor scope,
//     horizon, already-tried, no-go) that work for any situation.
//     Elements-agnostic. THIS IS THE LIFT-OUT TARGET for a future sibling
//     "ambiguity-probe" tool that fires on every UserPromptSubmit (probable
//     name: inkling). Keep genericProbes free of elements-aware
//     dependencies so the lift is mechanical.
//
//   - card-driven: probes that disambiguate WHICH of the top-K cards
//     actually applies. Elements-aware; uses type-in/type-out slot
//     straddle detection and surfaces critical-questions from cards.
//
// Future mesh with inkling (task #77): when inkling has a persistent
// user-mental-model, probe-mode should consume it — skip probes for axes
// inkling has high-confidence on (don't ask what's clearly demonstrated),
// surface probes for axes inkling flags as gap-flagged.
package probe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/framestatus"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Probe is one suggested disambiguation question.
type Probe struct {
	Question  string
	Source    string // "generic" | "card-driven" | "card-critical-question"
	Rationale string // why this probe matters (for Claude to understand intent)
}

// Input bundles everything the probe generator needs.
type Input struct {
	Context     string
	TopK        []*types.LexEntry
	MaxProbes   int
	FrameStatus framestatus.Map // optional; nil/empty = no frame labels
}

// Output is what Generate returns.
type Output struct {
	Probes      []Probe
	Ambiguities []string
}

// DefaultMaxProbes caps the probe list; more than this is over-interrogation.
const DefaultMaxProbes = 5

// Generate returns a set of disambiguation probes for the given context
// and top-K candidate atoms.
func Generate(in Input) Output {
	max := in.MaxProbes
	if max <= 0 {
		max = DefaultMaxProbes
	}

	probes := []Probe{}
	probes = append(probes, cardDrivenProbes(in.TopK)...) // elements-aware first (more relevant)
	probes = append(probes, genericProbes(in.Context)...)
	if len(probes) > max {
		probes = probes[:max]
	}

	return Output{
		Probes:      probes,
		Ambiguities: detectAmbiguities(in.TopK),
	}
}

// genericProbes returns fortune-teller staples that work regardless of
// elements. ELEMENTS-AGNOSTIC — keep this function lift-out-ready.
func genericProbes(ctxStr string) []Probe {
	lower := strings.ToLower(ctxStr)
	all := []Probe{
		{
			Question:  "Is this a one-shot decision, or are you setting a recurring stance?",
			Source:    "generic",
			Rationale: "decision-vs-posture picks which class of pattern applies",
		},
		{
			Question:  "Who is the decision being made by vs for — you, the team, or someone above?",
			Source:    "generic",
			Rationale: "actor scope changes which constraints bind",
		},
		{
			Question:  "What's the time horizon — days, weeks, months, years?",
			Source:    "generic",
			Rationale: "horizon picks between optionality-preservation and commitment-shaped moves",
		},
		{
			Question:  "What's already been tried that didn't work?",
			Source:    "generic",
			Rationale: "un-named tried-and-failed approaches lurk; rule-out set is load-bearing",
		},
		{
			Question:  "What would force a no-go answer?",
			Source:    "generic",
			Rationale: "surfaces conjugate constraints that aren't yet stated",
		},
	}

	// Cheap relevance filter: if the context already states a thing, that
	// generic probe is redundant. Move redundant probes to the end so the
	// MaxProbes truncation drops them first.
	hasDeadlineSignal := containsAny(lower, "deadline", "by friday", "by monday", "this week", "next week", "weeks", "months", "quarter", "horizon", "timeline")
	hasActorSignal := containsAny(lower, "my team", "we ", "i'm ", "i am ", "the team", "our ")
	hasTriedSignal := containsAny(lower, "tried", "already", "rejected", "failed", "didn't work", "abandoned")

	score := func(p Probe) int {
		s := 0
		if strings.Contains(p.Question, "horizon") && hasDeadlineSignal {
			s -= 10
		}
		if strings.Contains(p.Question, "Who is") && hasActorSignal {
			s -= 10
		}
		if strings.Contains(p.Question, "already been tried") && hasTriedSignal {
			s -= 10
		}
		return s
	}
	sort.SliceStable(all, func(i, j int) bool { return score(all[i]) > score(all[j]) })
	return all
}

// cardDrivenProbes generates probes that depend on the top-K elements atoms.
func cardDrivenProbes(topK []*types.LexEntry) []Probe {
	out := []Probe{}

	// type-in straddle: top-K cards from different stances
	stances := map[string]bool{}
	for _, e := range topK {
		if s := stanceOf(e.TypeIn); s != "" {
			stances[s] = true
		}
	}
	if len(stances) >= 2 {
		out = append(out, Probe{
			Question:  fmt.Sprintf("Is this fundamentally about %s? Top-K straddles these stances.", joinKeys(stances, " or ")),
			Source:    "card-driven",
			Rationale: "top-K split across type-in slots; answer picks which subset of cards applies",
		})
	}

	// Surface critical-questions breadth-first across top-K: one per card
	// (round 0), then a second from each card if budget remains (round 1).
	// Avoids a single critical-question-rich card monopolizing, but still
	// pulls a second from it when other cards have nothing.
	const cqBudget = 2
	cq := 0
	for round := 0; round < 2 && cq < cqBudget; round++ {
		for _, e := range topK {
			if cq >= cqBudget {
				break
			}
			if len(e.CriticalQuestions) <= round {
				continue
			}
			out = append(out, Probe{
				Question:  truncateLine(e.CriticalQuestions[round], 220),
				Source:    "card-critical-question",
				Rationale: fmt.Sprintf("from %s; answering determines whether this pattern actually fires", e.ID),
			})
			cq++
		}
	}

	return out
}

// detectAmbiguities returns human-readable descriptions of axes the
// future inkling tool will eventually track. V0 surfaces three: type-in
// straddle, critical-question density, lineage divergence.
func detectAmbiguities(topK []*types.LexEntry) []string {
	out := []string{}

	stances := map[string]bool{}
	cqTotal := 0
	sources := map[string]bool{}
	for _, e := range topK {
		if s := stanceOf(e.TypeIn); s != "" {
			stances[s] = true
		}
		cqTotal += len(e.CriticalQuestions)
		for _, l := range e.Lineage {
			// Group by the most granular handle available: tradition >
			// text > source. After the V118 source/tradition split,
			// `source:` is a coarse quality enum (primary, practitioner,
			// ...) and would collapse most atoms into one bucket — use
			// tradition or text for the divergence signal.
			switch {
			case l.Tradition != "":
				sources[l.Tradition] = true
			case l.Text != "":
				sources[l.Text] = true
			case l.Source != "":
				sources[l.Source] = true
			}
		}
	}

	if len(stances) >= 2 {
		out = append(out, fmt.Sprintf("type-in straddle: %s", joinKeys(stances, ", ")))
	}
	if cqTotal > 0 {
		out = append(out, fmt.Sprintf("critical-questions available: %d across top-K", cqTotal))
	}
	if len(sources) >= 2 {
		out = append(out, fmt.Sprintf("disparate lineages: %d distinct sources in top-K", len(sources)))
	}
	return out
}

// FormatMarkdown turns Input + Output into a markdown doc for Claude to
// consume. Claude paraphrases this into conversation; the markdown is
// not directly user-facing.
func FormatMarkdown(in Input, out Output) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# probes for: %s\n\n", truncateLine(in.Context, 100))

	fmt.Fprintln(&b, "## top-K candidates considered")
	for _, e := range in.TopK {
		canon := ""
		if len(e.CanonicalInstances) > 0 {
			canon = truncateLine(e.CanonicalInstances[0], 110)
		}
		fs, fsKnown := in.FrameStatus.Lookup(e.ID)
		if fsKnown {
			fmt.Fprintf(&b, "- **%s** %s — *frame: %s*\n  %s\n", e.ID, e.Name, fs.Label(), canon)
			if fs.Status == framestatus.Mixed && fs.Handle != "" {
				fmt.Fprintf(&b, "  _checkable handle: %s_\n", fs.Handle)
			}
		} else {
			fmt.Fprintf(&b, "- **%s** %s\n  %s\n", e.ID, e.Name, canon)
		}
	}

	if len(out.Ambiguities) > 0 {
		fmt.Fprintln(&b, "\n## ambiguity axes detected")
		for _, a := range out.Ambiguities {
			fmt.Fprintf(&b, "- %s\n", a)
		}
	}

	fmt.Fprintln(&b, "\n## suggested probes (ask BEFORE picking a card)")
	for i, p := range out.Probes {
		fmt.Fprintf(&b, "%d. *[%s]* %s\n", i+1, p.Source, p.Question)
		if p.Rationale != "" {
			fmt.Fprintf(&b, "   _why: %s_\n", p.Rationale)
		}
	}

	fmt.Fprintln(&b, "\n## how to use")
	fmt.Fprintln(&b, "Ask 2-3 of these in conversation BEFORE letting a card fire. Specific > generic; card-driven probes tell you WHICH pattern applies, generic probes resolve fortune-teller staples (decision-vs-posture, actor, horizon, already-tried). Skip probes whose answers are already in the conversation.")

	return b.String()
}

// stanceOf extracts the input-half of a type-in slot. Elements type-in
// fields look like "decision → action" or "state → posture"; we want
// the left side as the "kind of input" name.
func stanceOf(typeIn string) string {
	for _, sep := range []string{" → ", " -> ", "→", "->"} {
		if i := strings.Index(typeIn, sep); i >= 0 {
			return strings.TrimSpace(typeIn[:i])
		}
	}
	return strings.TrimSpace(typeIn)
}

func joinKeys(m map[string]bool, sep string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, sep)
}

func truncateLine(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
