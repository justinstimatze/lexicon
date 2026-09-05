package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// cmdCouncil is `lexicon read` pointed at a different output: instead of
// a synthesized report, it hands back the top-K atoms as distinct
// voices, each arguing from its own agent-instruction. Same scoring
// engine as read/what-if (patternIDCore) — this only changes the shape
// of what gets printed. The intended use is a stuck decision: paste the
// situation, get back a handful of independently-sourced takes rather
// than one synthesized answer.
func cmdCouncil(renderDir string, args []string) {
	fl := flag.NewFlagSet("council", flag.ExitOnError)
	topK := fl.Int("top-k", 5, "voices to seat")
	noLens := fl.Bool("no-lens", false, "skip the LLM-backed semantic lens (lexical-only on full pool)")
	format := fl.String("format", "json", "output format: json (default, agent-consumable) or text (a printed council session)")
	args = reorderFlagsFirst(args)
	_ = fl.Parse(args)

	rest := fl.Args()
	var src io.Reader = os.Stdin
	srcName := "stdin"
	if len(rest) > 0 && rest[0] != "-" {
		f, err := os.Open(rest[0])
		if err != nil {
			fatal("open %s: %v", rest[0], err)
		}
		defer f.Close()
		src = f
		srcName = rest[0]
	}
	data, err := io.ReadAll(src)
	if err != nil {
		fatal("read %s: %v", srcName, err)
	}
	contextStr := strings.TrimSpace(string(data))
	if contextStr == "" {
		fatal("council: empty input (from %s)", srcName)
	}

	if os.Getenv("LEXICON_LENS_TIMEOUT_MS") == "" {
		_ = os.Setenv("LEXICON_LENS_TIMEOUT_MS", "30000")
	}

	corp := loadCorpusOrFatal(renderDir)
	picked, scores, _, _, _ := corp.ScoreRaw(context.Background(), contextStr, *topK, *noLens)
	if len(picked) == 0 {
		fmt.Fprintln(os.Stderr, "(no voices surfaced)")
		return
	}

	if *format == "text" {
		fmt.Print(formatCouncilText(contextStr, picked, scores))
		return
	}
	fmt.Println(formatCouncilJSON(contextStr, picked, scores))
}

type councilVoice struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	TypeIn  string  `json:"type_in"`
	TypeOut string  `json:"type_out"`
	Score   float64 `json:"score"`
	// Voice is the atom's agent-instruction, read as what this voice
	// argues — the same field the web app's AtomCard surfaces as the
	// operational rule.
	Voice string `json:"voice"`
}

func formatCouncilJSON(contextStr string, picked []*types.LexEntry, scores map[string]float64) string {
	voices := make([]councilVoice, 0, len(picked))
	for _, e := range picked {
		voices = append(voices, councilVoice{
			ID:      e.ID,
			Name:    e.Name,
			TypeIn:  e.TypeIn,
			TypeOut: e.TypeOut,
			Score:   scores[e.ID],
			Voice:   e.AgentInstruction,
		})
	}
	out, err := json.Marshal(struct {
		Context string         `json:"context"`
		Voices  []councilVoice `json:"voices"`
	}{Context: contextStr, Voices: voices})
	if err != nil {
		fatal("marshal: %v", err)
	}
	return string(out)
}

func formatCouncilText(contextStr string, picked []*types.LexEntry, scores map[string]float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "COUNCIL - %d voices on:\n\"%s\"\n\n", len(picked), truncate(contextStr, 200))
	for _, e := range picked {
		fmt.Fprintf(&b, "%s (%s -> %s) [score %.2f]\n", e.Name, e.TypeIn, e.TypeOut, scores[e.ID])
		if e.AgentInstruction != "" {
			fmt.Fprintf(&b, "  %s\n\n", e.AgentInstruction)
		} else {
			fmt.Fprintf(&b, "  (no agent-instruction on file for %s)\n\n", e.ID)
		}
	}
	return b.String()
}
