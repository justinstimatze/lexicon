package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These cover what recentUserContext gained when it stopped scanning the whole
// transcript forward and started walking it backward: correctness across the
// chunk boundary the backward reader introduces, and the bound that was the
// point of the change.

func writeJSONL(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return path
}

func userLine(s string) string { return fmt.Sprintf(`{"type":"user","message":{"content":%q}}`, s) }
func asstLine(s string) string {
	return fmt.Sprintf(`{"type":"assistant","message":{"content":%q}}`, s)
}
func toolLine() string { return `{"type":"user","message":{"content":[{"type":"tool_result"}]}}` }
func padLines(n int, w int) []string {
	pad := strings.Repeat("z", w)
	out := make([]string, n)
	for i := range out {
		out[i] = asstLine(pad)
	}
	return out
}

// The wanted turns sit at the end of a transcript far larger than one chunk.
func TestRecentUserContextAcrossChunkBoundary(t *testing.T) {
	var lines []string
	lines = append(lines, userLine("ancient and should not appear"))
	lines = append(lines, padLines(tailChunk/1000*3, 1000)...) // ~3 chunks of filler
	lines = append(lines, userLine("older wanted"), toolLine(), asstLine("noise"), userLine("newer wanted"))
	path := writeJSONL(t, lines)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= tailChunk {
		t.Fatalf("fixture is %d bytes, needs to exceed tailChunk (%d)", info.Size(), tailChunk)
	}

	got := recentUserContext(path, "current", 2)
	for _, want := range []string{"older wanted", "newer wanted", "current"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "ancient") {
		t.Errorf("turns=2 should not reach the first message; got %q", got)
	}
}

// File order must survive the backward walk: oldest first, current prompt last.
func TestRecentUserContextPreservesOrder(t *testing.T) {
	lines := []string{userLine("one"), userLine("two"), userLine("three")}
	lines = append(padLines(tailChunk/500*2, 500), lines...) // push past a chunk
	got := recentUserContext(writeJSONL(t, lines), "four", 3)

	var idx []int
	for _, s := range []string{"one", "two", "three", "four"} {
		i := strings.Index(got, s)
		if i < 0 {
			t.Fatalf("missing %q in %q", s, got)
		}
		idx = append(idx, i)
	}
	for i := 1; i < len(idx); i++ {
		if idx[i] <= idx[i-1] {
			t.Errorf("out of order at %d: %q", i, got)
		}
	}
}

// A record longer than one chunk has to be rejoined from the carry, not
// silently dropped or truncated.
func TestRecentUserContextLineSpanningChunks(t *testing.T) {
	huge := strings.Repeat("q", tailChunk+4096)
	lines := append(padLines(200, 1000), userLine(huge))
	got := recentUserContext(writeJSONL(t, lines), "current", 1)
	if !strings.Contains(got, huge) {
		t.Errorf("a record spanning a chunk boundary was lost (got %d bytes)", len(got))
	}
}

// The reason the function was rewritten: it used to read every byte of a
// multi-GB transcript to keep the last few messages.
func TestRecentUserContextReadIsBounded(t *testing.T) {
	lines := append(padLines(4000, 2000), userLine("the only one wanted"))
	path := writeJSONL(t, lines)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 4<<20 {
		t.Fatalf("fixture too small to be meaningful: %d bytes", info.Size())
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	got := recentUserContext(path, "current", 1)
	runtime.ReadMemStats(&after)

	if !strings.Contains(got, "the only one wanted") {
		t.Fatalf("did not find the trailing message; got %q", got)
	}
	allocated := after.TotalAlloc - before.TotalAlloc
	budget := uint64(info.Size() / 4)
	t.Logf("allocated %d bytes for a %d byte transcript (budget %d)", allocated, info.Size(), budget)
	if allocated > budget {
		t.Errorf("allocated %d bytes reading a %d byte transcript (budget %d) — "+
			"the full-file scan is back", allocated, info.Size(), budget)
	}
}

// The current prompt is dropped only when it is the newest user message, not
// wherever it happens to appear earlier in the session.
func TestRecentUserContextDropsOnlyTrailingDuplicate(t *testing.T) {
	lines := []string{userLine("repeat me"), userLine("something else")}
	got := recentUserContext(writeJSONL(t, lines), "repeat me", 3)
	if strings.Count(got, "repeat me") != 2 {
		t.Errorf("an earlier identical message should survive; got %q", got)
	}

	lines = []string{userLine("something else"), userLine("repeat me")}
	got = recentUserContext(writeJSONL(t, lines), "repeat me", 3)
	if strings.Count(got, "repeat me") != 1 {
		t.Errorf("the trailing duplicate should be dropped; got %q", got)
	}
}
