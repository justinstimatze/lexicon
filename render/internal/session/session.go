// Package session owns session-log.jsonl — the per-session gut-check
// log per Q5 of render-function-design-v0.md.
//
// Q5 lesson (load-bearing — read carefully): per-session gut-check
// only, no per-call instrumentation, no per-entry rollup. Winze fell
// into the trap of building substantial measurement infrastructure
// that became its own thing rather than serving the use-loop. v0
// answers exactly one question: "is this thing helping me think, or
// not?" Per-call counterfactual scoring waits for v0.5+ if (and only
// if) the gut-check surfaces consistent mixed/not-useful signal.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// DefaultLogPath is session-log.jsonl next to the render binary.
// Gitignored per render/.gitignore — per-machine artifact, not for
// sharing.
var DefaultLogPath = "session-log.jsonl"

// MakeEntry stamps the current date and assembles a SessionLogEntry.
// Caller validates the vibe before calling (use types.ValidSessionTag).
func MakeEntry(vibe types.SessionTag, renderCallCount int, notes string) types.SessionLogEntry {
	return types.SessionLogEntry{
		SessionDate:     time.Now().Format("2006-01-02"),
		RenderCallCount: renderCallCount,
		Vibe:            vibe,
		Notes:           notes,
	}
}

// Append writes one JSONL line to logPath, creating the parent dir if
// missing. Append is O_APPEND-atomic for line writes shorter than
// PIPE_BUF (4096 on Linux); session entries fit comfortably.
func Append(logPath string, entry types.SessionLogEntry) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("session: mkdir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("session: open %s: %w", logPath, err)
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("session: write: %w", err)
	}
	return nil
}
