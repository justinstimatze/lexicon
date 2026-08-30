package main

import (
	"errors"
	"testing"
	"time"
)

// TestLoadElementsWithTimeoutExceedsBudget uses a budget of 0 so the
// timer branch of the select is essentially guaranteed to win over the
// spawned goroutine, which still has to schedule and do a real ReadDir
// syscall before it can send anything. Regression guard for the
// error-loader-timeout path added alongside the loader stalls found in
// metrics.jsonl (t_substrate_ms up to 116985ms on this host).
func TestLoadElementsWithTimeoutExceedsBudget(t *testing.T) {
	_, err := loadElementsWithTimeout(t.TempDir(), 0)
	if err == nil {
		t.Fatal("expected a timeout error with a zero budget, got nil")
	}
	if !errors.Is(err, errLoadTimeout) {
		t.Fatalf("expected errLoadTimeout, got: %v", err)
	}
}

// TestLoadElementsWithTimeoutCompletesWithinBudget checks the non-timeout
// path returns the real underlying error (a missing directory) rather
// than errLoadTimeout when there's ample budget — the two outcomes are
// meant to be distinguishable (see error-loader vs error-loader-timeout
// in cmd_metrics.go's outcome enum).
func TestLoadElementsWithTimeoutCompletesWithinBudget(t *testing.T) {
	_, err := loadElementsWithTimeout("/nonexistent/path/that/does/not/exist", 5*time.Second)
	if err == nil {
		t.Fatal("expected a readdir error for a nonexistent path, got nil")
	}
	if errors.Is(err, errLoadTimeout) {
		t.Fatalf("a fast failure on a missing dir shouldn't read as a timeout, got: %v", err)
	}
}
