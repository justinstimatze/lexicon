package assembly

import (
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// fixture builds a tiny elements where atom types are known so the
// type-checker has something to validate against. Ids here are entirely
// synthetic (never real elements atoms), shaped to satisfy IsLexID's
// post-2026-08-20 5-char format so the typechecker still recognizes them
// as id-shaped leaves rather than bare names.
func fixture() map[string]*types.LexEntry {
	return map[string]*types.LexEntry{
		"lex-fake2": {ID: "lex-fake2", TypeIn: "claim", TypeOut: "frame"},
		"lex-fake3": {ID: "lex-fake3", TypeIn: "frame", TypeOut: "posture"},
		"lex-fake4": {ID: "lex-fake4", TypeIn: "claim", TypeOut: "posture"}, // bad fit after lex-fake3
		"lex-4yhqs": {ID: "lex-4yhqs", TypeIn: "claim", TypeOut: "claim"},    // fixed-point shape
		"lex-chp44": {ID: "lex-chp44", TypeIn: "claim", TypeOut: "question"}, // not fixed-point
		"lex-fake5": {ID: "lex-fake5", TypeIn: "state", TypeOut: "process"},
		"lex-fake6": {ID: "lex-fake6", TypeIn: "claim", TypeOut: "process"}, // wrong type-in for parallel sibling of fake5
	}
}

func tcAssemble(t *testing.T, src string) (Node, *types.LexEntry, map[string]*types.LexEntry) {
	t.Helper()
	n, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	host := &types.LexEntry{ID: "lex-fake9"}
	return n, host, fixture()
}

func diagSummary(diags []Diagnostic) []string {
	out := make([]string, len(diags))
	for i, d := range diags {
		out[i] = d.Severity + ":" + d.Code
	}
	return out
}

func TestSequentialOK(t *testing.T) {
	n, host, sub := tcAssemble(t, "sequential(lex-fake2, lex-fake3)")
	d := TypeCheck(n, host, sub)
	for _, x := range d {
		if x.Severity == "error" {
			t.Errorf("unexpected error: %v", x)
		}
	}
}

func TestSequentialMismatch(t *testing.T) {
	// lex-fake2 type-out=frame, lex-fake4 type-in=claim → mismatch
	n, host, sub := tcAssemble(t, "sequential(lex-fake2, lex-fake4)")
	d := TypeCheck(n, host, sub)
	got := diagSummary(d)
	hasMismatch := false
	for _, s := range got {
		if s == "error:type-mismatch" {
			hasMismatch = true
		}
	}
	if !hasMismatch {
		t.Errorf("expected type-mismatch error, got %v", got)
	}
}

func TestParallelMismatch(t *testing.T) {
	// lex-fake5 type-in=state, lex-fake6 type-in=claim → mismatch
	n, host, sub := tcAssemble(t, "parallel(lex-fake5, lex-fake6)")
	d := TypeCheck(n, host, sub)
	hasMismatch := false
	for _, x := range d {
		if x.Code == "parallel-input-mismatch" {
			hasMismatch = true
		}
	}
	if !hasMismatch {
		t.Errorf("expected parallel-input-mismatch, got %v", diagSummary(d))
	}
}

func TestIterationFixedPoint(t *testing.T) {
	// lex-4yhqs is claim→claim (ok), lex-chp44 is claim→question (not ok)
	n, host, sub := tcAssemble(t, "iteration(lex-4yhqs)")
	d := TypeCheck(n, host, sub)
	for _, x := range d {
		if x.Code == "iteration-not-fixed-point" {
			t.Errorf("did not expect not-fixed-point on lex-4yhqs: %v", x)
		}
	}
	n2, host2, sub2 := tcAssemble(t, "iteration(lex-chp44)")
	d2 := TypeCheck(n2, host2, sub2)
	hasWarn := false
	for _, x := range d2 {
		if x.Code == "iteration-not-fixed-point" {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected iteration-not-fixed-point on lex-chp44, got %v", diagSummary(d2))
	}
}

func TestUnresolvableAtomWarn(t *testing.T) {
	n, host, sub := tcAssemble(t, "sequential(lex-fake2, lex-fake7)")
	d := TypeCheck(n, host, sub)
	hasWarn := false
	for _, x := range d {
		if x.Code == "unresolvable-atom" && x.Severity == "warning" {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected unresolvable-atom warning, got %v", diagSummary(d))
	}
}

func TestBareNameInStepLabelOpSuppressed(t *testing.T) {
	// Bare-name children of sequential() and classification() are
	// intentional tactic-internal step labels — NOT unresolved
	// elements-atom references. No warning should fire.
	n, host, sub := tcAssemble(t, "sequential(lex-fake2, solve-toy-version)")
	d := TypeCheck(n, host, sub)
	for _, x := range d {
		if x.Code == "unresolvable-atom" {
			t.Errorf("expected NO unresolvable-atom warning inside sequential(), got %v", diagSummary(d))
		}
	}
}

func TestBareNameOutsideStepLabelOpWarns(t *testing.T) {
	// Bare-name children of other ops (e.g. parallel) are still
	// flagged as forcing-functions.
	n, host, sub := tcAssemble(t, "parallel(lex-fake2, solve-toy-version)")
	d := TypeCheck(n, host, sub)
	hasWarn := false
	for _, x := range d {
		if x.Code == "unresolvable-atom" {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected unresolvable-atom for bare-name inside parallel(), got %v", diagSummary(d))
	}
}

func TestArrowSugarTypeFlow(t *testing.T) {
	// "lex-fake2 → lex-fake3" normalizes to sequential(lex-fake2, lex-fake3)
	// and the type-flow is checked end-to-end.
	n, host, sub := tcAssemble(t, "lex-fake2 → lex-fake3")
	d := TypeCheck(n, host, sub)
	for _, x := range d {
		if x.Severity == "error" {
			t.Errorf("unexpected error on well-formed → chain: %v", x)
		}
	}
}
