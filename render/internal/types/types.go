// Package types holds the on-disk shape of a lexicon entry plus the
// render/gate/session value types shared across packages.
//
// Schema accretion follows the 3-occurrence rule (winze convention
// back-port): a YAML field becomes a struct field once it has appeared
// in three or more entries. Until then it lives as ad-hoc YAML and is
// silently preserved by yaml.v3's mapping behavior on round-trip.
//
// Field tags use the canonical YAML names from SCHEMA.md, which include
// hyphens (`type-in`, `decomposes-into`, `critical-questions`) — so the
// Go field names are PascalCase but the YAML names match the elements.
package types

import "strings"

// LineageEntry is one provenance citation for a primitive. Until
// `quote` is populated with verbatim source text, the lineage is
// provisional and the entry stays `under-review`.
//
// Source is the lineage-quality category (the small enum: primary,
// practitioner, discovery-loop, cross-attestation, secondary).
// Tradition is the optional canon/school/work-cluster grouping
// (madhyamaka, walton, gof, etc.) — split out of Source to end the
// dual-encoding that mixed quality and grouping in one field.
type LineageEntry struct {
	Source    string `yaml:"source"`
	Tradition string `yaml:"tradition,omitempty"`
	Text      string `yaml:"text"`
	Citation  string `yaml:"citation"`
	Quote     string `yaml:"quote,omitempty"`
}

// unstakedSentinels are the leading markers the elements uses in a
// `quote:` field to say "no verbatim span is staked here." They accreted
// across separate audits, which is the whole problem this list solves:
// the renderers originally tested for "MISSING" alone, so the 63 entries
// marked with the later conventions — `[NOT VERIFIED at NN audit ...]`,
// `[NOT VERIFIED REFS-GROUNDED ...]`, `[paraphrase, not verbatim ...]`,
// `[MEMORY-LEVEL — verify before activation: ...]` — reported as
// VERIFIED on every human-facing surface. That inverts the signal on
// exactly the axis the quote-fidelity audit exists to protect.
//
// Matched as a PREFIX of the quote's leading bracketed segment, not
// anywhere in the field. Elements quotes legitimately contain the word
// "missing" in running prose (lex-gbru2, lex-tfpzb), and a substring test
// would flag those staked spans as unverified — the same bug pointed the
// other way.
var unstakedSentinels = []string{
	"MISSING",
	"NOT VERIFIED",
	"PARAPHRASE, NOT VERBATIM",
	"MEMORY-LEVEL",
}

// QuoteStaked reports whether a lineage entry's quote field carries an
// actual verbatim span, as opposed to being empty or bearing one of the
// unstaked-placeholder sentinels.
//
// This is the single definition of "is this citation staked" — graduation
// to `status: active` turns on it (elements-design principle 8), and
// every surface that badges a citation must ask here rather than
// re-deriving it from a magic string.
func (l LineageEntry) QuoteStaked() bool {
	return quoteStaked(l.Quote)
}

func quoteStaked(quote string) bool {
	q := strings.TrimSpace(quote)
	if q == "" {
		return false
	}
	// Strip one leading '[' so `[MISSING ...]` and a bare `MISSING ...`
	// are treated alike, then compare case-insensitively against the
	// sentinel prefixes.
	head := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(q, "[")))
	for _, s := range unstakedSentinels {
		if strings.HasPrefix(head, s) {
			return false
		}
	}
	return true
}

// LexEntry is one elements primitive (atom, molecule, or higher tier).
//
// Required fields per SCHEMA.md: ID, Name, TypeIn, TypeOut, Tier,
// Lineage (non-empty), CanonicalInstances (non-empty), Status.
//
// Molecule-tier additional fields (DecomposesInto, Premises,
// CriticalQuestions) become required for `_tier: molecule` and higher
// — but the loader doesn't enforce that yet because pre-mining
// molecules sometimes carry MISSING flags in DecomposesInto rather
// than full constituents.
type LexEntry struct {
	ID                 string         `yaml:"id"`
	Name               string         `yaml:"name"`
	// CommonName is the optional jargon-free, lay-readable label (~2-5 words)
	// for human-facing consumers (e.g., adversaria's reveal-card titles).
	// Descriptive-of-the-pattern, NOT prescriptive — should not telegraph
	// the "right answer" since downstream consumers may use it to hint at
	// a hidden pattern the player is meant to surface. Optional; consumers
	// fall back to humanizing Name when absent. Added V118 ba per
	// adversaria feature request 2026-06-22.
	CommonName         string         `yaml:"common-name,omitempty"`
	TypeIn             string         `yaml:"type-in"`
	TypeOut            string         `yaml:"type-out"`
	Tier               string         `yaml:"_tier"`
	Related            []string       `yaml:"related,omitempty"`
	// ScaffoldsFrom names atoms that prime your grasp of this one — soft
	// pedagogical scaffolding (not strict prerequisite). "Exposure helps;
	// not required to land." Directed but cycles ARE allowed (mutually-
	// priming pairs are meaningful, not a structural violation — lint
	// surfaces them as info, not error). Distinct from Related[] which
	// is symmetric/undirected/clustering. Locked in V118 at after a
	// 5-lens panel pressure-test refined the original "prerequisite"
	// proposal. See docs/principles/scaffolds-from-design.md.
	ScaffoldsFrom      []string       `yaml:"scaffolds-from,omitempty"`
	// EncounterTierOverride is the rare-case escape hatch for atoms
	// where the derived tier (computed by `lexicon tier-derive` from
	// lineage tradition + source + in-degree) is wrong — the Hofstadter-
	// translation case where tier-5 content has been compressed to
	// tier-2 prose. Integer 1-5; absent for the common case where
	// derived is correct. Lint emits info when override diverges >1
	// from derived. No stored encounter-tier itself — tier is a derived
	// view, not a stored property, per panel-skeptic refinement.
	EncounterTierOverride int `yaml:"encounter-tier-override,omitempty"`
	Evokes             []string       `yaml:"evokes,omitempty"`
	Premises           []string       `yaml:"premises,omitempty"`
	DecomposesInto     []string       `yaml:"decomposes-into,omitempty"`
	CriticalQuestions  []string       `yaml:"critical-questions,omitempty"`
	Assembly           string         `yaml:"assembly,omitempty"`
	Lineage            []LineageEntry `yaml:"lineage"`
	CanonicalInstances []string       `yaml:"canonical-instances"`
	SeverityTier       string         `yaml:"severity-tier,omitempty"`
	Status             string         `yaml:"status"`
	FormalIfAny        string         `yaml:"formal-if-any,omitempty"`

	// AgentInstruction is the operational "when you see this pattern,
	// do this" one-liner. Distinct from critical-questions (diagnostic
	// probes) — this is prescriptive: the agent-side decision rule. Added
	// V116 e for agent-tool-first surface. Optional; coverage builds
	// incrementally via dedicated CQ-fill / agent-instruction passes.
	AgentInstruction string `yaml:"agent-instruction,omitempty"`

	// Reaction-tier fields (_tier: reaction). A reaction is a
	// transformation — reactants turn into products via a mechanism,
	// modulated by catalysts (accelerate) and inhibitors (block) and
	// gated by conditions. Promoted to struct fields once the 14 V71
	// reactions cleared the 3-occurrence rule. See
	// docs/principles/reaction-tier-design.md. Mechanism is free-text
	// (NOT type-checked like Assembly — reaction pathways cross types).
	Reactants     []string `yaml:"reactants,omitempty"`
	Products      []string `yaml:"products,omitempty"`
	Mechanism     string   `yaml:"mechanism,omitempty"`
	Catalysts     []string `yaml:"catalysts,omitempty"`
	Inhibitors    []string `yaml:"inhibitors,omitempty"`
	Conditions    []string `yaml:"conditions,omitempty"`
	Reversibility string   `yaml:"reversibility,omitempty"`
}

// RenderMode names the representational modality the render function
// produces. v0 ships algebraic / meta-explanatory / narrative / visual
// / introspection. Phenomenological and dialogical modes are deferred
// per the molecule-representation-design.md 3-occurrence gating.
type RenderMode string

const (
	ModeAlgebraic       RenderMode = "algebraic"
	ModeNarrative       RenderMode = "narrative"
	ModeVisual          RenderMode = "visual"
	ModeIntrospection   RenderMode = "introspection"
	ModeMetaExplanatory RenderMode = "meta-explanatory"
)

// RenderOutput is what every per-mode renderer returns. Text is the
// user-facing payload; IntrospectionTrace is the optional --why
// appendix (token counts for LLM modes, classifier branch for the
// gate, etc.).
type RenderOutput struct {
	PrimitiveID        string
	Mode               RenderMode
	Text               string
	IntrospectionTrace string
}

// GateResult is one ranked entry from the deterministic gate.
type GateResult struct {
	PrimitiveID string
	ModeHint    RenderMode
	Score       float64
	Reason      string
}

// SessionTag is the per-session gut-check vibe. Per Q5 of
// render-function-design-v0.md: this is intentionally coarse; do NOT
// add per-call sentiment fields here. The "autonomous" value marks
// sessions where Claude was running unattended so they can be
// filtered out of human-driven gut-check totals.
type SessionTag string

const (
	TagUseful     SessionTag = "useful"
	TagMixed      SessionTag = "mixed"
	TagNotUseful  SessionTag = "not-useful"
	TagAutonomous SessionTag = "autonomous"
)

// ValidSessionTag returns true iff s is one of the allowed tags. Used
// by the CLI to validate user input before append.
func ValidSessionTag(s SessionTag) bool {
	switch s {
	case TagUseful, TagMixed, TagNotUseful, TagAutonomous:
		return true
	}
	return false
}

// SessionLogEntry is one line in session-log.jsonl. Append-only.
// Per Q5: render_call_count is informational; the load-bearing field
// is Vibe.
type SessionLogEntry struct {
	SessionDate     string     `json:"session_date"`
	RenderCallCount int        `json:"render_call_count"`
	Vibe            SessionTag `json:"vibe"`
	Notes           string     `json:"notes,omitempty"`
}
