package main

// `lexicon doctor` — observability snapshot for the production hook.
// Reads ~/.claude/lexicon/{metrics.jsonl, fires.jsonl, hook.log} and
// reports file growth, hook latency distribution, outcome breakdown,
// and a small set of alerts that catch silent degradation (lens
// disabled, file getting big, p95 approaching timeout cap).
//
// Built around the slimemold-200MB lesson: surface the data growth
// before it becomes a problem, surface the degradation before the user
// notices it. Read-only; no daemon, no rotation, no background work.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultDoctorWindow      = 100  // most-recent calls considered for latency rollups
	doctorCostPerCall        = 0.0012 // $/call legacy estimate; used only when no token data exists
	doctorAlertFiresMB       = 50.0
	doctorAlertMetricsMB     = 10.0
	doctorAlertP95Sec        = 7.0  // alert when total p95 exceeds this (within 1s of 8s lens timeout)
	doctorAlertLensDownPct   = 50
	doctorCostWindowDuration = 30 * 24 * time.Hour

	// Haiku 4.5 pricing as of 2025/2026 (per-token, USD).
	// Source: https://www.anthropic.com/pricing — Haiku 4.5 = $1/MTok in / $5/MTok out;
	// ephemeral 5min cache = 1.25x input (write) and 0.10x input (read).
	priceHaikuInput         = 1.0 / 1_000_000      // $0.000001 per input token
	priceHaikuOutput        = 5.0 / 1_000_000      // $0.000005 per output token
	priceHaikuCacheWrite5m  = 1.25 / 1_000_000     // $0.00000125 per cache-creation token
	priceHaikuCacheRead     = 0.10 / 1_000_000     // $0.00000010 per cache-read token
)

func cmdDoctor(renderDir string, args []string) {
	fl := flag.NewFlagSet("doctor", flag.ExitOnError)
	jsonOut := fl.Bool("json", false, "machine-readable JSON output")
	window := fl.Int("last", defaultDoctorWindow, "use this many most-recent metrics records for latency rollups")
	_ = fl.Parse(args)

	home, err := os.UserHomeDir()
	if err != nil {
		fatal("home dir: %v", err)
	}
	lexDir := filepath.Join(home, ".claude", "lexicon")

	stats := collectDoctorStats(lexDir, renderDir, *window)
	if *jsonOut {
		data, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(data))
		return
	}
	fmt.Print(formatDoctorReport(stats))
}

type doctorStats struct {
	GeneratedAt   string             `json:"generated_at"`
	LexiconDir    string             `json:"lexicon_dir"`
	Files         []doctorFileStat   `json:"files"`
	ElementsDir  string             `json:"elements_dir"`
	ElementsN    int                `json:"elements_entries"`
	Window        int                `json:"window"`
	Outcomes      map[string]int     `json:"outcomes"`
	LatencyTotal  doctorLatencyStat  `json:"latency_total_ms"`
	LatencyLens   doctorLatencyStat  `json:"latency_lens_ms"`
	LatencySubs   doctorLatencyStat  `json:"latency_elements_ms"`
	LatencyGate   doctorLatencyStat  `json:"latency_gate_ms"`
	LensCalledN   int                `json:"lens_called_n"`
	LensTotal     int                `json:"lens_total"`
	CostEstimate  doctorCostEstimate `json:"cost_estimate"`
	Configuration map[string]string  `json:"configuration"`
	Alerts        []string           `json:"alerts"`
}

type doctorFileStat struct {
	Path             string `json:"path"`
	Bytes            int64  `json:"bytes"`
	Lines            int    `json:"lines"`
	Bytes7d          int64  `json:"bytes_last_7d,omitempty"`
	Lines7d          int    `json:"lines_last_7d,omitempty"`
	OldestTs         string `json:"oldest_ts,omitempty"`
	NewestTs         string `json:"newest_ts,omitempty"`
	HasTimestampedTs bool   `json:"-"` // internal: did we find ts fields?
}

type doctorLatencyStat struct {
	N   int   `json:"n"`
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
	Max int64 `json:"max"`
}

type doctorCostEstimate struct {
	WindowDays          int     `json:"window_days"`
	LensCalls           int     `json:"lens_calls"`
	CacheHits           int     `json:"cache_hits"`           // V13: calls where CacheReadTokens > 0
	CacheHitRatePct     int     `json:"cache_hit_rate_pct"`   // V13: 100 * CacheHits / LensCalls
	InputTokensTotal    int64   `json:"input_tokens_total"`
	OutputTokensTotal   int64   `json:"output_tokens_total"`
	CacheReadTotal      int64   `json:"cache_read_tokens_total"`
	CacheCreationTotal  int64   `json:"cache_creation_tokens_total"`
	USD                 float64 `json:"usd"`
	USDExact            bool    `json:"usd_exact"` // true when computed from token counts
	Note                string  `json:"note"`
}

func collectDoctorStats(lexDir, renderDir string, window int) doctorStats {
	stats := doctorStats{
		GeneratedAt: nowTs(),
		LexiconDir:  lexDir,
		Window:      window,
		Outcomes:    map[string]int{},
	}

	for _, name := range []string{"metrics.jsonl", "fires.jsonl", "hook.log"} {
		path := filepath.Join(lexDir, name)
		fs := statFile(path)
		stats.Files = append(stats.Files, fs)
	}

	// Elements: just count *.yaml entries.
	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, "..", "elements")
	}
	stats.ElementsDir = elementsDir
	if entries, err := os.ReadDir(elementsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				stats.ElementsN++
			}
		}
	}

	// Latency + outcome rollups from metrics.jsonl.
	metricsPath := filepath.Join(lexDir, "metrics.jsonl")
	records := tailMetricsRecords(metricsPath, window)
	rollupMetrics(records, &stats)

	// Cost over last 30 days.
	stats.CostEstimate = estimateCost(metricsPath, doctorCostWindowDuration)

	// Configuration snapshot — uses the same resolveX helpers the hook
	// uses, so the doctor and the hook agree.
	stats.Configuration = map[string]string{
		"hook_threshold":      fmt.Sprintf("%.2f", resolveHookThreshold()),
		"hook_top_k":          fmt.Sprintf("%d", resolveHookTopK()),
		"lens_min_confidence": fmt.Sprintf("%.2f", resolveLensMinConfidence()),
		"lens_model":          "claude-haiku-4-5-20251001",
		"metrics_enabled":     fmt.Sprintf("%t", !metricsDisabled()),
	}

	stats.Alerts = computeAlerts(stats)
	return stats
}

func statFile(path string) doctorFileStat {
	fs := doctorFileStat{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		return fs // missing file → zero-valued stat
	}
	fs.Bytes = info.Size()
	// Count lines + grab oldest/newest ts when available (JSONL only).
	f, err := os.Open(path)
	if err != nil {
		return fs
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var bytes7d int64
	var lines7d int
	for scanner.Scan() {
		fs.Lines++
		line := scanner.Bytes()
		// Try to parse as JSONL with a ts field.
		var probe struct {
			Ts string `json:"ts"`
		}
		if err := json.Unmarshal(line, &probe); err == nil && probe.Ts != "" {
			fs.HasTimestampedTs = true
			if fs.OldestTs == "" {
				fs.OldestTs = probe.Ts
			}
			fs.NewestTs = probe.Ts
			if t, err := time.Parse(time.RFC3339Nano, probe.Ts); err == nil && t.After(cutoff) {
				bytes7d += int64(len(line)) + 1 // +1 for \n
				lines7d++
			}
		}
	}
	if fs.HasTimestampedTs {
		fs.Bytes7d = bytes7d
		fs.Lines7d = lines7d
	}
	return fs
}

func tailMetricsRecords(path string, window int) []metricsRecord {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	all := []metricsRecord{}
	for scanner.Scan() {
		var r metricsRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		all = append(all, r)
	}
	if window > 0 && len(all) > window {
		all = all[len(all)-window:]
	}
	return all
}

func rollupMetrics(records []metricsRecord, stats *doctorStats) {
	totals := []int64{}
	lensLat := []int64{}
	subsLat := []int64{}
	gateLat := []int64{}
	for _, r := range records {
		stats.Outcomes[r.Outcome]++
		stats.LensTotal++
		if r.LensCalled {
			stats.LensCalledN++
		}
		if r.TotalMs > 0 {
			totals = append(totals, r.TotalMs)
		}
		if r.LensCalled && r.LensMs > 0 {
			lensLat = append(lensLat, r.LensMs)
		}
		if r.ElementsMs > 0 {
			subsLat = append(subsLat, r.ElementsMs)
		}
		if r.GateMs > 0 {
			gateLat = append(gateLat, r.GateMs)
		}
	}
	stats.LatencyTotal = pctile(totals)
	stats.LatencyLens = pctile(lensLat)
	stats.LatencySubs = pctile(subsLat)
	stats.LatencyGate = pctile(gateLat)
}

func pctile(xs []int64) doctorLatencyStat {
	out := doctorLatencyStat{N: len(xs)}
	if len(xs) == 0 {
		return out
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	out.P50 = xs[len(xs)/2]
	idx95 := int(float64(len(xs)) * 0.95)
	if idx95 >= len(xs) {
		idx95 = len(xs) - 1
	}
	out.P95 = xs[idx95]
	out.Max = xs[len(xs)-1]
	return out
}

func estimateCost(path string, window time.Duration) doctorCostEstimate {
	out := doctorCostEstimate{
		WindowDays: int(window / (24 * time.Hour)),
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	cutoff := time.Now().Add(-window)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	tokensSeen := false
	for scanner.Scan() {
		var r metricsRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		if !r.LensCalled {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, r.Ts)
		if err != nil || t.Before(cutoff) {
			continue
		}
		out.LensCalls++
		if r.InputTokens+r.OutputTokens+r.CacheReadTokens+r.CacheCreationTokens > 0 {
			tokensSeen = true
		}
		if r.CacheReadTokens > 0 {
			out.CacheHits++
		}
		out.InputTokensTotal += r.InputTokens
		out.OutputTokensTotal += r.OutputTokens
		out.CacheReadTotal += r.CacheReadTokens
		out.CacheCreationTotal += r.CacheCreationTokens
	}
	if tokensSeen {
		out.USD = float64(out.InputTokensTotal)*priceHaikuInput +
			float64(out.OutputTokensTotal)*priceHaikuOutput +
			float64(out.CacheReadTotal)*priceHaikuCacheRead +
			float64(out.CacheCreationTotal)*priceHaikuCacheWrite5m
		out.USDExact = true
		out.Note = "exact (computed from per-call token counts)"
	} else {
		out.USD = float64(out.LensCalls) * doctorCostPerCall
		out.Note = "estimate (no token data in window; pre-TASK-4 calls)"
	}
	if out.LensCalls > 0 {
		out.CacheHitRatePct = 100 * out.CacheHits / out.LensCalls
	}
	return out
}

func computeAlerts(s doctorStats) []string {
	var alerts []string
	for _, f := range s.Files {
		mb := float64(f.Bytes) / (1024 * 1024)
		base := filepath.Base(f.Path)
		switch base {
		case "fires.jsonl":
			if mb > doctorAlertFiresMB {
				alerts = append(alerts, fmt.Sprintf("fires.jsonl is %.1f MB (> %.0f MB) — consider rotating", mb, doctorAlertFiresMB))
			}
		case "metrics.jsonl":
			if mb > doctorAlertMetricsMB {
				alerts = append(alerts, fmt.Sprintf("metrics.jsonl is %.1f MB (> %.0f MB) — consider `head -n -10000 metrics.jsonl > tmp && mv tmp metrics.jsonl`", mb, doctorAlertMetricsMB))
			}
		}
	}
	if s.LatencyTotal.N > 0 {
		p95s := float64(s.LatencyTotal.P95) / 1000
		if p95s > doctorAlertP95Sec {
			alerts = append(alerts, fmt.Sprintf("hook total p95 is %.1fs (> %.0fs; lens timeout is 8s) — investigate", p95s, doctorAlertP95Sec))
		}
	}
	if s.LensTotal > 0 {
		downPct := 100 - (100 * s.LensCalledN / s.LensTotal)
		if downPct > doctorAlertLensDownPct {
			alerts = append(alerts, fmt.Sprintf("lens disabled in %d%% of last %d calls — check ANTHROPIC_API_KEY and ~/.claude/lexicon/.env", downPct, s.LensTotal))
		}
	}
	return alerts
}

func formatDoctorReport(s doctorStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "lexicon doctor — observability snapshot (%s)\n\n", s.GeneratedAt)

	fmt.Fprintf(&b, "Lexicon data dir: %s\n", s.LexiconDir)
	for _, f := range s.Files {
		base := filepath.Base(f.Path)
		if f.Bytes == 0 && f.Lines == 0 {
			fmt.Fprintf(&b, "  %-16s missing or empty\n", base)
			continue
		}
		size := humanBytes(f.Bytes)
		grow := ""
		if f.HasTimestampedTs {
			grow = fmt.Sprintf("  +%d lines (%s) in last 7d", f.Lines7d, humanBytes(f.Bytes7d))
		}
		fmt.Fprintf(&b, "  %-16s %6d lines  %s%s\n", base, f.Lines, size, grow)
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "Elements\n  %s (%d entries)\n\n", s.ElementsDir, s.ElementsN)

	fmt.Fprintf(&b, "Hook performance (last %d calls)\n", s.LatencyTotal.N)
	if s.LatencyTotal.N == 0 {
		fmt.Fprintln(&b, "  no metrics yet — invoke `lexicon hook` at least once first")
	} else {
		fmt.Fprintf(&b, "  outcomes:    %s\n", formatOutcomes(s.Outcomes))
		fmt.Fprintf(&b, "  total ms:    p50=%d  p95=%d  max=%d\n", s.LatencyTotal.P50, s.LatencyTotal.P95, s.LatencyTotal.Max)
		fmt.Fprintf(&b, "  lens ms:     p50=%d  p95=%d  max=%d  (lens-called %d/%d)\n",
			s.LatencyLens.P50, s.LatencyLens.P95, s.LatencyLens.Max, s.LensCalledN, s.LensTotal)
		fmt.Fprintf(&b, "  elements ms: p50=%d  p95=%d  max=%d\n", s.LatencySubs.P50, s.LatencySubs.P95, s.LatencySubs.Max)
		fmt.Fprintf(&b, "  gate ms:     p50=%d  p95=%d  max=%d\n", s.LatencyGate.P50, s.LatencyGate.P95, s.LatencyGate.Max)
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "Lens API spend (last %d days)\n", s.CostEstimate.WindowDays)
	if s.CostEstimate.USDExact {
		fmt.Fprintf(&b, "  $%.4f over %d lens calls\n", s.CostEstimate.USD, s.CostEstimate.LensCalls)
		fmt.Fprintf(&b, "  cache hits:  %d / %d (%d%%)\n", s.CostEstimate.CacheHits, s.CostEstimate.LensCalls, s.CostEstimate.CacheHitRatePct)
		fmt.Fprintf(&b, "  tokens:      input=%d  output=%d  cache_read=%d  cache_write=%d\n",
			s.CostEstimate.InputTokensTotal, s.CostEstimate.OutputTokensTotal,
			s.CostEstimate.CacheReadTotal, s.CostEstimate.CacheCreationTotal)
	} else {
		fmt.Fprintf(&b, "  ~$%.4f over %d lens calls (estimated; no token data yet)\n", s.CostEstimate.USD, s.CostEstimate.LensCalls)
	}
	fmt.Fprintf(&b, "  (%s)\n\n", s.CostEstimate.Note)

	fmt.Fprintln(&b, "Configuration")
	keys := []string{"hook_threshold", "hook_top_k", "lens_min_confidence", "lens_model", "metrics_enabled"}
	for _, k := range keys {
		if v, ok := s.Configuration[k]; ok {
			fmt.Fprintf(&b, "  %-20s %s\n", k, v)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Alerts")
	if len(s.Alerts) == 0 {
		fmt.Fprintln(&b, "  none — looking healthy")
	} else {
		for _, a := range s.Alerts {
			fmt.Fprintf(&b, "  - %s\n", a)
		}
	}
	return b.String()
}

func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

func formatOutcomes(m map[string]int) string {
	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s:%d", p.k, p.v))
	}
	return strings.Join(parts, "  ")
}
