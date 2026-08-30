package lexicon

import (
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/embedgate"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// TestDecideEmbedNarrowingAlwaysNarrowsWhenGateHasResults locks in the
// V209 policy: any non-empty gateRes narrows, whether or not the top
// score clears threshold. Regression guard against reverting to V208
// (below-threshold falls through to the full pool) without noticing —
// exactly the kind of change the earlier version shipped with no test
// covering it at all.
func TestDecideEmbedNarrowingAlwaysNarrowsWhenGateHasResults(t *testing.T) {
	pool := map[string]*types.LexEntry{
		"lex-aaaaa": {ID: "lex-aaaaa", Name: "top-pick"},
		"lex-bbbbb": {ID: "lex-bbbbb", Name: "second-pick"},
	}
	gateRes := []embedgate.Result{
		{AtomID: "lex-aaaaa", Score: 0.55}, // below a 0.61-style threshold
		{AtomID: "lex-bbbbb", Score: 0.50},
	}

	narrowed, active, logMsg := DecideEmbedNarrowing(gateRes, pool, 0.61, 1088)

	if !active {
		t.Fatal("expected active=true for a below-threshold but non-empty gateRes (V209)")
	}
	if len(narrowed) != 2 {
		t.Fatalf("expected both atoms narrowed to, got %d", len(narrowed))
	}
	if narrowed[0].ID != "lex-aaaaa" {
		t.Fatalf("expected narrowed order to preserve gateRes order (highest score first), got %s first", narrowed[0].ID)
	}
	if !strings.Contains(logMsg, "narrowing anyway") {
		t.Fatalf("expected the below-threshold log line to say so, got: %s", logMsg)
	}
}

// TestDecideEmbedNarrowingAboveThresholdLogsDifferently checks the log
// line changes (not the behavior) when the top score clears threshold —
// narrowing itself is identical either way under V209.
func TestDecideEmbedNarrowingAboveThresholdLogsDifferently(t *testing.T) {
	pool := map[string]*types.LexEntry{
		"lex-aaaaa": {ID: "lex-aaaaa", Name: "top-pick"},
	}
	gateRes := []embedgate.Result{{AtomID: "lex-aaaaa", Score: 0.9}}

	narrowed, active, logMsg := DecideEmbedNarrowing(gateRes, pool, 0.61, 1088)

	if !active || len(narrowed) != 1 {
		t.Fatalf("expected narrowed to the one above-threshold result, got active=%v narrowed=%d", active, len(narrowed))
	}
	if strings.Contains(logMsg, "narrowing anyway") {
		t.Fatalf("above-threshold log line shouldn't say 'narrowing anyway', got: %s", logMsg)
	}
	if !strings.Contains(logMsg, "narrowed") {
		t.Fatalf("expected the ordinary narrow log line, got: %s", logMsg)
	}
}

// TestDecideEmbedNarrowingEmptyGateRes covers the (defensive) empty-slice
// case — the real caller only reaches this function inside an
// `else if len(gateRes) > 0` branch, but the function shouldn't assume
// that guard exists elsewhere, since a future caller might not preserve it.
func TestDecideEmbedNarrowingEmptyGateRes(t *testing.T) {
	narrowed, active, _ := DecideEmbedNarrowing(nil, map[string]*types.LexEntry{}, 0.61, 1088)
	if active {
		t.Fatal("expected active=false for empty gateRes")
	}
	if narrowed != nil {
		t.Fatalf("expected nil narrowed for empty gateRes, got %v", narrowed)
	}
}

// TestDecideEmbedNarrowingSkipsStaleAtomIDs covers a prototype cache
// referencing an atom no longer in pool (e.g. a deleted/renamed atom
// whose embedding was cached before the change) — the result should
// silently skip it rather than narrow to a shorter, wrong-length slice
// with a nil-shaped gap.
func TestDecideEmbedNarrowingSkipsStaleAtomIDs(t *testing.T) {
	pool := map[string]*types.LexEntry{
		"lex-aaaaa": {ID: "lex-aaaaa", Name: "still-here"},
	}
	gateRes := []embedgate.Result{
		{AtomID: "lex-aaaaa", Score: 0.9},
		{AtomID: "lex-zzzzz", Score: 0.8}, // stale — not in pool
	}

	narrowed, active, _ := DecideEmbedNarrowing(gateRes, pool, 0.61, 1088)

	if !active {
		t.Fatal("expected active=true")
	}
	if len(narrowed) != 1 || narrowed[0].ID != "lex-aaaaa" {
		t.Fatalf("expected the stale id silently skipped, got %d entries: %v", len(narrowed), narrowed)
	}
}
