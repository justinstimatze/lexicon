package lexicon

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/embedgate"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// EmbedGateBudget is the default local-embed stage (ollama) timeout.
const EmbedGateBudget = 6 * time.Second

// ResolveEmbedGateBudget honors LEXICON_EMBED_GATE_BUDGET_MS if set and
// parseable to a positive int; falls back to EmbedGateBudget otherwise.
func ResolveEmbedGateBudget() time.Duration {
	return ResolveDurationMs("LEXICON_EMBED_GATE_BUDGET_MS", EmbedGateBudget)
}

// ResolveDurationMs reads a millisecond duration from env, falling back
// to def on absence or invalid input.
func ResolveDurationMs(envKey string, def time.Duration) time.Duration {
	v := os.Getenv(envKey)
	if v == "" {
		return def
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms <= 0 {
		fmt.Fprintf(os.Stderr, "%s=%q invalid, using default %s\n", envKey, v, def)
		return def
	}
	return time.Duration(ms) * time.Millisecond
}

// DecideEmbedNarrowing applies the V209 policy: narrow to the embed
// gate's own top-K whenever it produced a ranking at all, regardless of
// whether the top score clears threshold — threshold only changes which
// log line gets emitted. Replaces the V208 policy (below-threshold fell
// through to the full pool instead), which metrics showed was mostly
// buying nothing: of 3825 real below-threshold fallthroughs, every
// recorded top score landed between 0.49 and 0.61 (never a genuine
// non-match), and the full-pool lens that ran anyway still needed a
// fresh, uncached system prompt on roughly 1 call in 5. Pure, no I/O, so
// it can be unit-tested without a real ollama+Anthropic round trip.
//
// beforeCount is the entries count prior to narrowing (for the log line
// only); pool resolves each result's AtomID back to a *LexEntry, skipping
// any that aren't found (defends against a stale prototype cache
// referencing an atom no longer in the pool).
func DecideEmbedNarrowing(gateRes []embedgate.Result, pool map[string]*types.LexEntry, threshold float64, beforeCount int) (narrowed []*types.LexEntry, active bool, logMsg string) {
	if len(gateRes) == 0 {
		return nil, false, "embed gate: no results to narrow to"
	}
	narrowed = make([]*types.LexEntry, 0, len(gateRes))
	for _, r := range gateRes {
		if e, ok := pool[r.AtomID]; ok {
			narrowed = append(narrowed, e)
		}
	}
	active = true
	if gateRes[0].Score < threshold {
		logMsg = fmt.Sprintf("embed gate: top sim %.3f below threshold %.2f (top=%s); narrowing anyway (V209)",
			gateRes[0].Score, threshold, gateRes[0].AtomID)
	} else {
		logMsg = fmt.Sprintf("embed gate: narrowed %d -> %d (top=%s @ %.3f)",
			beforeCount, len(narrowed), gateRes[0].AtomID, gateRes[0].Score)
	}
	return narrowed, active, logMsg
}
