package main

// cmd_calib.go — derive an embedding-gate threshold from a held-out POS/NEG
// probe corpus. The threshold is what the V14 hook uses to decide gate-trip-or-
// silence (cmd_hook.go: gate trips iff embedTopSim >= Threshold()), so the
// calibration question is: at what top-1 cosine does ordinary work prose stop
// matching anything strongly, and at what top-1 cosine do real-situation
// prompts start matching their gold atom?
//
// Pattern lifted from cupel's `cupel calib` subcommand (github.com/
// justinstimatze/cupel, cmd/cupel/calib.go) isomorphic; cupel has many
// engines × ~2 POS each, lexicon has many atoms × representative subset).
//
// Output is human-readable. Sweeps thresholds across a sensible range and
// reports POS-fire-rate (POS where top-1 sim ≥ threshold) + NEG-silence-rate
// (NEG where top-1 sim < threshold) + POS-routed-rate (POS where gold atom is
// in top-K AND top-1 sim ≥ threshold — i.e. the lens actually sees gold).
//
// The threshold recommendation is the smallest T where NEG-silence-rate is 100%
// (no false fires) and POS-fire-rate is maximized. If those goals conflict, the
// report shows the tradeoff and the user picks.

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/embedgate"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

type calibPOSResult struct {
	Prompt   string
	GoldID   string
	Top1ID   string
	Top1Sim  float64
	GoldRank int     // 1-indexed; 0 if gold not in scored set
	GoldSim  float64 // gold's own sim
	InTopK   bool
}

type calibNEGResult struct {
	Prompt  string
	Top1ID  string
	Top1Sim float64
}

func cmdCalib(renderDir string, args []string) {
	fs := flag.NewFlagSet("calib", flag.ExitOnError)
	topK := fs.Int("top-k", embedgate.DefaultTopK, "top-K passed to the lens — recall window")
	verbose := fs.Bool("v", false, "print per-prompt results")
	_ = fs.Parse(args)

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calib: loader: %s\n", err)
		os.Exit(1)
	}
	atoms := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		atoms = append(atoms, e)
	}
	ctx := context.Background()
	protos, err := embedgate.LoadOrBuildPrototypes(ctx, atoms)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calib: build prototypes: %s\n", err)
		fmt.Fprintln(os.Stderr, "hint: is ollama running? (curl -fsS "+embedgate.OllamaURL()+"/api/tags)")
		os.Exit(1)
	}

	positives := embedgate.ProbePositives()
	negatives := embedgate.ProbeNegatives()
	if len(positives) == 0 || len(negatives) == 0 {
		fmt.Fprintln(os.Stderr, "calib: probe corpus is empty (need both POS and NEG)")
		os.Exit(1)
	}

	// Verify gold atom IDs exist; bail fast on typos in the corpus.
	missing := []string{}
	for _, p := range positives {
		if _, ok := pool[p.AtomID]; !ok {
			missing = append(missing, p.AtomID)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "calib: POS corpus references missing atoms: %v\n", missing)
		os.Exit(1)
	}

	// Embed all probe texts in one batched call (faster than one-at-a-time).
	allTexts := make([]string, 0, len(positives)+len(negatives))
	for _, p := range positives {
		allTexts = append(allTexts, p.Text)
	}
	allTexts = append(allTexts, negatives...)
	vecs, err := embedgate.EmbedTexts(ctx, allTexts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calib: embed probes: %s\n", err)
		os.Exit(1)
	}

	posResults := make([]calibPOSResult, len(positives))
	for i, p := range positives {
		posResults[i] = scorePOS(p, vecs[i], protos, *topK)
	}
	negResults := make([]calibNEGResult, len(negatives))
	for i, n := range negatives {
		negResults[i] = scoreNEG(n, vecs[len(positives)+i], protos)
	}

	if *verbose {
		printPerPromptResults(posResults, negResults)
	}
	printSummary(posResults, negResults, *topK, len(protos))
}

// scorePOS computes top-1 sim, gold atom rank/sim, and top-K membership for one POS prompt.
func scorePOS(p embedgate.ProbeExample, pv []float64, protos map[string][][]float64, k int) calibPOSResult {
	type scored struct {
		id  string
		sim float64
	}
	all := make([]scored, 0, len(protos))
	for id, vecs := range protos {
		all = append(all, scored{id, embedgate.BestCosine(pv, vecs)})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].sim > all[j].sim })

	r := calibPOSResult{Prompt: p.Text, GoldID: p.AtomID}
	if len(all) > 0 {
		r.Top1ID = all[0].id
		r.Top1Sim = all[0].sim
	}
	for i, s := range all {
		if s.id == p.AtomID {
			r.GoldRank = i + 1
			r.GoldSim = s.sim
			if i < k {
				r.InTopK = true
			}
			break
		}
	}
	return r
}

// scoreNEG computes top-1 sim for one NEG prompt — what's the strongest match
// for an ordinary irrelevant prompt, and how high does it score?
func scoreNEG(text string, pv []float64, protos map[string][][]float64) calibNEGResult {
	r := calibNEGResult{Prompt: text}
	for id, vecs := range protos {
		s := embedgate.BestCosine(pv, vecs)
		if s > r.Top1Sim {
			r.Top1Sim = s
			r.Top1ID = id
		}
	}
	return r
}

func printPerPromptResults(pos []calibPOSResult, neg []calibNEGResult) {
	fmt.Println("--- POS results ---")
	for _, r := range pos {
		mark := "✓"
		if r.Top1ID != r.GoldID {
			mark = "✗"
		}
		fmt.Printf("  %s gold=%s top1=%s sim=%.3f gold_rank=%d gold_sim=%.3f  %q\n",
			mark, r.GoldID, r.Top1ID, r.Top1Sim, r.GoldRank, r.GoldSim, truncate(r.Prompt, 70))
	}
	fmt.Println("--- NEG results ---")
	for _, r := range neg {
		fmt.Printf("  top1=%s sim=%.3f  %q\n", r.Top1ID, r.Top1Sim, truncate(r.Prompt, 70))
	}
	fmt.Println()
}

func printSummary(pos []calibPOSResult, neg []calibNEGResult, topK int, atomCount int) {
	posTop1 := make([]float64, len(pos))
	posGold := make([]float64, 0, len(pos))
	inTopK := 0
	top1IsGold := 0
	for i, r := range pos {
		posTop1[i] = r.Top1Sim
		if r.InTopK {
			inTopK++
			posGold = append(posGold, r.GoldSim)
		}
		if r.Top1ID == r.GoldID {
			top1IsGold++
		}
	}
	negTop1 := make([]float64, len(neg))
	for i, r := range neg {
		negTop1[i] = r.Top1Sim
	}

	posTop1Floor := minSlice(posTop1)
	negTop1Ceil := maxSlice(negTop1)
	posTop1Sorted := sortedCopy(posTop1)
	negTop1Sorted := sortedCopy(negTop1)

	fmt.Println("=== calibration summary ===")
	fmt.Printf("model:    %s\n", embedgate.EmbedModel())
	fmt.Printf("atoms:    %d  (prototypes in cache)\n", atomCount)
	fmt.Printf("POS:      %d prompts × %d unique gold atoms\n", len(pos), uniqueGoldCount(pos))
	fmt.Printf("NEG:      %d prompts\n", len(neg))
	fmt.Printf("top-K:    %d  (lens sees this many atoms on a trip)\n\n", topK)

	fmt.Println("POS top-1 sim distribution:")
	fmt.Printf("  min=%.3f  p25=%.3f  p50=%.3f  p75=%.3f  max=%.3f\n",
		posTop1Sorted[0], pct(posTop1Sorted, 0.25), pct(posTop1Sorted, 0.50), pct(posTop1Sorted, 0.75), posTop1Sorted[len(posTop1Sorted)-1])
	fmt.Println("NEG top-1 sim distribution:")
	fmt.Printf("  min=%.3f  p25=%.3f  p50=%.3f  p75=%.3f  max=%.3f\n",
		negTop1Sorted[0], pct(negTop1Sorted, 0.25), pct(negTop1Sorted, 0.50), pct(negTop1Sorted, 0.75), negTop1Sorted[len(negTop1Sorted)-1])
	fmt.Println()

	fmt.Printf("POS recall:\n")
	fmt.Printf("  top-1 == gold:           %d / %d  (%.0f%%)\n", top1IsGold, len(pos), 100*float64(top1IsGold)/float64(len(pos)))
	fmt.Printf("  gold in top-%d:          %d / %d  (%.0f%%)\n", topK, inTopK, len(pos), 100*float64(inTopK)/float64(len(pos)))
	if len(posGold) > 0 {
		sort.Float64s(posGold)
		fmt.Printf("  gold-sim when in top-K:  min=%.3f  p25=%.3f  p50=%.3f  max=%.3f\n",
			posGold[0], pct(posGold, 0.25), pct(posGold, 0.50), posGold[len(posGold)-1])
	}
	fmt.Println()

	fmt.Println("threshold sweep (gate trips when top-1 sim ≥ T):")
	fmt.Println("  T       POS fire   POS routed   NEG silence   notes")
	sep := strings.Repeat("─", 60)
	fmt.Println("  " + sep)
	for _, T := range []float64{0.50, 0.52, 0.54, 0.56, 0.58, 0.60, 0.62, 0.64} {
		posFire := 0
		posRouted := 0
		for _, r := range pos {
			if r.Top1Sim >= T {
				posFire++
				if r.InTopK {
					posRouted++
				}
			}
		}
		negSilence := 0
		for _, r := range neg {
			if r.Top1Sim < T {
				negSilence++
			}
		}
		note := ""
		if math.Abs(T-embedgate.DefaultThreshold) < 1e-9 {
			note = "current default"
		}
		fmt.Printf("  %.2f    %3d/%3d    %3d/%3d      %3d/%3d       %s\n",
			T, posFire, len(pos), posRouted, len(pos), negSilence, len(neg), note)
	}
	fmt.Println()

	// Recommendation: smallest T where all NEG silenced, picked among the
	// sweep grid; that maximizes POS recall while eliminating false fires.
	rec := recommendedThreshold(negTop1)
	posFireAtRec, posRoutedAtRec := countAtThreshold(pos, rec)
	fmt.Println("recommendation:")
	fmt.Printf("  POS top-1 floor:  %.3f  (smallest POS top-1 sim — should fire here)\n", posTop1Floor)
	fmt.Printf("  NEG top-1 ceil:   %.3f  (highest noise-prompt sim — must silence above this)\n", negTop1Ceil)
	if negTop1Ceil < posTop1Floor {
		fmt.Printf("  clean separation: any T in (%.3f, %.3f] silences all NEG and fires all POS\n", negTop1Ceil, posTop1Floor)
		fmt.Printf("  pick midpoint:    %.3f\n", (negTop1Ceil+posTop1Floor)/2)
	} else {
		fmt.Printf("  ⚠ overlap: NEG ceil (%.3f) ≥ POS floor (%.3f); some POS will miss or some NEG will fire\n", negTop1Ceil, posTop1Floor)
		fmt.Printf("  T just above NEG ceil:  %.3f  (silences all NEG; misses %d POS)\n", rec,
			len(pos)-posFireAtRec)
	}
	fmt.Printf("  suggested LEXICON_GATE_THRESHOLD = %.3f  (POS fires %d/%d, POS routed %d/%d, NEG silenced ALL)\n",
		rec, posFireAtRec, len(pos), posRoutedAtRec, len(pos))
	fmt.Printf("  current DefaultThreshold = %.3f\n", embedgate.DefaultThreshold)
}

// recommendedThreshold = smallest T (rounded to 0.005) such that no NEG fires.
// We bump just above NEG ceil so that the highest-scoring NEG silences cleanly.
func recommendedThreshold(negTop1 []float64) float64 {
	ceil := maxSlice(negTop1)
	t := math.Ceil((ceil+0.0005)*1000) / 1000 // round up to nearest 0.001
	// snap to 0.005 grid for stable defaults
	t = math.Ceil(t*200) / 200
	return t
}

func countAtThreshold(pos []calibPOSResult, T float64) (fire, routed int) {
	for _, r := range pos {
		if r.Top1Sim >= T {
			fire++
			if r.InTopK {
				routed++
			}
		}
	}
	return
}

func minSlice(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxSlice(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func sortedCopy(xs []float64) []float64 {
	out := make([]float64, len(xs))
	copy(out, xs)
	sort.Float64s(out)
	return out
}

func pct(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(q * float64(len(sorted)-1))
	return sorted[idx]
}

func uniqueGoldCount(pos []calibPOSResult) int {
	seen := map[string]bool{}
	for _, r := range pos {
		seen[r.GoldID] = true
	}
	return len(seen)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
