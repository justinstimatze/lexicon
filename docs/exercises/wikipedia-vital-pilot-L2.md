# Wikipedia Vital Articles — "express it in lexicon only" exercise — Level 2 (100 topics)

**What this is:** the Level-1 pilot (`wikipedia-vital-pilot-L1.md`) scaled to the full Vital-Articles **Level 2** set (~100 canonical knowledge-spine topics). For each topic, the *real* lead paragraph was pulled from the local Wikipedia ZIM (`lexicon zim-fetch --raw <title>` → gozimhttpd, `wikipedia_en_all_nopic_2025-12.zim`) — **not** a hand-gloss, unlike L1 — and fed through `lexicon gate --top-k 6` over all 356 atoms (`LEXICON_LENS_TIMEOUT_MS=30000`). No atoms minted or edited. Raw results were written to a scratch file outside the repo (100 lines, 100/100 real ZIM leads).

## Headline: the coverage histogram

The L1 pilot graded by a confidence-band gradient. At n=100 that proved **too generous** — raw lens confidence counts a topic as "matched" even when the only atoms firing are *representation meta-patterns* (map-not-territory, coarse-graining, spotlight-choice) that apply to literally any describable subject. So the truer axis is **deep vs shallow vs zero**:

| Class | Definition | Count |
|---|---|---|
| **DEEP** | ≥1 *domain-dynamics* atom fired (evolution, feedback, collective-action, gradient, …) | **73** |
| **SHALLOW** | matched, but *only* representation-meta atoms fired (map-territory / coarse-graining / spotlight / cognitive-environment / picture-theory / axis-decomposition) | **13** |
| **ZERO** | lens returned "no semantically relevant primitives" | **14** |

(The old conf-band view, for reference: STRONG ≥0.8 = 33, MODERATE 0.6–0.8 = 52, ZERO = 14, lens-error fallback = 1. Note Chemistry / Chemical-element / Matter scored conf 0.75–0.82 yet are SHALLOW — which is exactly why confidence overstates coverage and the deep/shallow split is the metric to trust.)

### By category (deep / shallow / zero, n per category)

| Category | n | DEEP | SHALLOW | ZERO | Read |
|---|---|---|---|---|---|
| **Arts** | 6 | 6 | 0 | 0 | full deep — aesthetic/creative dynamics |
| **Philosophy & religion** | 6 | 6 | 0 | 0 | full deep — reasoning-about-reasoning is home turf |
| **Society** | 23 | 20 | 2 | 1 | ~87% deep — collective-action / rights / power / structure |
| **History** | 7 | 6 | 1 | 0 | deep — causal inference + process-tracing |
| **Mathematics** | 6 | 5 | 1 | 0 | better than L1 implied; only Arithmetic shallow |
| **Everyday life** | 6 | 4 | 1 | 1 | mixed — activities deep, artifacts thin |
| **Technology** | 5 | 3 | 1 | 1 | weakest non-physical — adoption/diffusion gap |
| **Science** | 30 | 19 | 5 | 6 | **split** — see below |
| **Geography** | 11 | 4 | 2 | 5 | **most non-deep** — places & landmasses |

## The finding, sharpened (L2 anchors what L1 only suggested)

L1 (n=10) produced the claim *"elements/ is a library of reasoning/dynamics patterns, not a content ontology."* That claim was load-bearing but unanchored. L2 (n=100) both **confirms and refines** it:

**1. The boundary is entity/substance vs process/dynamic — and it cuts *within* categories, not just between them.** Science is the clean demonstration: process/dynamic topics express deeply (Evolution, Ecology, Disease, Death, Reproduction, Energy, Time, Geology, Medicine), while *entity* and *substance* topics go zero or shallow:

- **Science ZERO (6):** Sun, Earth, Animal, Water, Climate, Physics
- **Science SHALLOW (5):** Chemistry, Chemical element, Matter, Electricity, Death-adjacent
- This is the **L1 "Earth = zero" diagnostic reproduced at scale.** A proper-noun particular or a lump of matter is the thing patterns are patterns *of* (Goertzel) — not itself a compressible regularity. Reframe the entity to its processes and coverage reappears (L1 proved this: plate tectonics / water cycle / erosion all matched 0.7–0.92).

**2. Geography is the other non-deep cluster, for the same reason:** Land, Ocean, Africa, Asia, Oceania all returned ZERO (place-particulars / landmasses); only the *discipline* "Geography" and settlement/method topics matched, and those only shallowly. Places are particulars, not patterns.

**3. The "matched" rate is inflated by three universal-solvent atoms.** The five most-fired atoms across all 100 topics are *all* epistemic/representational meta-patterns:

| Atom | Hits | What it is |
|---|---|---|
| lex-68abd map-not-territory | 43 | representation ≠ referent |
| lex-u74ex spotlight-choice | 38 | selective attention |
| lex-4yhqs coarse-graining | 38 | abstraction-level choice |
| lex-e2tnt cognitive-environment-as-scope | 19 | relevance-bounding |
| lex-3r2f8 picture-theory-of-meaning | 16 | structure-isomorphism |

These apply to *anything you can describe*, so they fire almost everywhere — which is precisely why a topic can show high confidence yet be a shallow match. Discounting them is what separates the genuine 73% from the 13% representational veneer. (Elements/'s *most reusable* primitives being about **how we represent** rather than **what is represented** is itself coherent with a transfer-tool design — the high-reuse core is meta-cognitive.)

**Net:** the L1 claim survives in sharpened form — **deep expression tracks process / dynamics / inquiry / social-structure; shallow-or-zero tracks material substance, formal content, and proper-noun particulars.** That is the patternist boundary, drawn at 73 / 13 / 14, and it is the *right* boundary for a transfer tool.

## Gaps surfaced (the three L1 gaps, confirmed at scale)

1. **Physical/material-science content** — the single largest non-deep cluster (Science: 6 zero + 5 shallow; Geography: 5 zero). Almost certainly an intentional design boundary, **but** the L1 caveat stands: physical-*dynamics* atoms exist yet are thinner and more `under-review` than social/epistemic ones. A dedicated physical-science pattern-source pass would deepen the entity-adjacent processes *if* breadth there is wanted — a balance choice, not a hole.
2. **Technology-adoption / diffusion** — Technology is the weakest non-physical category (3 deep / 1 shallow / 1 zero; Transport zeroed). Elements/ has invention/TRIZ tricks but not spread + tool-human coevolution. Rogers 1962 (*Diffusion of Innovations*) remains the named candidate.
3. **Formal mathematics** — softer than L1 suggested (Mathematics is 5 deep / 1 shallow), but Arithmetic shallow + the absence of proof/axiomatization primitives persists; likely partly addressed by a future computation / cellular-automata pass.

## Molecule-granularity caveat (recorded, deferred)

The gate returns a **flat bag of top-k atoms**, so a topic carried by 4 co-firing atoms reads the same as one carried by 1. Many DEEP topics here are really **molecules** — Science ≈ falsification + experimental-design + paradigm-crisis + triangulation; Government / War / Religion likewise composed. Molecule-granularity expression would likely reclassify some SHALLOW topics upward (a topic that looks thin at atom-granularity may be a coherent molecule) and sharpen the DEEP ones. Build atom-level coverage first (done); layer molecule-composition later. Memory: `wikipedia-vital-articles-utility-exercise`.

## Method notes

- **Real ZIM leads, 100/100** (vs L1's hand-glosses) — the `gozimhttpd` + `zim-fetch --raw` path is the working scaler; lead extraction = first substantial `<p>` after stripping infobox/style/script.
- **Use the shallow/deep split, not raw confidence, as the coverage metric** at L3 — raw confidence overstated by ~13 points here.
- **L3 (1000 topics)** is a Workflow fan-out job (pipeline over topics → gate → tabulate). Needs explicit multi-agent opt-in; not auto-launched. The L3 deliverable would be the full quantified map: *what fraction of canonical human knowledge elements/ expresses, by topic-type* — with the L2 finding as the prior (expect ~70% deep, concentrated away from material/place/formal-content).
