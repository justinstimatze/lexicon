package main

// Per-hook-call metrics. One line per `lexicon hook` invocation,
// regardless of fire/silence outcome — so `lexicon doctor` can report
// total cost (lens calls × $/call), latency p50/p95, and the silenced-
// call cost that fires.jsonl can't show. Append-only, JSONL.
//
// Designed to fail-soft: every error path swallows. The hook never
// blocks on a metrics-write failure.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// metricsRecord is one hook invocation's full perf-and-outcome snapshot.
// Written to ~/.claude/lexicon/metrics.jsonl by writeMetricsRecord.
//
// Outcome enum (in order they can occur):
//   - "skipped-env"          LEXICON_SKIP=1 short-circuit
//   - "skipped-empty-prompt" stdin had whitespace-only prompt
//   - "skipped-system-event"  V54.2: stdin was a Claude Code system event
//                             (task-notification) not user input
//   - "error-stdin"          JSON decode of stdin failed
//   - "error-loader"         elements load failed (bad path, unreadable dir/file)
//   - "error-loader-timeout" elements load was still running when
//                            LEXICON_ELEMENTS_LOAD_BUDGET_MS ran out —
//                            distinct from error-loader: this is disk/
//                            memory pressure, not a config problem
//   - "silence-lens-empty"   lens returned [] (nothing relevant)
//   - "silence-conf"         lens top conf < LEXICON_LENS_MIN_CONFIDENCE
//   - "silence-threshold"    gate top score < threshold (lens-disabled path)
//   - "silence-embed-gate"   V14: embedding gate top sim < threshold (no lens call)
//   - "silence-empty-context" injection text empty (degenerate case)
//   - "fire"                 normal fire; injection emitted
//   - "panic"                hook panicked (unreachable in normal flow)
type metricsRecord struct {
	Ts          string  `json:"ts"`
	SessionID   string  `json:"session_id,omitempty"`
	Outcome     string  `json:"outcome"`
	TotalMs     int64   `json:"t_total_ms"`
	ElementsMs int64   `json:"t_elements_ms,omitempty"`
	EmbedGateMs int64   `json:"t_embed_gate_ms,omitempty"` // V14: embedding pre-filter
	EmbedActive bool    `json:"embed_active,omitempty"`    // true when embed gate narrowed the pool
	EmbedTopSim float64 `json:"embed_top_sim,omitempty"`   // cosine of top match
	LensMs      int64   `json:"t_lens_ms,omitempty"`
	GateMs      int64   `json:"t_gate_ms,omitempty"`
	PoolSize    int     `json:"pool_size,omitempty"`
	LensCalled  bool    `json:"lens_called"`
	LensTopConf float64 `json:"lens_top_conf,omitempty"`
	HookEventID string  `json:"hook_event_id,omitempty"` // joins to fires.jsonl on fire
	// V13 TASK 4: prompt-cache + token accounting (zero when lens
	// didn't run, lens errored before token return, or model didn't
	// honor the cache_control hint).
	InputTokens         int64 `json:"input_tokens,omitempty"`
	OutputTokens        int64 `json:"output_tokens,omitempty"`
	CacheReadTokens     int64 `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int64 `json:"cache_creation_tokens,omitempty"`
}

// metricsDisabled returns true iff metrics writing should be skipped.
// Defaults to enabled; flip via LEXICON_METRICS_DISABLED=1.
func metricsDisabled() bool {
	return os.Getenv("LEXICON_METRICS_DISABLED") == "1"
}

// writeMetricsRecord appends one record to ~/.claude/lexicon/metrics.jsonl.
// Silent on error — perf instrumentation must never break the hook.
func writeMetricsRecord(rec metricsRecord) {
	if metricsDisabled() {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".claude", "lexicon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	path := filepath.Join(dir, "metrics.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	fmt.Fprintln(f, string(data))
}

// nowTs returns an RFC3339Nano UTC timestamp string.
func nowTs() string { return time.Now().UTC().Format(time.RFC3339Nano) }
