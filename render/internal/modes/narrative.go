package modes

import (
	"context"
	"fmt"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// systemPrompt is the load-bearing render-instruction that distinguishes
// lexicon's narrative mode from a generic LLM call. Constraints: 100-300
// words, second-person, weave defeaters as critical-questions, do NOT
// surface the entry name (the user shouldn't have to know the canonical
// label to use the move).
const systemPrompt = `You are lexicon's render function in narrative mode.

Your job: take a typed cognitive primitive (a "lex entry") plus the user's context, and produce a short narrative (100-300 words) that makes the primitive deployable in the user's moment of need.

Constraints:
- Second-person ("you"). Imagine the user is mid-cognitive-bind on the situation described in the context.
- 100-300 words. Time-shaped (a sequence of moves), not structure-shaped (a decomposition).
- Do NOT name the entry or reveal its lex-NNNN id. Just demonstrate the move. The user shouldn't have to know the canonical name.
- If the entry has critical-questions, surface 1-2 of them naturally as defeaters the user should hold ready — woven into the prose, not as a bullet list.
- Match the user's working register if vocabulary hints are provided.
- Output ONLY the narrative. No preamble, no meta-commentary, no markdown headers.`

// Narrative renders a deployment-mode prose paragraph via LLM. Takes a
// client.Client for testability — production code passes the real
// SDK-backed client; tests pass a fake.
func Narrative(
	ctx context.Context,
	c client.Client,
	e *types.LexEntry,
	userContext string,
	workingVocab []string,
) (types.RenderOutput, error) {
	userMsg := buildUserMessage(e, userContext, workingVocab)
	resp, err := c.CreateMessage(ctx, client.MessageRequest{
		System:    systemPrompt,
		UserText:  userMsg,
		MaxTokens: 1024,
	})
	if err != nil {
		return types.RenderOutput{}, err
	}
	trace := fmt.Sprintf(
		"model=%s input_tokens=%d output_tokens=%d",
		client.Model, resp.InputTokens, resp.OutputTokens,
	)
	return types.RenderOutput{
		PrimitiveID:        e.ID,
		Mode:               types.ModeNarrative,
		Text:               resp.Text,
		IntrospectionTrace: trace,
	}, nil
}

func buildUserMessage(e *types.LexEntry, ctx string, vocab []string) string {
	var lines []string
	add := func(s string) { lines = append(lines, s) }

	add(fmt.Sprintf("Entry name (do not surface to user): %s", e.Name))
	add(fmt.Sprintf("Type signature: %s → %s", e.TypeIn, e.TypeOut))
	add(fmt.Sprintf("Tier: %s", e.Tier))
	if len(e.Evokes) > 0 {
		add("Gestural near-synonyms (for shape-grasp): " + strings.Join(e.Evokes, ", "))
	}
	if len(e.Premises) > 0 {
		add("Premise structure:")
		for _, p := range e.Premises {
			add("  - " + p)
		}
	}
	if len(e.CriticalQuestions) > 0 {
		add("Critical questions (defeaters):")
		for _, q := range e.CriticalQuestions {
			add("  - " + q)
		}
	}
	if len(e.CanonicalInstances) > 0 {
		add("Canonical example: " + e.CanonicalInstances[0])
	}
	add("")
	add("User's context: " + ctx)
	if len(vocab) > 0 {
		add("User's working vocabulary hints: " + strings.Join(vocab, ", "))
	}
	return strings.Join(lines, "\n")
}
