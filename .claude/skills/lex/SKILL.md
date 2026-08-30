---
description: Identify the elements patterns operating in a situation, transcript, news story, decision, or passage of text the user pastes or describes. Surfaces the named atoms that apply, translates each into "here's the move" plain language, and flags the adjacency frontier (atoms the situation evokes but elements/ doesn't yet contain). Use whenever the user asks "what's going on here", "what patterns apply", "what should I do", or pastes content for analysis. Also use proactively on any sufficiently rich passage the user introduces.
---

# /lex — pattern recognition through elements

## What this does

Given a situation (pasted text, described scenario, transcript, news story, decision under consideration), surface the **elements primitives operating in it**, translate each into actionable language, and flag what the situation suggests but elements/ doesn't yet name (the adjacency frontier).

This is the lexicon "product loop." It is the *use* of elements/, not maintenance of it.

## How to run it

### 1. Identify

Call the MCP tool `mcp__lexicon__lexicon_read` with the situation as input. The tool returns a markdown analysis listing atoms that fire on the input, with confidence scores and verbatim atom material.

If MCP is unavailable, fall back to the CLI:

```bash
cd render && go run ./cmd/lexicon read --context "<situation>"
```

(run from the repo root of wherever lexicon is checked out)

The CLI emits the same shape of result.

### 2. Translate (don't expose the chemistry)

Per the `feedback_interpret_dont_expose_chemistry` project memory: surface the move, not the atom anatomy. The user does not want to read `lex-8ennw agent-instruction: …`. They want to hear:

> "Two things are operating here. First, [plain-language name of the pattern]: [one-sentence summary of why it applies, citing the situation's specifics]. The move is [practitioner-actionable advice from the atom's agent-instruction]. Second, …"

Pick the **2-4 most load-bearing** atoms — the ones whose absence would change the user's read of the situation. Suppress the long tail. If 7 atoms fire weakly, do not list 7.

For each, include a short verbatim phrase from the atom's `evokes` or `canonical-instances` list when it sharpens the read. Cite `(lex-NNNN)` once per atom so the user can `/lexicon read lex-NNNN` to drill in.

### 3. Frontier (optional, only when it helps)

If the situation gestures at something the surfaced atoms don't fully cover, call `mcp__lexicon__lexicon_extrapolate` with the constellation of surfaced atom ids. It returns the adjacency frontier — atoms ranked by how many of the constellation point at them.

The frontier answers: "given these patterns I've named, what's adjacent that I might want next?" Useful when:
- The user is exploring (not just looking for an answer)
- The situation seems to be evoking a pattern elements/ doesn't yet have an atom for (potential mint candidate)
- The user explicitly asks for adjacencies or "what else"

Do NOT run the frontier on every invocation — it adds noise when the user just wants the move.

### 4. Flag gaps honestly

Per `feedback_proactive_clarification` and `feedback_ugly_intermediate`: if the situation evokes something elements/ genuinely lacks, **say so**. "There's no atom yet for the way X works here — the closest is lex-NNNN but it doesn't quite capture the asymmetry." That's useful intelligence, both for the user and for the next mining pass.

Do not silently invent an atom that doesn't exist. Do not over-confidently name a pattern when the actual fit is weak.

## What NOT to do

- **Do not list every atom that fires.** 2-4 load-bearing > 10 weak ones. The product is the read, not the catalog.
- **Do not expose the YAML field names.** No "agent-instruction:", no "type-in:", no `_tier:`. The user is here for pattern recognition, not the schema.
- **Do not run extrapolate by default.** Only when the user is exploring or when the frontier genuinely adds value.
- **Do not invent atoms.** If elements/ doesn't have it, say so and stop.
- **Do not lecture.** The user already knows their situation. Name the pattern, point at the move, get out of the way.

## Worked example

User: *"My team keeps shipping features nobody uses. Every quarter we ship more, every quarter usage barely moves. The PM keeps saying 'we need to ship faster.'"*

Good response (sketch):

> Two patterns running together here:
>
> **Goodhart on velocity.** "Ship faster" was an organizational metric chosen as a proxy for "delivering value." Now the metric is the target — features-shipped goes up while user-value doesn't move. This is the canonical Goodhart failure mode (lex-tb3re). The move: redefine the operational target as something downstream of shipping — adoption, retention, or a specific user outcome — and watch what changes about which features get prioritized.
>
> **The shape of the activity replaced its purpose.** Performing-the-work has supplanted accomplishing-the-work; the team is optimizing the ritual of velocity, not the consequence (lex-0XXX — feels like a refinement of lex-6gy96 value-capture but worth checking).
>
> Adjacent that elements/ may not name yet: the *PM-side incentive* — they ship a feature, they get credit; they refuse to ship, they get nothing. That asymmetry is the engine. Elements/ may have it under elite-overproduction (lex-dqa9j) at a different scale; not a clean fit at the team scale.

## Related tooling

- `/lexicon-mint` — for *adding* an atom (maintainer side; not what end users invoke)
- `mcp__lexicon__lexicon_read` — primitive: returns atom analysis as JSON / markdown
- `mcp__lexicon__lexicon_extrapolate` — primitive: returns adjacency frontier as JSON
- `mcp__lexicon__lexicon_list` — primitive: enumerate atoms with filters

## Reference

Full project conventions in `CLAUDE.md` at the repo root. Elements/ is a "moving frontier extension tool" (per `project_lexicon_as_moving_frontier_tool`), not a permanent foundation — patterns get added, refined, and occasionally deprecated. When elements/ doesn't have a clean atom for the situation, that's data, not a failure.
