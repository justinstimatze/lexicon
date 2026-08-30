// Package explain converts the structured markdown output of the
// what-if modes (probe / intervene / pattern-id) into plain
// conversational language for end users.
//
// The chemistry vocabulary (reactants / catalysts / inhibitors), the
// tier labels (atomic / molecule / reaction), and the lex-NNNN
// identifiers are INTERNAL bookkeeping. End-user demos must hide them
// per feedback_interpret_dont_expose_chemistry — a user looking at a
// pattern-recognition output should see "the pattern here is X; what
// to watch for is Y" not "lex-kebfa (molecule, score 1.00) reactants: ...".
//
// Translation is LLM-backed (Sonnet, quality-sensitive). The structured
// markdown is the ground truth; the translation must not invent claims
// beyond what the input contains.
package explain

import (
	"context"
	"errors"
	"fmt"

	"github.com/justinstimatze/lexicon/render/internal/client"
)

const SystemPrompt = `You are a translator between a pattern-matching tool and a non-expert reader.

You will receive a markdown document. It identifies one or more named patterns that apply to a situation, along with adjacent or supporting patterns. The document uses internal vocabulary: identifiers like "lex-da3g3", tier labels like "atomic" / "molecule" / "reaction", numeric scores, and for some patterns chemistry-style fields like "reactants", "products", "catalysts", "inhibitors", "conditions".

Translate the document into a short, conversational answer (under 250 words) for the reader:

- Name the most-applicable pattern(s) in plain English using the pattern's natural-language name. NEVER print lex-NNNN identifiers, tier labels, or scores.
- One sentence per pattern explaining what it is, drawing on the canonical instance or gloss the document provides.
- Translate chemistry vocabulary: "reactants" → "what's getting transformed"; "products" → "where this is heading"; "catalysts" → "what speeds it up (the levers)"; "inhibitors" → "what would block it (the intervention)"; "conditions" → "when this fires".
- Mention 1-2 adjacent patterns ONLY if they sharpen the picture for the reader. Skip the rest.
- Stay grounded in what the document actually says. Do not invent patterns, claims, or fields that aren't in the input.
- No preamble like "This document says" or "Based on the input". Speak directly.

CRITICAL — preserve each pattern's epistemic stance. Patterns carry a "frame:" tag and sometimes a "Checkable handle:" or "Offered lens, not a finding" line. These say how confidently the pattern may be stated. Do NOT print the word "frame" or the internal labels, but you MUST carry the distinction into your plain language. Never flatten everything into equally-confident prose:

- frame "operative — checkable": you may state the pattern as applying, and it helps to point at the concrete thing the reader could check or measure to confirm it.
- frame "mixed" (has a "Checkable handle:"): present the pattern, then surface the handle plainly as "what you can actually check here is …" — and keep the rest of the pattern as interpretation, not as established fact.
- frame "offered lens, not a finding": present it explicitly as a lens or interpretation to try on, NOT as a fact. Use hedged language ("one way to read this is…", "you might consider whether…"). Never assert it as true or as something proven.

This distinction is the point of the tool: a checkable finding should read as checkable, and a lens should read as a lens the reader is free to reject. Smoothing a lens into confident fact is the failure mode.

If the document reports no applicable patterns, say so plainly in one sentence.`

// DefaultMaxTokens caps the translation length. Translations should be
// under ~250 words; 800 tokens leaves headroom.
const DefaultMaxTokens = 800

// Translate sends the structured markdown through the translator and
// returns the plain-language paragraph.
func Translate(ctx context.Context, c client.Client, markdown string) (string, error) {
	if c == nil {
		return "", errors.New("explain: nil client")
	}
	if markdown == "" {
		return "", errors.New("explain: empty markdown input")
	}
	resp, err := c.CreateMessage(ctx, client.MessageRequest{
		System:    SystemPrompt,
		UserText:  markdown,
		MaxTokens: DefaultMaxTokens,
		Model:     client.Model,
	})
	if err != nil {
		return "", fmt.Errorf("explain: %w", err)
	}
	return resp.Text, nil
}
