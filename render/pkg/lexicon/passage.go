package lexicon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/embedgate"
	"github.com/justinstimatze/lexicon/render/internal/lens"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// TraceOptions configures TracePassage. The zero value means 50 embed-gate
// candidates, at most 3 picks, a 0.6 confidence floor, and the client's
// default (quality) model.
type TraceOptions struct {
	// Candidates is how many atoms the embed gate hands the lens. The
	// hook uses 20 under a latency budget; a passage is longer and more
	// mixed than a prompt, and this path has no latency budget, so the
	// default is wider. Every hit records its gate rank, so a run reports
	// whether the picks were coming from beyond 20 (they are — see the
	// summary `document-trace` prints).
	Candidates int
	// MaxPicks per passage, at most lens.MaxPassagePicks.
	MaxPicks int
	// MinConfidence drops lens picks below it. The lens prompt already
	// asks the model to omit below 0.6; this enforces it.
	MinConfidence float64
	// Model overrides the client default.
	Model string
	// LensRetries is how many times a failed lens call is retried before
	// the passage is reported untraced. Batch runs hit transient 429/529s.
	LensRetries int
}

func (o TraceOptions) withDefaults() TraceOptions {
	if o.Candidates <= 0 {
		o.Candidates = 50
	}
	if o.MaxPicks <= 0 {
		o.MaxPicks = lens.MaxPassagePicks
	}
	if o.MinConfidence <= 0 {
		o.MinConfidence = 0.6
	}
	if o.LensRetries < 0 {
		o.LensRetries = 0
	}
	return o
}

// TraceHit is one pattern the lens found in a passage.
type TraceHit struct {
	Entry      *types.LexEntry
	Confidence float64
	// Evidence is the model's verbatim span; EvidenceStart/End are its
	// rune offsets in the passage, or -1 when it could not be anchored.
	// EvidencePartial is true when only the leading words anchored.
	Evidence        string
	EvidenceStart   int
	EvidenceEnd     int
	EvidencePartial bool
	Why             string
	// GateRank is the atom's 1-based position in the embed gate's cosine
	// ranking — the recall diagnostic for Candidates.
	GateRank int
}

// TraceResult is what TracePassage returns. Traced is true when the lens
// ran and answered, even with zero hits (an empty answer is a finding).
// Traced false with a Note means the pipeline could not judge the
// passage; there is deliberately no lexical fallback that would fill
// Hits with keyword collisions in that case.
type TraceResult struct {
	Traced      bool
	Note        string
	Hits        []TraceHit
	Diagnostics []string
}

// TracePassage judges one passage of a document against the corpus: embed
// gate narrows to opts.Candidates, lens.FilterPassage picks with evidence,
// hits are ranked by lens confidence alone. Compare ScoreRaw, which is
// the hook's path (20 candidates, the hook's prompt, a deterministic gate
// with tier and vocabulary heuristics, always top-K results) — reused on
// documents it produced two hits per paragraph regardless of whether
// anything fit, and ranked them partly on word collisions.
func (corp *Corpus) TracePassage(ctx context.Context, passage string, meta lens.PassageMeta, opts TraceOptions) TraceResult {
	opts = opts.withDefaults()
	var res TraceResult
	diag := func(format string, args ...any) {
		res.Diagnostics = append(res.Diagnostics, fmt.Sprintf(format, args...))
	}
	untraced := func(format string, args ...any) TraceResult {
		res.Note = fmt.Sprintf(format, args...)
		diag("untraced: %s", res.Note)
		return res
	}

	if lens.Disabled() {
		return untraced("lens disabled (LEXICON_LENS_DISABLED=1 or no ANTHROPIC_API_KEY)")
	}
	c, err := client.New()
	if err != nil {
		return untraced("lens client: %v", err)
	}

	embedCtx, embedCancel := context.WithTimeout(ctx, ResolveEmbedGateBudget())
	gateRes, gateErr := embedgate.Score(embedCtx, passage, corp.allEntries, opts.Candidates)
	embedCancel()
	switch {
	case errors.Is(gateErr, embedgate.ErrColdCache):
		return untraced("embed gate: prototype cache cold (run: lexicon build-prototypes)")
	case gateErr != nil:
		return untraced("embed gate: %v", gateErr)
	case len(gateRes) == 0:
		return untraced("embed gate: no candidates")
	}
	candidates := make([]*types.LexEntry, 0, len(gateRes))
	rank := make(map[string]int, len(gateRes))
	for i, r := range gateRes {
		if e, ok := corp.pool[r.AtomID]; ok {
			candidates = append(candidates, e)
			rank[e.ID] = i + 1
		}
	}
	diag("embed gate: %d -> %d candidates (top=%s @ %.3f, #%d=%s @ %.3f)",
		len(corp.allEntries), len(candidates), gateRes[0].AtomID, gateRes[0].Score,
		len(gateRes), gateRes[len(gateRes)-1].AtomID, gateRes[len(gateRes)-1].Score)

	var lensRes lens.PassageResult
	var lensErr error
	for attempt := 0; attempt <= opts.LensRetries; attempt++ {
		if attempt > 0 {
			diag("lens: retry %d after: %v", attempt, lensErr)
			select {
			case <-ctx.Done():
				return untraced("lens: %v", ctx.Err())
			case <-time.After(time.Duration(attempt) * 3 * time.Second):
			}
		}
		lensRes, lensErr = lens.FilterPassage(ctx, passage, meta, candidates, c, opts.Model, opts.MaxPicks)
		if lensErr == nil {
			break
		}
	}
	if lensErr != nil {
		return untraced("lens: %v", lensErr)
	}
	res.Traced = true

	kept := 0
	for _, p := range lensRes.Picks {
		if p.Confidence < opts.MinConfidence {
			diag("lens: dropped %s at %.2f (floor %.2f)", p.ID, p.Confidence, opts.MinConfidence)
			continue
		}
		e := corp.pool[p.ID]
		if e == nil {
			continue
		}
		h := TraceHit{
			Entry:         e,
			Confidence:    p.Confidence,
			Evidence:      p.Evidence,
			EvidenceStart: -1,
			EvidenceEnd:   -1,
			Why:           p.Why,
			GateRank:      rank[p.ID],
		}
		if p.Evidence != "" {
			if s, en, partial, ok := lens.LocateEvidence(passage, p.Evidence); ok {
				h.EvidenceStart, h.EvidenceEnd, h.EvidencePartial = s, en, partial
			} else {
				diag("lens: %s evidence not found in passage: %q", p.ID, p.Evidence)
			}
		}
		res.Hits = append(res.Hits, h)
		kept++
	}
	diag("lens: %d candidates -> %d picks, %d kept", len(candidates), len(lensRes.Picks), kept)
	return res
}
