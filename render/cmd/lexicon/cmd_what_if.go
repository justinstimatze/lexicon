package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/explain"
	"github.com/justinstimatze/lexicon/render/internal/framestatus"
	"github.com/justinstimatze/lexicon/render/internal/gate"
	"github.com/justinstimatze/lexicon/render/internal/lens"
	"github.com/justinstimatze/lexicon/render/internal/loader"
	"github.com/justinstimatze/lexicon/render/internal/probe"
	"github.com/justinstimatze/lexicon/render/internal/types"
	pkglexicon "github.com/justinstimatze/lexicon/render/pkg/lexicon"
)

// cobraFallbackProbes is the lex-prwfm-derived default probe list used
// in greedy mode when the picked entry has no critical-questions populated.
// See five-what-ifs-design.md §V31 dialogue-shape constraint.
var cobraFallbackProbes = []string{
	"what would game this?",
	"who benefits from the gameability?",
	"what counter-incentive does this create?",
	"what happens to the people who CAN'T do this?",
}

type whatIfStep struct {
	Depth  int
	CardID string
	Name   string
	Probe  string
	Answer string
}

func cmdWhatIf(renderDir string, args []string) {
	fl := flag.NewFlagSet("what-if", flag.ExitOnError)
	mode := fl.String("mode", "probe", "probe (V36 default — return disambiguation questions as markdown for Claude to weave into conversation) | greedy (V31 interactive REPL prototype) | intervene (V71 reaction-steering: firing reaction → product, catalysts to watch, inhibitors to add) | pattern-id (V96 plain-language identification of applicable patterns + their adjacent neighbors)")
	contextStr := fl.String("context", "", "starting situation (required)")
	depth := fl.Int("depth", 5, "(greedy mode) trajectory depth")
	topK := fl.Int("top-k", 3, "candidates surfaced per level (greedy) / considered (probe)")
	noLens := fl.Bool("no-lens", false, "skip the LLM-backed semantic lens (lexical-only on full pool)")
	maxProbes := fl.Int("max-probes", probe.DefaultMaxProbes, "(probe mode) cap on probes returned")
	explainFlag := fl.Bool("explain", false, "route the markdown output through an LLM-backed translator so end-users see plain conversational language instead of chemistry vocabulary, tier labels, and lex-NNNN ids")
	format := fl.String("format", "json", "(intervene + pattern-id) output format: json (default, agent-consumable) or markdown")
	detail := fl.Bool("detail", false, "(pattern-id, json format) include critical_questions and the full 6-neighbor adjacency list. Default omits critical_questions and caps adjacencies at 3.")
	_ = fl.Parse(args)

	// CLI/batch use can afford to wait; the 8s lens default is tuned for
	// the hook hot path. An explicit env value still wins.
	if os.Getenv("LEXICON_LENS_TIMEOUT_MS") == "" {
		_ = os.Setenv("LEXICON_LENS_TIMEOUT_MS", "30000")
	}

	if *contextStr == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal("read stdin: %v", err)
		}
		*contextStr = string(data)
	}
	if strings.TrimSpace(*contextStr) == "" {
		fatal("--context is required (the situation to disambiguate; use '-' for stdin)")
	}

	switch *mode {
	case "probe":
		runProbe(renderDir, *contextStr, *topK, *maxProbes, *noLens, *explainFlag)
	case "greedy":
		runGreedy(renderDir, *contextStr, *depth, *topK, *noLens)
	case "intervene":
		runIntervene(renderDir, *contextStr, *topK, *noLens, *explainFlag, *format)
	case "pattern-id":
		runPatternID(renderDir, *contextStr, *topK, *noLens, *explainFlag, *detail, *format)
	default:
		fatal("--mode must be 'probe', 'greedy', 'intervene', or 'pattern-id', got %q", *mode)
	}
}

// emitOutput is the print site shared by all non-greedy modes. When
// explainFlag is set, the structured markdown is translated through an
// LLM into plain conversational language before printing (per memory rule
// feedback_interpret_dont_expose_chemistry). Translation failures fall
// back to raw markdown with a stderr note — never silent.
func emitOutput(output string, explainFlag bool) {
	if !explainFlag {
		fmt.Print(output)
		return
	}
	c, err := client.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "explain: client init: %v (falling back to raw markdown)\n", err)
		fmt.Print(output)
		return
	}
	translated, err := explain.Translate(context.Background(), c, output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "explain: %v (falling back to raw markdown)\n", err)
		fmt.Print(output)
		return
	}
	fmt.Println(translated)
}

// runProbe: V36 default. Run lens+gate once on the starting context to get
// top-K cards; pass them to probe.Generate; emit markdown to stdout for
// Claude to consume + paraphrase into conversation. Non-interactive.
func runProbe(renderDir, contextStr string, topK, maxProbes int, noLens, explainFlag bool) {
	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("%v", err)
	}
	allEntries := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		allEntries = append(allEntries, e)
	}

	candidatePool := allEntries
	var lensConfidences map[string]float64
	if !noLens && !lens.Disabled() {
		if c, err := client.New(); err != nil {
			fmt.Fprintf(os.Stderr, "lens: client init: %v (falling back to lexical full-pool)\n", err)
		} else {
			lensRes, lensErr := lens.Filter(context.Background(), contextStr, allEntries, c, false)
			switch {
			case lensErr != nil:
				fmt.Fprintf(os.Stderr, "lens: %v (falling back to lexical full-pool)\n", lensErr)
			case len(lensRes.Entries) == 0:
				fmt.Fprintln(os.Stderr, "lens: no semantically relevant primitives — falling back to lexical full-pool")
			default:
				fmt.Fprintf(os.Stderr, "lens: %d -> %d candidates\n", len(allEntries), len(lensRes.Entries))
				candidatePool = lensRes.Entries
				lensConfidences = lensRes.Confidences
			}
		}
	}

	// Frame-status (Principle 10): label each candidate's epistemic posture
	// so the probe consumer can see at a glance whether a card grounds in an
	// external check or in judgment, AND let the gate down-weight constitutive
	// atoms rather than just label them after the fact. Fail-soft: missing
	// register drops labels/down-weighting, doesn't break the render.
	fsMap, err := framestatus.Load(renderDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "frame-status: %v (rendering without frame labels)\n", err)
	}

	results := gate.Run(gate.Input{
		Pool:        candidatePool,
		Context:     contextStr,
		TopK:        topK,
		Confidences: lensConfidences,
		FrameStatus: fsMap,
	})
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "(no candidates surfaced; no probes to suggest)")
		return
	}

	topKEntries := make([]*types.LexEntry, 0, len(results))
	for _, r := range results {
		if e := pool[r.PrimitiveID]; e != nil {
			topKEntries = append(topKEntries, e)
		}
	}

	in := probe.Input{
		Context:     contextStr,
		TopK:        topKEntries,
		MaxProbes:   maxProbes,
		FrameStatus: fsMap,
	}
	out := probe.Generate(in)

	emitOutput(probe.FormatMarkdown(in, out), explainFlag)
}

// runPatternID: V96 plain-language identification. Lens+gate the situation,
// keep the top-K applicable patterns regardless of tier, and render each
// one as: name + one-line gloss + adjacencies walked from the related-list.
// Chemistry vocabulary (reactants/catalysts/inhibitors) is deliberately
// NOT exposed here — that's what `intervene` mode is for. This mode is the
// primary demo surface: "here's what applies, here's what's nearby."
// patternIDCore is the shared pipeline behind runPatternID (single passage)
// and runPartitions (N passages, cross-fire aggregation): lens-filter
// against the corpus, run the gate, and return the picked entries plus
// their scores and lexical-match flags. Callers scoring N passages
// against the same corpus (runPartitions) load it once via
// pkglexicon.LoadCorpus and call ScoreRaw per passage, rather than
// reloading per call.
//
// This CLI is itself a thin wrapper over pkg/lexicon now -- see that
// package's Corpus.ScoreRaw for the actual implementation.
func loadCorpusOrFatal(renderDir string) *pkglexicon.Corpus {
	corp, err := pkglexicon.LoadCorpus(renderDir)
	if err != nil {
		fatal("%v", err)
	}
	return corp
}

func runPatternID(renderDir, contextStr string, topK int, noLens, explainFlag, detail bool, format ...string) {
	outputFormat := "json"
	if len(format) > 0 && format[0] != "" {
		outputFormat = format[0]
	}
	corp := loadCorpusOrFatal(renderDir)
	picked, scores, lexMatch, _, diag := corp.ScoreRaw(context.Background(), contextStr, topK, noLens)
	for _, d := range diag {
		fmt.Fprintln(os.Stderr, d)
	}
	if len(picked) == 0 {
		fmt.Fprintln(os.Stderr, "(no patterns surfaced)")
		return
	}

	if outputFormat == "json" {
		fmt.Println(formatPatternIDJSON(contextStr, picked, scores, lexMatch, corp.Pool(), corp.FrameStatus(), !noLens, topK, detail))
		return
	}
	emitOutput(formatPatternID(contextStr, picked, scores, corp.Pool(), corp.FrameStatus()), explainFlag)
}

// formatPatternIDJSON returns the structured JSON shape of the same data
// formatPatternID emits as markdown. The shape is the agent-consumable
// contract — id/name/tier/score, lexical_match, frame-status, gloss,
// agent-instruction, and adjacencies. Stable enough that consumers can pin
// to specific keys. detail=false (the default) omits critical_questions and
// caps adjacencies at 3 rather than 6 — critical_questions runs several
// hundred chars per entry and adjacencies duplicate content a follow-up
// lexicon_constellation call already provides on demand, so the full shape
// costs real per-call tokens for content most callers never read. detail=true
// restores the original always-everything shape for a caller doing deep
// analysis on a single passage.
func formatPatternIDJSON(contextStr string, picked []*types.LexEntry, scores map[string]float64, lexMatch map[string]bool, pool map[string]*types.LexEntry, fsMap framestatus.Map, lensUsed bool, topK int, detail bool) string {
	maxAdjacencies := 3
	maxCQs := 0
	if detail {
		maxAdjacencies = 6
		maxCQs = 3
	}
	type adjacency struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Gloss string `json:"gloss,omitempty"`
	}
	type pattern struct {
		ID                string      `json:"id"`
		Name              string      `json:"name"`
		Tier              string      `json:"tier"`
		Score             float64     `json:"score"`
		LexicalMatch      bool        `json:"lexical_match"`
		FrameStatus       string      `json:"frame_status,omitempty"`
		FrameHandle       string      `json:"frame_handle,omitempty"`
		Gloss             string      `json:"gloss,omitempty"`
		AgentInstruction  string      `json:"agent_instruction,omitempty"`
		CriticalQuestions []string    `json:"critical_questions,omitempty"`
		Adjacencies       []adjacency `json:"adjacencies,omitempty"`
	}
	type out struct {
		Context  string    `json:"context"`
		TopK     int       `json:"top_k"`
		LensUsed bool      `json:"lens_used"`
		Patterns []pattern `json:"patterns"`
	}

	patterns := make([]pattern, 0, len(picked))
	for _, e := range picked {
		tier := e.Tier
		if tier == "" {
			tier = "atomic"
		}
		p := pattern{
			ID:               e.ID,
			Name:             e.Name,
			Tier:             tier,
			Score:            scores[e.ID],
			LexicalMatch:     lexMatch[e.ID],
			Gloss:            patternGloss(e),
			AgentInstruction: e.AgentInstruction,
		}
		if fs, ok := fsMap.Lookup(e.ID); ok {
			p.FrameStatus = string(fs.Status)
			p.FrameHandle = fs.Handle
		}
		if len(e.CriticalQuestions) > 0 {
			limit := maxCQs
			if len(e.CriticalQuestions) < limit {
				limit = len(e.CriticalQuestions)
			}
			p.CriticalQuestions = e.CriticalQuestions[:limit]
		}
		for _, n := range pickAdjacencies(e, pool, maxAdjacencies) {
			p.Adjacencies = append(p.Adjacencies, adjacency{
				ID:    n.ID,
				Name:  n.Name,
				Gloss: patternGloss(n),
			})
		}
		patterns = append(patterns, p)
	}

	doc := out{
		Context:  strings.TrimSpace(contextStr),
		TopK:     topK,
		LensUsed: lensUsed,
		Patterns: patterns,
	}
	buf, err := jsonMarshalIndent(doc)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return buf
}

func jsonMarshalIndent(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// formatPatternID renders the pattern-id output as markdown. Each top-K
// pattern gets its name, tier, one-line gloss (canonical-instance[0]), and
// adjacencies — the related-list resolved to neighbor name + their own
// one-line gloss, capped to avoid wall-of-text on heavily-connected atoms.
func formatPatternID(contextStr string, picked []*types.LexEntry, scores map[string]float64, pool map[string]*types.LexEntry, fsMap framestatus.Map) string {
	const maxAdjacencies = 6
	var b strings.Builder
	fmt.Fprintf(&b, "# Patterns applicable to: %s\n\n", strings.TrimSpace(contextStr))

	for _, e := range picked {
		tier := e.Tier
		if tier == "" {
			tier = "atomic"
		}
		fs, fsKnown := fsMap.Lookup(e.ID)
		fmt.Fprintf(&b, "## %s — %s\n", e.ID, e.Name)
		if fsKnown {
			fmt.Fprintf(&b, "*tier: %s · score: %.2f · frame: %s*\n\n", tier, scores[e.ID], fs.Label())
		} else {
			fmt.Fprintf(&b, "*tier: %s · score: %.2f*\n\n", tier, scores[e.ID])
		}
		// Principle 10 rendering: lead a mixed pattern with its checkable
		// handle; flag a constitutive pattern as an offered lens before the
		// gloss so it never reads as a finding.
		if fsKnown {
			switch fs.Status {
			case framestatus.Mixed:
				if fs.Handle != "" {
					fmt.Fprintf(&b, "**Checkable handle:** %s\n\n", fs.Handle)
				}
			case framestatus.Constitutive:
				fmt.Fprintf(&b, "**Offered lens, not a finding** — this grounds in judgment, not an external check. Hold it as a way of seeing to try on, and test it against your own case rather than taking it as established.\n\n")
			}
		}
		if gloss := patternGloss(e); gloss != "" {
			fmt.Fprintf(&b, "%s\n\n", gloss)
		}
		neighbors := pickAdjacencies(e, pool, maxAdjacencies)
		if len(neighbors) > 0 {
			fmt.Fprintln(&b, "**Nearby patterns:**")
			for _, n := range neighbors {
				ng := patternGloss(n)
				if ng == "" {
					fmt.Fprintf(&b, "- %s — %s\n", n.ID, n.Name)
				} else {
					fmt.Fprintf(&b, "- %s — %s. %s\n", n.ID, n.Name, ng)
				}
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// patternGloss returns a one-line user-readable description for an atom.
// Prefers first canonical-instance; falls back to the first evokes phrase.
// Truncated to keep neighbor lists scannable.
func patternGloss(e *types.LexEntry) string {
	if e == nil {
		return ""
	}
	if len(e.CanonicalInstances) > 0 {
		return truncateGreedy(e.CanonicalInstances[0], 140)
	}
	if len(e.Evokes) > 0 {
		return truncateGreedy(e.Evokes[0], 140)
	}
	return ""
}

// pickAdjacencies resolves an atom's related-list to LexEntry pointers,
// skipping self-references and missing pool entries, capped at max.
func pickAdjacencies(e *types.LexEntry, pool map[string]*types.LexEntry, max int) []*types.LexEntry {
	out := make([]*types.LexEntry, 0, max)
	for _, id := range e.Related {
		if id == e.ID {
			continue
		}
		neighbor, ok := pool[id]
		if !ok || neighbor == nil {
			continue
		}
		out = append(out, neighbor)
		if len(out) >= max {
			break
		}
	}
	return out
}

// runIntervene: V71 reaction-steering. Lens+gate the situation, keep the
// firing REACTIONS (the steerable transformations), and render each as
// reactants → product with the catalysts accelerating it and the
// inhibitors that would block it. The reaction tier's predict/intervene
// payoff: "what's this turning into, and where are the levers?"
func runIntervene(renderDir, contextStr string, topK int, noLens, explainFlag bool, format ...string) {
	outputFormat := "markdown"
	if len(format) > 0 && format[0] != "" {
		outputFormat = format[0]
	}
	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("%v", err)
	}
	allEntries := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		allEntries = append(allEntries, e)
	}

	candidatePool := allEntries
	var lensConfidences map[string]float64
	if !noLens && !lens.Disabled() {
		if c, err := client.New(); err != nil {
			fmt.Fprintf(os.Stderr, "lens: client init: %v (falling back to lexical full-pool)\n", err)
		} else {
			lensRes, lensErr := lens.Filter(context.Background(), contextStr, allEntries, c, false)
			switch {
			case lensErr != nil:
				fmt.Fprintf(os.Stderr, "lens: %v (falling back to lexical full-pool)\n", lensErr)
			case len(lensRes.Entries) == 0:
				fmt.Fprintln(os.Stderr, "lens: no semantically relevant primitives — falling back to lexical full-pool")
			default:
				fmt.Fprintf(os.Stderr, "lens: %d -> %d candidates\n", len(allEntries), len(lensRes.Entries))
				candidatePool = lensRes.Entries
				lensConfidences = lensRes.Confidences
			}
		}
	}

	// Frame-status (Principle 10): label each surfaced reaction's epistemic
	// posture, and let the gate down-weight constitutive candidates. Reactions
	// are typically constitutive (offered lenses) — the existing generous
	// `consider` slice below (built so reactions surface even when atoms
	// outrank them lexically) already absorbs this discount by design.
	fsMap, err := framestatus.Load(renderDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "frame-status: %v (rendering without frame labels)\n", err)
	}

	// Consider a generous top slice so reactions surface even when atoms
	// outrank them lexically, then keep the top `topK` reactions.
	consider := topK * 5
	if consider < 15 {
		consider = 15
	}
	results := gate.Run(gate.Input{
		Pool:        candidatePool,
		Context:     contextStr,
		TopK:        consider,
		Confidences: lensConfidences,
		FrameStatus: fsMap,
	})

	reactions := make([]*types.LexEntry, 0, topK)
	for _, r := range results {
		e := pool[r.PrimitiveID]
		if e != nil && e.Tier == "reaction" {
			reactions = append(reactions, e)
			if len(reactions) >= topK {
				break
			}
		}
	}

	if outputFormat == "json" {
		fmt.Println(formatInterveneJSON(contextStr, reactions, results, pool, fsMap))
		return
	}
	emitOutput(formatIntervene(contextStr, reactions, results, pool, fsMap), explainFlag)
}

// formatInterveneJSON returns the structured JSON shape of the reaction
// predictions. The shape is the agent-consumable contract for
// lexicon_predict — each reaction carries reactants/products/catalysts/
// inhibitors/conditions/mechanism alongside the per-atom gloss and
// adjacencies. Stable enough for consumers to pin to specific keys.
func formatInterveneJSON(contextStr string, reactions []*types.LexEntry, allResults []types.GateResult, pool map[string]*types.LexEntry, fsMap framestatus.Map) string {
	type reactionDoc struct {
		ID            string   `json:"id"`
		Name          string   `json:"name"`
		Tier          string   `json:"tier"`
		FrameStatus   string   `json:"frame_status,omitempty"`
		FrameHandle   string   `json:"frame_handle,omitempty"`
		Mechanism     string   `json:"mechanism,omitempty"`
		Reversibility string   `json:"reversibility,omitempty"`
		Reactants     []string `json:"reactants,omitempty"`
		Products      []string `json:"products,omitempty"`
		Catalysts     []string `json:"catalysts,omitempty"`
		Inhibitors    []string `json:"inhibitors,omitempty"`
		Conditions    []string `json:"conditions,omitempty"`
		Gloss         string   `json:"gloss,omitempty"`
	}
	type fallback struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Tier  string  `json:"tier"`
		Score float64 `json:"score"`
	}
	type out struct {
		Context         string        `json:"context"`
		Reactions       []reactionDoc `json:"reactions"`
		FallbackMatches []fallback    `json:"fallback_matches,omitempty"`
	}

	docs := make([]reactionDoc, 0, len(reactions))
	for _, e := range reactions {
		d := reactionDoc{
			ID:            e.ID,
			Name:          e.Name,
			Tier:          e.Tier,
			Mechanism:     e.Mechanism,
			Reversibility: e.Reversibility,
			Reactants:     e.Reactants,
			Products:      e.Products,
			Catalysts:     e.Catalysts,
			Inhibitors:    e.Inhibitors,
			Conditions:    e.Conditions,
			Gloss:         patternGloss(e),
		}
		if fs, ok := fsMap.Lookup(e.ID); ok {
			d.FrameStatus = string(fs.Status)
			d.FrameHandle = fs.Handle
		}
		docs = append(docs, d)
	}

	var fb []fallback
	if len(reactions) == 0 {
		for i, r := range allResults {
			if i >= 3 {
				break
			}
			e := pool[r.PrimitiveID]
			if e == nil {
				continue
			}
			fb = append(fb, fallback{ID: e.ID, Name: e.Name, Tier: e.Tier, Score: r.Score})
		}
	}

	doc := out{
		Context:         strings.TrimSpace(contextStr),
		Reactions:       docs,
		FallbackMatches: fb,
	}
	buf, err := jsonMarshalIndent(doc)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return buf
}

func formatIntervene(contextStr string, reactions []*types.LexEntry, allResults []types.GateResult, pool map[string]*types.LexEntry, fsMap framestatus.Map) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Intervene — %s\n\n", strings.TrimSpace(contextStr))

	if len(reactions) == 0 {
		b.WriteString("No reaction fired on this situation — only atoms/molecules matched, so nothing is mid-transformation to steer. Top matches:\n")
		shown := 0
		for _, r := range allResults {
			e := pool[r.PrimitiveID]
			if e == nil {
				continue
			}
			fmt.Fprintf(&b, "- %s **%s** (%s, %.2f)\n", e.ID, e.Name, e.Tier, r.Score)
			if shown++; shown >= 3 {
				break
			}
		}
		return b.String()
	}

	for _, e := range reactions {
		fmt.Fprintf(&b, "## %s — %s\n", e.ID, e.Name)
		if fs, ok := fsMap.Lookup(e.ID); ok {
			fmt.Fprintf(&b, "*frame: %s*\n\n", fs.Label())
			if fs.Status == framestatus.Constitutive {
				fmt.Fprintf(&b, "**Offered lens, not a finding** — the intervention slots below are ways of seeing the situation. Test against your own case rather than treating them as established levers.\n\n")
			} else if fs.Status == framestatus.Mixed && fs.Handle != "" {
				fmt.Fprintf(&b, "**Checkable handle:** %s\n\n", fs.Handle)
			}
		}
		if e.Mechanism != "" {
			fmt.Fprintf(&b, "**firing:** %s\n\n", truncateGreedy(e.Mechanism, 280))
		}
		row := func(label string, vals []string) {
			if len(vals) > 0 {
				fmt.Fprintf(&b, "- **%s:** %s\n", label, joinSlots(vals, pool))
			}
		}
		row("reactants (what's being transformed)", e.Reactants)
		row("conditions (true here?)", e.Conditions)
		row("product (where it's heading)", e.Products)
		row("catalysts (accelerating it — the levers)", e.Catalysts)
		row("inhibitors (would block it — the intervention)", e.Inhibitors)
		if e.Reversibility != "" {
			fmt.Fprintf(&b, "- **reversibility:** %s\n", truncateGreedy(e.Reversibility, 200))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// joinSlots renders reaction-slot values: lex-NNNN ids get their name
// appended from the pool; free-text slots are truncated. Semicolon-joined.
func joinSlots(vals []string, pool map[string]*types.LexEntry) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		if strings.HasPrefix(v, "lex-") && !strings.ContainsAny(v, " \t") {
			if e, ok := pool[v]; ok {
				parts = append(parts, fmt.Sprintf("%s (%s)", v, e.Name))
				continue
			}
		}
		parts = append(parts, truncateGreedy(v, 160))
	}
	return strings.Join(parts, "; ")
}

// runGreedy: V31 interactive REPL prototype. At each level: lens+gate top-K,
// user picks one, picks a probe-back (card's critical-questions or cobra-
// fallback from lex-prwfm), types answer; answer rolls into next-level
// context. Linear single trajectory.
func runGreedy(renderDir, contextStr string, depth, topK int, noLens bool) {
	elementsDir := filepath.Join(renderDir, loader.DefaultElementsDir)
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("%v", err)
	}
	allEntries := make([]*types.LexEntry, 0, len(pool))
	for _, e := range pool {
		allEntries = append(allEntries, e)
	}

	in := bufio.NewReader(os.Stdin)
	transcript := make([]whatIfStep, 0, depth)
	currentContext := contextStr

	// Load once, outside the per-level loop — the register file is static
	// across the whole REPL session, unlike currentContext/lensRes which
	// are genuinely different each level. Fail-soft: missing register just
	// drops frame-status down-weighting for the whole session.
	fsMap, fsErr := framestatus.Load(renderDir)
	if fsErr != nil {
		fmt.Fprintf(os.Stderr, "frame-status: %v (running without frame down-weighting)\n", fsErr)
	}

	fmt.Printf("five-what-ifs greedy · depth=%d · top-k=%d · lens=%v\n", depth, topK, !noLens)
	fmt.Printf("starting situation: %s\n", currentContext)

	for d := 0; d < depth; d++ {
		fmt.Printf("\n──────── level %d/%d ────────\n", d+1, depth)

		candidatePool := allEntries
		var lensConfidences map[string]float64
		if !noLens && !lens.Disabled() {
			if c, err := client.New(); err != nil {
				fmt.Fprintf(os.Stderr, "lens: client init: %v (falling back to lexical full-pool)\n", err)
			} else {
				lensRes, lensErr := lens.Filter(context.Background(), currentContext, allEntries, c, false)
				switch {
				case lensErr != nil:
					fmt.Fprintf(os.Stderr, "lens: %v (falling back to lexical full-pool)\n", lensErr)
				case len(lensRes.Entries) == 0:
					fmt.Fprintln(os.Stderr, "lens: no semantically relevant primitives at this level — falling back to lexical full-pool")
				default:
					fmt.Fprintf(os.Stderr, "lens: %d -> %d candidates\n", len(allEntries), len(lensRes.Entries))
					candidatePool = lensRes.Entries
					lensConfidences = lensRes.Confidences
				}
			}
		}

		results := gate.Run(gate.Input{
			Pool:        candidatePool,
			Context:     currentContext,
			TopK:        topK,
			Confidences: lensConfidences,
			FrameStatus: fsMap,
		})
		if len(results) == 0 {
			fmt.Println("(no candidates surfaced; stopping)")
			break
		}

		for i, r := range results {
			entry := pool[r.PrimitiveID]
			name := "?"
			canon := ""
			if entry != nil {
				name = entry.Name
				if len(entry.CanonicalInstances) > 0 {
					canon = truncateGreedy(entry.CanonicalInstances[0], 100)
				}
			}
			fmt.Printf("  %d. %s  %s  (%.2f)\n     %s\n", i+1, r.PrimitiveID, name, r.Score, canon)
		}

		pickIdx, quit := promptPick(in, "pick card", 1, len(results))
		if quit {
			break
		}
		picked := pool[results[pickIdx-1].PrimitiveID]
		if picked == nil {
			fmt.Println("(picked entry not in pool; stopping)")
			break
		}

		probes := picked.CriticalQuestions
		probeSource := "card.critical-questions"
		if len(probes) == 0 {
			probes = cobraFallbackProbes
			probeSource = "cobra-fallback (lex-prwfm)"
		}
		fmt.Printf("\nprobes (%s):\n", probeSource)
		for i, p := range probes {
			fmt.Printf("  %d. %s\n", i+1, p)
		}
		fmt.Printf("  %d. (custom — type your own)\n", len(probes)+1)

		probeIdx, quit := promptPick(in, "pick probe", 1, len(probes)+1)
		if quit {
			break
		}
		var probeText string
		if probeIdx <= len(probes) {
			probeText = probes[probeIdx-1]
		} else {
			probeText = promptLine(in, "custom probe")
			if probeText == "" {
				fmt.Println("(empty probe; stopping)")
				break
			}
		}

		answer := promptLine(in, "answer")
		if answer == "" {
			fmt.Println("(empty answer; stopping)")
			break
		}

		transcript = append(transcript, whatIfStep{
			Depth:  d + 1,
			CardID: picked.ID,
			Name:   picked.Name,
			Probe:  probeText,
			Answer: answer,
		})

		currentContext = fmt.Sprintf("%s\nthen via %s: %s — %s", currentContext, picked.Name, probeText, answer)
	}

	fmt.Println("\n════════ transcript ════════")
	fmt.Printf("starting: %s\n", contextStr)
	for _, s := range transcript {
		fmt.Printf("[%d] %s (%s)\n    probe:  %s\n    answer: %s\n", s.Depth, s.Name, s.CardID, s.Probe, s.Answer)
	}
	if len(transcript) == depth {
		fmt.Printf("\n(reached depth %d)\n", depth)
	} else {
		fmt.Printf("\n(stopped at depth %d)\n", len(transcript))
	}
}

func promptPick(in *bufio.Reader, label string, lo, hi int) (int, bool) {
	for {
		fmt.Printf("\n%s [%d-%d, q to quit]: ", label, lo, hi)
		line, err := in.ReadString('\n')
		if err != nil {
			return 0, true
		}
		line = strings.TrimSpace(line)
		if line == "q" || line == "Q" {
			return 0, true
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= lo && n <= hi {
			return n, false
		}
		fmt.Printf("  invalid; need integer in [%d, %d] or q\n", lo, hi)
	}
}

func promptLine(in *bufio.Reader, label string) string {
	fmt.Printf("%s: ", label)
	line, err := in.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

func truncateGreedy(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
