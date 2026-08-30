package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRecentUserContext verifies the V71 transcript context-accumulation:
// it picks recent genuine user turns (string content), skips tool-result/
// image turns (list content), assistant turns, and system events; honors
// the turns window; never duplicates the current prompt; and fails soft
// (turns=0 or empty path → unchanged).
func TestRecentUserContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"first situation about X"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"assistant reply"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"tool output noise"}]}}`,
		`{"type":"user","message":{"role":"user","content":"<task-notification>bg done</task-notification>"}}`,
		`{"type":"user","message":{"role":"user","content":"second situation about Y"}}`,
		`{"type":"user","message":{"role":"user","content":"current prompt Z"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := recentUserContext(path, "current prompt Z", 0); got != "current prompt Z" {
		t.Errorf("turns=0 should be unchanged; got %q", got)
	}
	if got := recentUserContext("", "current prompt Z", 3); got != "current prompt Z" {
		t.Errorf("empty path should be unchanged; got %q", got)
	}

	got := recentUserContext(path, "current prompt Z", 2)
	for _, want := range []string{"first situation about X", "second situation about Y", "current prompt Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("turns=2 missing %q; got %q", want, got)
		}
	}
	for _, noise := range []string{"tool output noise", "task-notification", "assistant reply"} {
		if strings.Contains(got, noise) {
			t.Errorf("turns=2 should skip %q; got %q", noise, got)
		}
	}
	if strings.Count(got, "current prompt Z") != 1 {
		t.Errorf("current prompt should appear exactly once; got %q", got)
	}

	got1 := recentUserContext(path, "current prompt Z", 1)
	if strings.Contains(got1, "first situation about X") {
		t.Errorf("turns=1 should drop the older turn; got %q", got1)
	}
	if !strings.Contains(got1, "second situation about Y") {
		t.Errorf("turns=1 should keep the most recent prior turn; got %q", got1)
	}
}
