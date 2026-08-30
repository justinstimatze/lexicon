package gate

import (
	"math"
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/framestatus"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func mkEntry(opts ...func(*types.LexEntry)) *types.LexEntry {
	e := &types.LexEntry{
		ID:                 "lex-0001",
		Name:               "test",
		TypeIn:             "claim",
		TypeOut:            "posture",
		Tier:               "atomic",
		Lineage:            []types.LineageEntry{{Source: "x"}},
		CanonicalInstances: []string{"x"},
		Status:             "active",
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

func withID(id string) func(*types.LexEntry)    { return func(e *types.LexEntry) { e.ID = id } }
func withTier(t string) func(*types.LexEntry)   { return func(e *types.LexEntry) { e.Tier = t } }
func withName(n string) func(*types.LexEntry)   { return func(e *types.LexEntry) { e.Name = n } }
func withStatus(s string) func(*types.LexEntry) { return func(e *types.LexEntry) { e.Status = s } }

// Empty context defaults to deployment — that's the most-common
// case for a "what move applies here?" query mid-conversation.
func TestClassifyContextEmptyIsDeployment(t *testing.T) {
	if ClassifyContext("") != StanceDeployment {
		t.Fatal("empty context should classify as deployment")
	}
}

// Mid-bind / stuck / right-now cues all signal deployment. Cue list
// is intentionally short and conservative — false-positives are worse
// than false-negatives because the deployment-default catches the
// unclassified case.
func TestClassifyContextDeploymentCues(t *testing.T) {
	for _, ctx := range []string{
		"user mid-bind on expert claims",
		"stuck on credentialism question",
		"trying to figure out next move",
	} {
		if got := ClassifyContext(ctx); got != StanceDeployment {
			t.Fatalf("ctx %q: got %s, want deployment", ctx, got)
		}
	}
}

// Design cues take precedence over deployment cues — if both fire,
// the user is in a design conversation about a deployment situation,
// and the elements' structure is what they need.
func TestClassifyContextDesignTakesPrecedence(t *testing.T) {
	if got := ClassifyContext("design while user is mid-bind"); got != StanceDesign {
		t.Fatalf("design+deployment cue: got %s, want design", got)
	}
}

// Load-bearing invariant: in deployment context, molecule must
// outrank atomic. Smoke test: lex-kebfa (molecule) vs lex-0001
// (atomic) under "mid-bind" context.
func TestRunDeploymentRanksMoleculeFirst(t *testing.T) {
	atom := mkEntry(withID("lex-0001"), withTier("atomic"))
	mol := mkEntry(withID("lex-kebfa"), withTier("molecule"))
	got := Run(Input{Pool: []*types.LexEntry{atom, mol}, Context: "user mid-bind on expert claims"})
	if got[0].PrimitiveID != "lex-kebfa" {
		t.Fatalf("expected molecule first, got %v", got)
	}
	if !strings.Contains(got[0].Reason, "stance:deployment") {
		t.Fatalf("expected deployment stance in reason, got %q", got[0].Reason)
	}
}

// Conversely: in design context, atom must outrank molecule. The
// asymmetry is exactly what fixes the original v0 finding.
func TestRunDesignRanksAtomFirst(t *testing.T) {
	atom := mkEntry(withID("lex-0001"), withTier("atomic"))
	mol := mkEntry(withID("lex-kebfa"), withTier("molecule"))
	got := Run(Input{Pool: []*types.LexEntry{atom, mol}, Context: "design conversation about decomposition"})
	if got[0].PrimitiveID != "lex-0001" {
		t.Fatalf("expected atom first, got %v", got)
	}
}

// sub-atomic entries are paraphrase-test elements; they should never
// surface to the user. Hard filter, not a low-score boost.
func TestRunFiltersSubAtomic(t *testing.T) {
	atom := mkEntry(withID("lex-0001"), withTier("atomic"))
	prime := mkEntry(withID("lex-4yhqs"), withTier("sub-atomic"))
	got := Run(Input{Pool: []*types.LexEntry{atom, prime}})
	for _, r := range got {
		if r.PrimitiveID == "lex-4yhqs" {
			t.Fatalf("sub-atomic leaked through: %v", got)
		}
	}
}

// Deprecated entries stay in the elements for provenance but must
// not surface to the user.
func TestRunFiltersDeprecated(t *testing.T) {
	live := mkEntry(withID("lex-0001"))
	dead := mkEntry(withID("lex-chp44"), withStatus("deprecated"))
	got := Run(Input{Pool: []*types.LexEntry{live, dead}})
	for _, r := range got {
		if r.PrimitiveID == "lex-chp44" {
			t.Fatalf("deprecated leaked through: %v", got)
		}
	}
}

// Vocab name-token match boosts score — deliberate, smaller than the
// tier asymmetry so vocab can't override the deployment-default. Uses
// token-aware matching so prompt single-words match against hyphenated
// entry handles.
func TestRunVocabNameMatchBoosts(t *testing.T) {
	a := mkEntry(withID("lex-0001"), withTier("atomic"), withName("alpha"))
	b := mkEntry(withID("lex-0002"), withTier("atomic"), withName("beta"))
	got := Run(Input{
		Pool: []*types.LexEntry{a, b}, Context: "design",
		WorkingVocab: []string{"alpha"},
	})
	if got[0].PrimitiveID != "lex-0001" {
		t.Fatalf("expected name-matched entry first, got %v", got)
	}
	if !strings.Contains(got[0].Reason, "name-token-in-vocab") {
		t.Fatalf("expected name-token-in-vocab reason, got %q", got[0].Reason)
	}
}

// Token-aware: prompt word "expert" matches hyphenated entry name
// "argument-from-expert-opinion". The hook depends on this — without
// it, every prompt produces the same top-k regardless of relevance.
func TestRunVocabTokenMatchInsideHyphenatedName(t *testing.T) {
	target := mkEntry(withID("lex-kebfa"), withTier("atomic"), withName("argument-from-expert-opinion"))
	other := mkEntry(withID("lex-z8m97"), withTier("atomic"), withName("gof-decorator"))
	got := Run(Input{
		Pool: []*types.LexEntry{target, other}, Context: "design",
		WorkingVocab: []string{"expert"},
	})
	if got[0].PrimitiveID != "lex-kebfa" {
		t.Fatalf("expected expert-token-matched entry first, got %v", got)
	}
	if !strings.Contains(got[0].Reason, "name-token-in-vocab") {
		t.Fatalf("expected name-token-in-vocab reason, got %q", got[0].Reason)
	}
}

// Stop-word-length guard: short tokens like "of"/"to"/"by" that
// hyphenated handles commonly contain shouldn't trigger matches against
// any common-word vocab. Otherwise every entry containing "of" matches
// every prompt with "of".
func TestRunVocabIgnoresShortTokens(t *testing.T) {
	target := mkEntry(withID("lex-0001"), withTier("atomic"), withName("argument-from-expert-opinion"))
	other := mkEntry(withID("lex-0002"), withTier("atomic"), withName("gof-decorator"))
	got := Run(Input{
		Pool: []*types.LexEntry{target, other}, Context: "design",
		WorkingVocab: []string{"of"}, // 2 chars — should not match
	})
	for _, r := range got {
		if strings.Contains(r.Reason, "token-in-vocab") {
			t.Fatalf("short token leaked through as match: %v", r)
		}
	}
}

// under-review applies a 0.7 multiplier so verified entries outrank
// provisional ones at the same tier.
func TestRunUnderReviewIsDownweighted(t *testing.T) {
	live := mkEntry(withID("lex-0001"))
	rev := mkEntry(withID("lex-0002"), withStatus("under-review"))
	got := Run(Input{Pool: []*types.LexEntry{live, rev}, Context: "design"})
	if got[0].PrimitiveID != "lex-0001" {
		t.Fatalf("expected active first, got %v", got)
	}
	if got[0].Score <= got[1].Score {
		t.Fatalf("expected active to outscore under-review, got %v", got)
	}
}

// TestRunDeterministicUnderInputReorder is the regression guard for the
// costean 2026-06-30 report: byte-identical input produced a different
// top-K every run. Root cause was tier×status scoring producing large tie
// groups whose order was decided by the caller's input order — and the
// read path feeds a Go map (randomized iteration). Run must now be a total
// function of the pool's CONTENT, not its enumeration order: the same
// entries in any order must yield the same ranked IDs, with ties broken by
// PrimitiveID ascending.
func TestRunDeterministicUnderInputReorder(t *testing.T) {
	// All same tier/status → all tie on score → order is pure ID tiebreak.
	mk := func(id string) *types.LexEntry { return mkEntry(withID(id)) }
	forward := []*types.LexEntry{mk("lex-0003"), mk("lex-0001"), mk("lex-0004"), mk("lex-0002")}
	reversed := []*types.LexEntry{mk("lex-0002"), mk("lex-0004"), mk("lex-0001"), mk("lex-0003")}

	ids := func(rs []types.GateResult) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = r.PrimitiveID
		}
		return out
	}
	gotF := ids(Run(Input{Pool: forward, Context: "design", TopK: 4}))
	gotR := ids(Run(Input{Pool: reversed, Context: "design", TopK: 4}))

	want := []string{"lex-0001", "lex-0002", "lex-0003", "lex-0004"}
	if strings.Join(gotF, ",") != strings.Join(want, ",") {
		t.Fatalf("forward order: got %v, want ID-ascending %v", gotF, want)
	}
	if strings.Join(gotR, ",") != strings.Join(gotF, ",") {
		t.Fatalf("reordered input changed ranking: %v vs %v", gotR, gotF)
	}
}

// --- Confidence-weighted scoring (lens.Confidences wiring) ---

// nil Confidences must be a byte-identical no-op — REQUIRED backward
// compatibility for every caller that never ran the lens.
func TestRunConfidenceNilMapNoOp(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	base, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	withNil, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	if base != withNil {
		t.Fatalf("nil Confidences changed score: %v vs %v", base, withNil)
	}
}

// A non-nil but empty Confidences map must behave identically to nil —
// not "confidence=0 for everyone."
func TestRunConfidenceEmptyMapNoOp(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	base, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	withEmpty, _ := scoreEntry(e, StanceDesign, nil, map[string]float64{}, nil)
	if base != withEmpty {
		t.Fatalf("empty Confidences map changed score: %v vs %v", base, withEmpty)
	}
}

// An entry whose ID is absent from a populated Confidences map must be a
// no-op too — isolates the `ok`-gated lookup specifically, not just "map
// is present."
func TestRunConfidenceMissingEntryIsNoOp(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	base, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	confs := map[string]float64{"lex-9999": 0.1}
	withOther, _ := scoreEntry(e, StanceDesign, nil, confs, nil)
	if base != withOther {
		t.Fatalf("confidence entry for a DIFFERENT ID changed this entry's score: %v vs %v", base, withOther)
	}
}

// Two otherwise-identical entries must rank by confidence when present —
// this is the whole point of the fix: the lens's own relevance judgment
// gets real weight instead of being discarded.
func TestRunConfidenceOrdersByWeight(t *testing.T) {
	a := mkEntry(withID("lex-0001"))
	b := mkEntry(withID("lex-0002"))
	got := Run(Input{
		Pool: []*types.LexEntry{a, b}, Context: "design",
		Confidences: map[string]float64{"lex-0001": 0.3, "lex-0002": 0.9},
	})
	if got[0].PrimitiveID != "lex-0002" {
		t.Fatalf("expected higher-confidence entry first, got %v", got)
	}
	if got[0].Score <= got[1].Score {
		t.Fatalf("expected strictly higher score for higher confidence, got %v", got)
	}
}

// confidence=0.0 explicitly present must not zero out an otherwise
// tier-appropriate atom — the floor guarantees a single bad lens
// judgment doesn't fully erase a candidate.
func TestRunConfidenceFloorPreventsZeroing(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	score, reason := scoreEntry(e, StanceDesign, nil, map[string]float64{"lex-0001": 0.0}, nil)
	if score <= 0 {
		t.Fatalf("confidence=0 zeroed the score entirely: %v (reason %q)", score, reason)
	}
}

// Out-of-range confidence values (from unvalidated lens JSON) must be
// clamped, not produce a score outside the valid weight range.
func TestRunConfidenceClampsOutOfRange(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	baseline, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	over, _ := scoreEntry(e, StanceDesign, nil, map[string]float64{"lex-0001": 1.5}, nil)
	under, _ := scoreEntry(e, StanceDesign, nil, map[string]float64{"lex-0001": -0.4}, nil)
	atOne, _ := scoreEntry(e, StanceDesign, nil, map[string]float64{"lex-0001": 1.0}, nil)
	atZero, _ := scoreEntry(e, StanceDesign, nil, map[string]float64{"lex-0001": 0.0}, nil)
	if over != atOne {
		t.Fatalf("confidence=1.5 not clamped to 1.0: got %v, want %v", over, atOne)
	}
	if under != atZero {
		t.Fatalf("confidence=-0.4 not clamped to 0.0: got %v, want %v", under, atZero)
	}
	if over > baseline || under > baseline {
		t.Fatalf("clamped scores exceeded the unweighted baseline: over=%v under=%v baseline=%v", over, under, baseline)
	}
}

// --- Frame-status-aware scoring (oracle-risk register wiring) ---

// A constitutive-classified atom must score strictly lower than an
// otherwise-identical unclassified one — down-weight, not exclusion.
func TestRunFrameStatusConstitutiveDownweighted(t *testing.T) {
	a := mkEntry(withID("lex-0001"))
	b := mkEntry(withID("lex-0002"))
	fs := framestatus.Map{"lex-0002": framestatus.Entry{Status: framestatus.Constitutive}}
	got := Run(Input{Pool: []*types.LexEntry{a, b}, Context: "design", FrameStatus: fs})
	if got[0].PrimitiveID != "lex-0001" {
		t.Fatalf("expected non-constitutive entry to rank first, got %v", got)
	}
	if got[0].Score <= got[1].Score {
		t.Fatalf("expected strictly higher score for the non-constitutive entry, got %v", got)
	}
	found := false
	for _, r := range got {
		if r.PrimitiveID == "lex-0002" {
			found = true
		}
	}
	if !found {
		t.Fatalf("constitutive entry was excluded entirely, not just down-weighted: %v", got)
	}
}

// Navigational, mixed, and unclassified atoms must all score identically
// to each other — only Constitutive triggers the discount.
func TestRunFrameStatusNavigationalMixedUnaffected(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	baseline, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	nav, _ := scoreEntry(e, StanceDesign, nil, nil, framestatus.Map{"lex-0001": framestatus.Entry{Status: framestatus.Navigational}})
	mixed, _ := scoreEntry(e, StanceDesign, nil, nil, framestatus.Map{"lex-0001": framestatus.Entry{Status: framestatus.Mixed}})
	if nav != baseline {
		t.Fatalf("navigational status changed score: got %v, want %v", nav, baseline)
	}
	if mixed != baseline {
		t.Fatalf("mixed status changed score: got %v, want %v", mixed, baseline)
	}
}

// FrameStatus nil, and FrameStatus present-but-lacking-the-ID, must both
// match the pre-change baseline.
func TestRunFrameStatusMissingIsNoOp(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	baseline, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	withNilMap, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	withOtherID, _ := scoreEntry(e, StanceDesign, nil, nil, framestatus.Map{"lex-9999": framestatus.Entry{Status: framestatus.Constitutive}})
	if withNilMap != baseline || withOtherID != baseline {
		t.Fatalf("missing/absent frame-status entry changed score: nil=%v other=%v baseline=%v", withNilMap, withOtherID, baseline)
	}
}

// Confidence and frame-status compose multiplicatively — pins the exact
// numeric product so a future change can't silently alter the
// compounding-discount behavior for an atom that's both low-confidence
// and constitutive.
func TestRunConfidenceAndFrameStatusComposeMultiplicatively(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	base, _ := scoreEntry(e, StanceDesign, nil, nil, nil)
	conf := 0.6
	fs := framestatus.Map{"lex-0001": framestatus.Entry{Status: framestatus.Constitutive}}
	got, _ := scoreEntry(e, StanceDesign, nil, map[string]float64{"lex-0001": conf}, fs)
	want := base * confidenceWeight(conf) * constitutiveDownweight
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("composed score = %v, want %v (base=%v, confWeight=%v, downweight=%v)",
			got, want, base, confidenceWeight(conf), constitutiveDownweight)
	}
}

// With both maps absent, the Reason string must contain neither new
// token — downstream code does strings.Contains(r.Reason,
// "token-in-vocab") matching that must stay unaffected in shape.
func TestRunReasonExcludesNewTokensWhenAbsent(t *testing.T) {
	e := mkEntry(withID("lex-0001"))
	_, reason := scoreEntry(e, StanceDesign, nil, nil, nil)
	if strings.Contains(reason, "lens-confidence") {
		t.Fatalf("reason contains lens-confidence token when Confidences absent: %q", reason)
	}
	if strings.Contains(reason, "constitutive-downweight") {
		t.Fatalf("reason contains constitutive-downweight token when FrameStatus absent: %q", reason)
	}
}

// Same determinism guard as TestRunDeterministicUnderInputReorder, but
// with Confidences/FrameStatus populated — confirms the new scoring
// factors don't reintroduce map-iteration nondeterminism.
func TestRunDeterministicUnderInputReorderWithNewFields(t *testing.T) {
	mk := func(id string) *types.LexEntry { return mkEntry(withID(id)) }
	forward := []*types.LexEntry{mk("lex-0003"), mk("lex-0001"), mk("lex-0004"), mk("lex-0002")}
	reversed := []*types.LexEntry{mk("lex-0002"), mk("lex-0004"), mk("lex-0001"), mk("lex-0003")}
	confs := map[string]float64{"lex-0001": 0.4, "lex-0003": 0.4}
	fs := framestatus.Map{"lex-0002": framestatus.Entry{Status: framestatus.Constitutive}}

	ids := func(rs []types.GateResult) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = r.PrimitiveID
		}
		return out
	}
	gotF := ids(Run(Input{Pool: forward, Context: "design", TopK: 4, Confidences: confs, FrameStatus: fs}))
	gotR := ids(Run(Input{Pool: reversed, Context: "design", TopK: 4, Confidences: confs, FrameStatus: fs}))
	if strings.Join(gotF, ",") != strings.Join(gotR, ",") {
		t.Fatalf("reordered input changed ranking with new fields populated: %v vs %v", gotR, gotF)
	}
}

func TestRunRespectsTopK(t *testing.T) {
	pool := []*types.LexEntry{
		mkEntry(withID("lex-0001")),
		mkEntry(withID("lex-0002")),
		mkEntry(withID("lex-0003")),
		mkEntry(withID("lex-0004")),
	}
	got := Run(Input{Pool: pool, TopK: 2})
	if len(got) != 2 {
		t.Fatalf("expected 2 results with topK=2, got %d", len(got))
	}
}
