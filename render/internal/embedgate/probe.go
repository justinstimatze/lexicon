package embedgate

// probe.go — the held-out POS/NEG seed corpus used by `lexicon calib` to derive
// a data-driven threshold for the embedding gate. Pattern lifted from cupel's
// probe.go (github.com/justinstimatze/cupel, cmd/cupel/probe.go).
//
// HELD-OUT INTEGRITY: Each POS prompt is in user-prompt register (the way someone
// would type the situation into Claude) and deliberately avoids overlap with
// any of the gold atom's PrototypeTexts (name + one canonical-instance + evokes,
// one text per instance — see gate.go). A high cosine here reflects the
// situational register the gate must catch, not lexical echo from a prototype.
// NEG examples are ordinary benign work-or-life prose — programming questions,
// planning, narrative — that should not match any atom strongly.
//
// SCOPE: The corpus is a representative slice, not a full atom census. It spans
// a programming pattern (lex-z8m97), reasoning patterns (lex-kebfa, lex-betym),
// social/cognitive (lex-vacbr, lex-h2ehw), a feminist-theory cluster
// (lex-cgqta, lex-kny9v, lex-6qevt, lex-ymckb, lex-et8q2, lex-jnvtj), a
// creative/labor-economics cluster (lex-smxtw, lex-f74h5), classic mechanism patterns
// (lex-spm8x, lex-33w6z), and pedagogical/method (lex-bpr6b). The threshold
// derived here should generalize to the rest of the elements; if a future
// atom-cluster reveals systematic separation difference, extend the corpus and
// re-calibrate. Counts are deliberately not stated: this comment previously
// claimed "14 atoms" while listing 13 and the literal held 16, and cited a
// "247-atom elements" that had since passed a thousand. Read the literal.
//
// OVERRIDABLE: set LEXICON_CALIB_CORPUS=/path.json to a JSON array of
// {"atom_id":"lex-NNNN","text":"..."} objects (atom_id:"" ⇒ negative) to
// calibrate against a richer or session-derived corpus without recompiling.

import (
	"encoding/json"
	"os"
)

// ProbeExample is one labeled corpus line. AtomID=="" marks a benign negative.
type ProbeExample struct {
	AtomID string `json:"atom_id"`
	Text   string `json:"text"`
}

// builtinProbeCorpus is the committed seed set. Each POS is held-out from the
// gold atom's prototype (no shared phrasings); each NEG is ordinary work or
// narrative prose. Smoke-test sims already observed for some of these — see
// SESSION-53-HANDOFF.md §V53.gate smoke table.
var builtinProbeCorpus = []ProbeExample{
	// lex-z8m97 gof-decorator — wrap behavior dynamically without subclassing
	{"lex-z8m97", "I want to add logging and rate-limiting around an existing HTTP handler in Go without modifying its signature; what's the cleanest way to stack those concerns?"},
	{"lex-z8m97", "How do I add cross-cutting behavior like auth checks and metrics to a function chain such that each piece is independently swappable?"},

	// lex-bpr6b polya-work-backwards-via-related-problem
	{"lex-bpr6b", "This bug only shows up under load; I have a stack trace from prod and no local repro. How do I trace back from the symptom to a hypothesis I can test?"},
	{"lex-bpr6b", "I'm stuck on a hard proof — is there a known result this might reduce to, working backwards from the conclusion?"},

	// lex-kebfa argument-from-expert-opinion (presumption + defeaters)
	{"lex-kebfa", "A senior researcher with a long track record said treatment X is effective for condition Y — how much weight does that carry, and when should I push back?"},
	{"lex-kebfa", "Two domain experts disagree on whether this architectural choice scales; how do I think about whose testimony to defer to?"},

	// lex-spm8x feedback-loop-as-mechanism
	{"lex-spm8x", "The system's output is being fed back as its next input and the behavior is drifting — what's the right framing for analyzing why it diverges versus settles?"},
	{"lex-spm8x", "Thermostats, autopilots, and the body's blood-sugar regulation all do the same thing structurally. What's that underlying mechanism called?"},

	// lex-33w6z sacrifice-as-transformation
	{"lex-33w6z", "She gave up the partner-track offer to do the PhD. The giving-up itself seems to be what made the new identity stick — what's that pattern?"},
	{"lex-33w6z", "Why does paying real costs to get into a club feel more transformative than just being granted membership?"},

	// lex-vacbr identity-protective-cognition
	{"lex-vacbr", "Smart, well-educated people seem to reject empirical findings most strongly when those findings clash with their political tribe's commitments. What's that effect called?"},
	{"lex-vacbr", "My uncle keeps dismissing the data on climate change in a way that tracks his social group more than the evidence — is there a name for that bias?"},

	// lex-h2ehw trust-as-elements-for-obedience (Milgram)
	{"lex-h2ehw", "Why did the Yale-coat experiments get such high compliance? People weren't being coerced — something about the institutional setting seems load-bearing."},
	{"lex-h2ehw", "Patients let nurses do invasive things they'd never let a stranger do; the willingness seems to ride on the uniform and the building, not the procedure itself."},

	// lex-betym confirmation-filtered-curiosity-exploration (Wason 2-4-6)
	{"lex-betym", "I notice I keep generating tests that would confirm my hypothesis rather than ones that would break it. The search itself feels curiosity-driven but it's tilted."},
	{"lex-betym", "Why do people in the 2-4-6 task feel like they're exploring openly but end up never testing a number sequence that would disprove their rule?"},

	// lex-smxtw competence-substitution-inverts-the-seniority-premium
	{"lex-smxtw", "The mid-career professionals at our firm are the ones most exposed to the new tools — the juniors and the very-senior partners are weirdly safer. What's the shape of this?"},
	{"lex-smxtw", "AI is hollowing out the experienced-but-not-elite layer of knowledge work, where the experience premium used to live; how do I think about that displacement?"},

	// lex-f74h5 authenticity-premium-rises-with-slop
	{"lex-f74h5", "As AI-generated content floods every channel, the price of verified-human creative work seems to be climbing. Is there a market dynamic that predicts this?"},
	{"lex-f74h5", "Cheap synthetic content is everywhere now; ironically, hand-crafted alternatives are seeing premium pricing. What's the economic mechanism?"},

	// lex-cgqta naturalization-erases-the-mechanism-that-produced-the-effect (V53.1)
	{"lex-cgqta", "The system kept women out of the field for a century and now points at the demographic outcome as evidence women aren't suited for it. What's that move?"},
	{"lex-cgqta", "A culture imposes a constraint, then forgets it imposed the constraint, and treats the resulting pattern as natural. Is there a name for that erasure?"},

	// lex-kny9v material-conditions-as-elements-of-intellectual-production (V53.1)
	{"lex-kny9v", "Why does it matter so much that Woolf insisted on money and a room of one's own? The argument seems to be about the prerequisites for thinking, not the thinking itself."},
	{"lex-kny9v", "Whose voices we hear from in any era seems to track who had the leisure and stability to write — what's the load-bearing concept there?"},

	// lex-6qevt lived-position-as-irreplaceable-epistemic-elements (V53.1)
	{"lex-6qevt", "Sympathetic outsiders can study a community deeply but there's still a kind of knowledge that only a member can produce. What's the framing for that epistemic gap?"},
	{"lex-6qevt", "Cooper's preface from 1892 argued that a particular standpoint had a representational role no second-hand witness could fill — what's the pattern called?"},

	// lex-ymckb subordinated-group-as-magnifying-mirror-for-dominant-self (V53.2)
	{"lex-ymckb", "Mild critique from a subordinated group seems to enrage dominant-group members way out of proportion to the content. Like the criticism is threatening some load-bearing self-reflection, not just a claim. What's the structural pattern?"},
	{"lex-ymckb", "Why does it feel like a certain class of person needs the deference of others in order to feel real — and any withdrawal of that deference produces disproportionate anger? Woolf had a phrase about a looking-glass."},

	// lex-et8q2 intersectional-position-not-decomposable-into-component-axes (V53.2)
	{"lex-et8q2", "When we analyze Black women's experience as race plus gender, we keep missing things specific to the intersection. What's the framing that treats the intersection as its own irreducible position, not the sum of the axes?"},
	{"lex-et8q2", "Crenshaw's 1989 argument is that an intersectional position isn't the addition of component axes — it's a structural position in its own right. What's the underlying claim, and where does it bite when policy tries to decompose it?"},

	// lex-jnvtj incoherent-demands-as-control-mechanism (V53.2)
	{"lex-jnvtj", "My boss expects me to be both highly assertive AND highly accommodating, knowing both can't be jointly satisfied, and every failure to thread the needle is cited as evidence I'm not leadership material. Is this a recognized dynamic?"},
	{"lex-jnvtj", "There's a kind of double-bind where the demands themselves are contradictory and the chronic-failure-to-meet-both becomes the evidence used against the person. The contradiction looks like the point, not bad design — what's that called?"},

	// negatives — ordinary benign work / mundane requests / neutral narrative
	{"", "What time does the post office on Mission Street close on Saturdays?"},
	{"", "Help me write a short thank-you note to my landlord for fixing the heater."},
	{"", "I'm planning a long weekend in Portland in October — what neighborhoods should I stay in if I want walkable food options?"},
	{"", "Convert this CSV to a table I can paste into Notion: name,age,city\\nA,30,NYC\\nB,25,LA"},
	{"", "She had the kettle on and the morning light was coming in through the kitchen window, low and gold across the table."},
	{"", "What's the syntax for a for-loop in Lua again?"},
	{"", "Remind me what mise en place means in a kitchen context."},
}

// probeCorpus returns the active corpus — LEXICON_CALIB_CORPUS override if set
// and parseable, else builtinProbeCorpus.
func probeCorpus() []ProbeExample {
	if p := os.Getenv("LEXICON_CALIB_CORPUS"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			var c []ProbeExample
			if json.Unmarshal(b, &c) == nil && len(c) > 0 {
				return c
			}
		}
	}
	return builtinProbeCorpus
}

// ProbePositives returns the labeled atom-matched examples (AtomID != "").
func ProbePositives() []ProbeExample {
	var out []ProbeExample
	for _, e := range probeCorpus() {
		if e.AtomID != "" {
			out = append(out, e)
		}
	}
	return out
}

// ProbeNegatives returns the benign example texts (AtomID == "").
func ProbeNegatives() []string {
	var out []string
	for _, e := range probeCorpus() {
		if e.AtomID == "" {
			out = append(out, e.Text)
		}
	}
	return out
}
