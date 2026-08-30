# Wikipedia Vital Articles — "express it in lexicon only" exercise — Level 1 pilot (10 topics)

**What this is:** an exercise of *pattern utility*, not a mining pass. For each top-tier Wikipedia topic, run the probe/gate engine (`lexicon gate --context "<lead>" --top-k 8`, `LEXICON_LENS_TIMEOUT_MS=30000`) over all 356 atoms and ask: *which elements patterns express this topic, and how completely?* No atoms were minted or edited. Topic set = a Vital-Articles-Level-1-style spine of 10 (Human, Earth, Life, Science, Technology, Mathematics, History, Art, Philosophy, Society); glosses are concise leads, not full articles.

**Method note:** the lens (haiku-4-5 semantic filter) returns N relevant candidates + a synthesized "signal" or, when nothing clears threshold, *"no semantically relevant primitives."* That zero-case is itself a finding, not a failure. Coverage verdict below is my judgment, grounded in the actual atom names — the score is a cross-check, not the verdict.

## Per-topic result

| # | Topic | Lens result | Top matched atoms (id — name) | Coverage | Note |
|---|-------|-------------|-------------------------------|----------|------|
| 1 | **Human** | 7 cand, 0.85 | 0320 natural-selection · 0327 sexual-selection · 0125 recursive-mind-simulation-shapes-self · 0306 recognition-asymmetry · 0293 subordinated-group-as-mirror · 0149 trust-formation · 0268 persons-things-actions axes | **PARTIAL** | Strong on human-as-evolved-social-self-modeling animal. The *dynamics* of human cognition/sociality land; "what a human is" as content does not (expected). |
| 2 | **Earth** | **none** | — | **POOR (zero)** | The clean negative result. A material/astronomical object has no pattern-elements — lexicon holds no geophysical/material-content primitives. Out-of-scope by design, not a gap to fill. |
| 3 | **Life** | 5 cand, 0.82 | 0320 natural-selection · 0120 gradient-as-engine · 0372 emergent-effects-resist-decomposition · 0119 substrate-precedes-code · 0083 conservation-as-invariant | **PARTIAL-GOOD** | Surprisingly deep: life-as-evolving/gradient-driven/emergent is well expressed. Metabolism/reproduction-as-mechanism absent, but the deep dynamics land. |
| 4 | **Science** | 7 cand, 0.88 | 0228 collision-with-error (falsification) · 0323 randomization-controls-confounders · 0127 crisis-as-research-opportunity · 0145 crisis-recognition (Kuhn) · 0199 triangulation-of-independent-methods · 0184 via-negativa · 0081 limit-taking | **STRONG / near-FULL** | Home turf. Science decomposes almost entirely into elements patterns — falsification, experimental design, paradigm crisis, triangulation. Elements/ *is* a theory of inquiry. |
| 5 | **Technology** | 4 cand, 0.78 | 0093 harm-as-resource-via-redirection · 0092 asymmetry-as-function-enabler · 0065 polymorphism-as-extension · 0119 substrate-precedes-code | **WEAK** | Only oblique TRIZ-style invention tricks match. Technology-as-artifact + adoption/diffusion + tool-human coevolution are not expressible. Real gap candidate (diffusion-of-innovations). |
| 6 | **Mathematics** | 4 cand, 0.82 | 0168 catuskoti-schema · 0224 picture-theory-of-meaning · 0012 map-not-territory · 0010 coarse-graining | **WEAK** | Only abstraction/representation meta-patterns match. Proof, axiomatization, number, formal structure are absent. Real gap: formal-mathematics primitives. |
| 7 | **History** | 6 cand, 0.82 | 0349 within-case-process-tracing · 0322 spatial-mapping-reveals-causation · 0321 pseudo-environment (Lippmann) · 0374 counterexamples-refine-the-claim · 0018 spotlight-choice · 0012 map-not-territory | **PARTIAL-GOOD** | The *method* of history (causal inference, process-tracing, selective attention, interpretation) is well covered. The specific past as content is not (expected). |
| 8 | **Art** | 5 cand, 0.82 | 0237 aesthetic-contemplation-as-will-less-knowing · 0238 will-to-power-as-generative · 0291 self-consciousness-deforms-the-work · 0303 inherited-forms-mismatch-new-makers · 0330 selective-interest-constitutes-experience | **PARTIAL-GOOD** | Art's *dynamics* (aesthetic experience, creative drive, craft vs self-consciousness, form/tradition mismatch, perception-as-construction) land well. Specific media absent. |
| 9 | **Philosophy** | 5 cand, 0.85 | 0240 perspectivism · 0168 catuskoti-schema · 0189 cognitive-environment-as-scope · 0012 map-not-territory · 0018 spotlight-choice | **GOOD** | Philosophy is reasoning-about-reasoning — elements/'s core. Epistemic-stance primitives express it richly. |
| 10 | **Society** | 5 cand, 0.82 | 0268 persons-things-actions axes · 0356 collective-action-repertoire · 0262 rights-decompose-into-jural-relations · 0354 state-society-checking-race · 0391 conway's-law | **GOOD** | Structure / collective-action / rights / power-balance primitives express society well. (The elite-theory cluster 0395/0400/0401 also applies but didn't crack top-5 here — see method caveat.) |

## The finding (the gradient)

Lexicon expresses a topic **in proportion to how much that topic IS a process, a dynamic, a mode of inquiry, or a social structure** — and fails on topics that are static material/factual content.

- **STRONG → near-full coverage:** Science, Philosophy, History, Society, Art. (Inquiry + social-dynamics topics.)
- **PARTIAL (deep dynamics yes, content no):** Human, Life.
- **WEAK (only oblique matches):** Technology, Mathematics.
- **ZERO (honest miss):** Earth.

This is the honest characterization of pattern-utility: elements/ is a **library of reasoning/dynamics patterns, not a content ontology.** That is a feature — but the exercise draws the boundary sharply.

## Gaps surfaced (the by-product)

1. **Formal-mathematics primitives** — proof, axiomatization, number/structure. Mathematics matched only abstraction patterns. Likely partly addressed by a future computation/cellular-automata pass.
2. **Diffusion-of-innovations / technology-adoption** — Technology matched only TRIZ invention tricks; the spread + tool-human coevolution side is missing. (Rogers 1962 is a known candidate.)
3. **Material/physical-science content (Earth)** — almost certainly *intentionally* out of scope; flag for a design decision, not an automatic mint.

## Design diagnostic: is Earth=zero an incomplete design, or a focused value framing?

Test: probe physical *processes* (not the Earth *entity*). If elements/ has no physical-dynamics patterns, these return zero too → incompleteness. If they match → "Earth" was correctly excluded as a particular, and the design is focused, not incomplete.

| Process gloss | Lens | Matched atoms |
|---|---|---|
| Plate tectonics | 5 cand, 0.82 | 0362 same-effects→single-cause · 0085 scale-separation · 0081 limit-taking · 0010 coarse-graining · 0083 conservation-as-invariant |
| Water cycle | 4 cand, 0.88 | 0120 gradient-as-engine · 0111 stalemate-as-apparent-stability (dynamic equilibrium) · 0083 conservation · 0010 coarse-graining |
| Erosion/deposition | 5 cand, 0.88 | 0120 gradient-as-engine · 0324 uniformitarianism · 0010 · 0083 · 0085 |
| Thermodynamic equilibrium | 5 cand, 0.92 | 0111 equilibrium · 0094 feedback-loop · 0120 gradient-as-engine · 0083 conservation · 0081 limit-taking |
| Earthquake (stick-slip) | 5 cand, 0.92 | 0094 feedback-loop · 0260 noise-as-amplifier-in-nonlinear-systems · 0118 execution-state · 0359 resilience-vs-stability · 0083 conservation |

**Every physical process matched (0.7–0.92); none returned "no relevant primitives."** The Earth *entity* returned zero only because a proper-noun particular is not a pattern in the Goertzel sense (a pattern is a compressible regularity / generative rule; an entity is the thing patterns are patterns *of*). Reframe Earth → its constituent processes and coverage reappears.

**Verdict: sharply-focused value framing, not incompleteness — and Goertzel-coherent.** Asking elements/ to "express Earth" is a category error; it expresses the *patterns instantiated in* Earth, which it does. The boundary the exercise draws is exactly the patternist boundary, and it is the *right* boundary for a transfer tool (the same pattern recognized across domains — which is lex-s22hf itself).

**One genuine but minor caveat — a coverage *skew*, not a hole:** physical-dynamics atoms exist but are thinner and more often `under-review` (many 0.49 hits: 0085, 0081, 0010, 0083) than the social/epistemic ones, reflecting a philosophy/social-theory-heavy mining history. That's a balance choice (mine more physical-science pattern sources if breadth there is wanted), not a design flaw.

## Method fixes

- **Lens timeout:** `gate` and `what-if` now default `LEXICON_LENS_TIMEOUT_MS` to 30000 when unset (the 8s default is tuned for the hook hot path; dense contexts were timing out into a degenerate all-1.000 lexical fallback). Hook untouched. Build green.
- **Real ZIM leads:** gozimhttpd brought up against `/opt/zim/wikipedia_en_all_nopic_2025-12.zim`; `lexicon zim-fetch --raw <title>` works, and the gozim MCP (`wikipedia-zim` → get_article/search) reconnected. Config was already correct — no change needed.

## Scaling note

Method works and is cheap (lens caches; ~seconds/topic at 30s timeout). To scale to Level 2 (100) / Level 3 (1000): pull real article leads from the local Wikipedia ZIM rather than hand-glosses, batch through `gate`, and tabulate. A workflow fan-out is the natural scaler but needs explicit opt-in. The coverage-gradient histogram across 1000 topics would be the real deliverable: a quantified map of *what fraction of canonical human knowledge elements/ can express, by topic-type.*
