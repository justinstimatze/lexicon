package main

// cmd_replay_fires.go — replay the last N V13 fires through the V14 embedding
// gate. Real-world validation of the threshold against actual past traffic
// (vs. the synthetic POS/NEG corpus in probe.go).
//
// Reads ~/.claude/lexicon/fires.jsonl (legacy V13 lexical-gate fires; each has
// the original prompt text + V13's chosen top-atom), embeds each prompt with
// the current V14 prototypes, and reports per-prompt:
//
//   - V14 top-1 atom + sim
//   - whether V14 at current threshold would FIRE or SILENCE
//   - V14's rank for the atom V13 picked (was V13's pick even close?)
//   - whether V14 would agree with V13 (same top-atom)
//
// Summary aggregates: silenced-rate, agreement-rate, same-atom-rate. Use this
// to validate the threshold against the actual prompt distribution Claude
// Code sees — which is dominated by short acknowledgments ("go", "sure", "1")
// that the V13 lexical gate falsely matched and that V14 should silence.

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/embedgate"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// fireRecord (cmd_hook.go) is the shape we read from fires.jsonl.

func cmdReplayFires(renderDir string, args []string) {
	fs := flag.NewFlagSet("replay-fires", flag.ExitOnError)
	n := fs.Int("n", 30, "number of most-recent fires to replay")
	verbose := fs.Bool("v", false, "print per-fire results")
	_ = fs.Parse(args)

	fires, err := readLastFires(*n)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay-fires: %s\n", err)
		os.Exit(1)
	}
	if len(fires) == 0 {
		fmt.Fprintln(os.Stderr, "replay-fires: no fires found in fires.jsonl")
		os.Exit(1)
	}

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay-fires: loader: %s\n", err)
		os.Exit(1)
	}
	atoms := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		atoms = append(atoms, e)
	}
	ctx := context.Background()
	protos, err := embedgate.LoadOrBuildPrototypes(ctx, atoms)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay-fires: build prototypes: %s\n", err)
		fmt.Fprintln(os.Stderr, "hint: is ollama running? (curl -fsS "+embedgate.OllamaURL()+"/api/tags)")
		os.Exit(1)
	}

	prompts := make([]string, len(fires))
	for i, f := range fires {
		prompts[i] = f.PromptSnippet
	}
	vecs, err := embedgate.EmbedTexts(ctx, prompts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay-fires: embed: %s\n", err)
		os.Exit(1)
	}

	threshold := embedgate.Threshold()
	topK := embedgate.TopK()

	type rep struct {
		fire       fireRecord
		v14Top1ID  string
		v14Top1Sim float64
		v13InTopK  bool
		v13Rank    int
		v13Sim     float64
		wouldFire  bool
		sameAtom   bool
	}
	reports := make([]rep, len(fires))

	for i, f := range fires {
		// Score this prompt against every prototype.
		type scored struct {
			id  string
			sim float64
		}
		all := make([]scored, 0, len(protos))
		for id, atomVecs := range protos {
			all = append(all, scored{id, embedgate.BestCosine(vecs[i], atomVecs)})
		}
		sort.Slice(all, func(a, b int) bool { return all[a].sim > all[b].sim })

		r := rep{fire: f}
		if len(all) > 0 {
			r.v14Top1ID = all[0].id
			r.v14Top1Sim = all[0].sim
			r.wouldFire = all[0].sim >= threshold
		}
		v13ID := ""
		if len(f.TopResults) > 0 {
			v13ID = f.TopResults[0].PrimitiveID
			r.sameAtom = (r.v14Top1ID == v13ID)
		}
		for j, s := range all {
			if s.id == v13ID {
				r.v13Rank = j + 1
				r.v13Sim = s.sim
				if j < topK {
					r.v13InTopK = true
				}
				break
			}
		}
		reports[i] = r
	}

	if *verbose {
		fmt.Println("--- per-fire replay (V13 lexical → V14 embedding) ---")
		for _, r := range reports {
			mark := "🔴SILENCE"
			if r.wouldFire {
				mark = "🟢 FIRE  "
			}
			agree := "≠"
			if r.sameAtom {
				agree = "="
			}
			v13ID := "—"
			if len(r.fire.TopResults) > 0 {
				v13ID = r.fire.TopResults[0].PrimitiveID
			}
			fmt.Printf("  %s  v14=%s sim=%.3f  v13=%s rank=%d sim=%.3f %s  %q\n",
				mark, r.v14Top1ID, r.v14Top1Sim, v13ID, r.v13Rank, r.v13Sim, agree,
				truncate(r.fire.PromptSnippet, 60))
		}
		fmt.Println()
	}

	// Summary.
	silenced := 0
	fired := 0
	agreedSilent := 0
	agreedFire := 0
	diffFire := 0
	v13InTopK := 0
	var fireSims, silenceSims []float64
	for _, r := range reports {
		if r.wouldFire {
			fired++
			if r.sameAtom {
				agreedFire++
			} else {
				diffFire++
			}
			fireSims = append(fireSims, r.v14Top1Sim)
		} else {
			silenced++
			if r.sameAtom {
				agreedSilent++
			}
			silenceSims = append(silenceSims, r.v14Top1Sim)
		}
		if r.v13InTopK {
			v13InTopK++
		}
	}

	fmt.Println("=== replay summary ===")
	fmt.Printf("model:      %s  threshold: %.3f  top-K: %d\n",
		embedgate.EmbedModel(), threshold, topK)
	fmt.Printf("fires:      %d (from ~/.claude/lexicon/fires.jsonl, most-recent)\n\n", len(reports))
	fmt.Printf("V14 behavior on V13's prompt set:\n")
	fmt.Printf("  silence:  %d / %d  (%.0f%%)  — V13 noise the V14 gate now catches\n",
		silenced, len(reports), 100*float64(silenced)/float64(len(reports)))
	fmt.Printf("  fire:     %d / %d  (%.0f%%)\n",
		fired, len(reports), 100*float64(fired)/float64(len(reports)))
	fmt.Printf("    same atom as V13:        %d  (V14 confirms V13's pick)\n", agreedFire)
	fmt.Printf("    different atom from V13: %d  (V14 chose differently)\n", diffFire)
	fmt.Printf("  V13-atom-in-V14-top-%d:   %d / %d  (%.0f%%)  — V13's pick reachable to V14 lens\n",
		topK, v13InTopK, len(reports), 100*float64(v13InTopK)/float64(len(reports)))

	if len(fireSims) > 0 {
		sort.Float64s(fireSims)
		fmt.Printf("\nFIRE top-1 sim:    min=%.3f  p50=%.3f  max=%.3f\n",
			fireSims[0], fireSims[len(fireSims)/2], fireSims[len(fireSims)-1])
	}
	if len(silenceSims) > 0 {
		sort.Float64s(silenceSims)
		fmt.Printf("SILENCE top-1 sim: min=%.3f  p50=%.3f  max=%.3f\n",
			silenceSims[0], silenceSims[len(silenceSims)/2], silenceSims[len(silenceSims)-1])
	}
}

// readLastFires reads the LAST n records from ~/.claude/lexicon/fires.jsonl
// (or LEXICON_FIRES_PATH override). Uses a sliding window so it doesn't load
// the whole file (~1900 lines on this host).
func readLastFires(n int) ([]fireRecord, error) {
	path := os.Getenv("LEXICON_FIRES_PATH")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".claude", "lexicon", "fires.jsonl")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	window := make([][]byte, 0, n)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		if len(window) < n {
			window = append(window, cp)
		} else {
			copy(window, window[1:])
			window[n-1] = cp
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]fireRecord, 0, len(window))
	for _, b := range window {
		var r fireRecord
		if err := json.Unmarshal(b, &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
