# Reaction tier — design (2026-05-28)

## Why a third tier

The chemistry frame had two first-class entities: **atoms** (single cognitive moves) and **molecules** (named assemblies of atoms via the bond grammar — `sequential`/`choice`/`parallel`/`iteration`/`conditional`/`scoping`). urfaced that a large share of `_tier: molecule` entries don't fit: they are **causal transformations** ("X amplifies / routes-to / sustains Y"), not static bonded structures.

A **molecule is a structure** — atoms that co-occur / co-constitute a stable compound. A bond says how parts *stay together*.

A **reaction is a transformation** — input states turn into output states via a mechanism, modulated by catalysts/inhibitors and gated by conditions. The bond grammar can't express this (it has no "A amplifies B" operator, and the reactant/product atoms have non-chaining types — a `sequential` over them type-errors). That mismatch is the diagnostic that an entry is a reaction, not a molecule.

This tier exists to support the **predict / intervene** capability (the "cool vision"): given a situation, the lens diagnoses *which transformation is firing*, predicts its product, and surfaces the **catalyst** that accelerates it and the **inhibitor** that would block it — a steering tool, not just a labeling deck. Molecules answer "what is this?"; reactions answer "what is this turning into, and where are the intervention points?".

## Schema (slots)

A `_tier: reaction` entry keeps the common fields (`id`, `name`, `type-in`, `type-out`, `related`, `evokes`, `lineage`, `critical-questions`, `canonical-instances`, `status`) and replaces molecule's `decomposes-into:`/`assembly:` with:

| Slot | Meaning | Chemistry analogue |
|---|---|---|
| `reactants:` | input atoms/states consumed or transformed | reactants |
| `products:` | output atom/state produced | products |
| `mechanism:` | the step-pathway reactants→products (free-text or DSL; NOT type-checked like `assembly:`, because reaction pathways legitimately cross types) | reaction mechanism |
| `catalysts:` | atoms that **accelerate/enable** the reaction without being consumed (this is the "amplifies" relation) | catalyst |
| `inhibitors:` | atoms/conditions that **dampen or block** it | inhibitor / poison |
| `conditions:` | triggering context required for the reaction to fire | temperature / pressure |
| `reversibility:` | one-shot vs reversible vs reaches equilibrium | ⇌ / equilibrium |

`reactants`/`catalysts`/`inhibitors` reference `lex-NNNN` ids; those referenced atoms should appear in `related` so reciprocation holds.

## Molecule vs reaction — the test

- **Reaction** if it is a transformation that *happens to* a system: reactants → product, with modulators (envy+impotence → value-inversion; dissonance → hypervigilance; counterexample → refined-claim). You *diagnose* it.
- **Molecule** if it is a static configuration or a deliberate procedure you *point to or apply* (argument-from-expert-opinion lex-kebfa; causal-inference-method-selection lex-e9z2y; a co-occurring syndrome). It carries `assembly:` when the parts compose type-cleanly.

Borderline cases exist (a procedure is process-y); the operational cutoff is: **does it have catalysts/inhibitors that modulate a state-transformation?** If yes → reaction.

## Tooling impact

- **Loader / gates:** none. The loader tolerates the new tier (free string) and the new slots (ad-hoc YAML, per the 3-occurrence rule). `lexicon lint` only type-checks entries with an `assembly:` field, so reactions (which use `mechanism:`, not `assembly:`) are skipped — no false type errors. `db lint` + reciprocation operate on raw YAML + `related`, unaffected.
- **Render (follow-up, small):** `internal/modes/visual.go` and `meta_explanatory.go`'s `describeTier` branch on tier value; add a `reaction` case so reactions render first-class rather than generically. (A `compound` tier is already anticipated in visual.go — precedent for >2 tiers.)
- **Predict/intervene (later, the real feature):** promote the slots to `types.LexEntry` struct fields and add a `what-if --mode intervene` consumer that reads catalysts/inhibitors to surface intervention points.

## Migration

Re-tier the transformation-shaped molecules. First proof: lex-96xn9 (envy+impotence → value-inversion). Candidate batch: the dissonance/influence "X-routes-to/amplifies-Y" cluster (lex-etdx7, 0159, 0161, 0162, 0177) and lex-96xn9/0369; reassess lex-8c35v/0146/0157/0158/0160 case-by-case. Procedural/structural molecules (lex-kebfa, 0379, 0380, 0381, 0087, 0088, 0095) stay molecules.
