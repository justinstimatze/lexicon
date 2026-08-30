package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Append must be JSONL — exactly one entry per line, parseable
// independently. Tools downstream (jq, future calibration analyzers)
// rely on this contract.
func TestAppendWritesJSONLLine(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "session-log.jsonl")
	entries := []types.SessionLogEntry{
		MakeEntry(types.TagUseful, 3, "first"),
		MakeEntry(types.TagAutonomous, 7, "second"),
	}
	for _, e := range entries {
		if err := Append(logPath, e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for i, line := range lines {
		var got types.SessionLogEntry
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d unparseable: %v", i, err)
		}
	}
}

// MakeEntry stamps today's date and copies through the other fields
// — the date format MUST be YYYY-MM-DD (matches winze cal_log
// convention; matches Q5-resolved schema).
func TestMakeEntryStampsDateFormat(t *testing.T) {
	e := MakeEntry(types.TagAutonomous, 0, "")
	if len(e.SessionDate) != 10 || e.SessionDate[4] != '-' || e.SessionDate[7] != '-' {
		t.Fatalf("session_date format wrong: %s", e.SessionDate)
	}
	if e.Vibe != types.TagAutonomous {
		t.Fatalf("vibe wrong: %s", e.Vibe)
	}
}

// Notes default-empty: omitted from JSON via omitempty, so consumers
// that rely on the absence-vs-empty-string distinction still work.
func TestAppendOmitsEmptyNotes(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "session-log.jsonl")
	if err := Append(logPath, MakeEntry(types.TagMixed, 1, "")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(logPath)
	if strings.Contains(string(data), `"notes"`) {
		t.Fatalf("expected notes field omitted, got %s", string(data))
	}
}
