# Lexicon elements-design principles

*Elements-META principles (principles ABOUT the elements), distinct from elements-CONTENT (atoms and molecules IN the elements). Principles 1-7 distill Goertzel's own consolidation pass over his source material; principle 8 distills the project's own graduation conventions. The Goertzel-derived principles survived a critical-filter pass — they are NOT load-bearing for Goertzel's grandiose-AGI claims; they ARE load-bearing for lexicon's own evaluative purposes.*

*This doc is an elements-quality artifact, not elements content. It changes how we evaluate atom additions, mining passes, and elements growth — but it doesn't add atoms.*

## Why these principles, not others

Goertzel 2006 *The Hidden Pattern* offered ~30 candidate elements-meta principles across 22 chapters. The 7 Goertzel-derived principles below survived a critical-filter pass that:

1. **Rejected** infection-risk material (quantum mind, Novamente-architectural prescriptions, universal-mind cosmicism, singularity/post-human framings, Bateson magic-numbers as principles)
2. **Lifted** material that is operationally crisp + non-duplicate of existing lex content + survives the "humble + analytical" register test (vs the "grandiose + self-vindicating" register that produced rejection)
3. **Distinguished** principles that fit the atom pattern (added → lex-e5des dialogical-vs-monological-belief-system) from principles that DO NOT fit the atom pattern (the 7 below — they're principles for evaluating atoms, not atoms themselves)

Elements format + assembly is already validated by use; these principles formalize the evaluation discipline that's been operating informally since early in the project.

## The seven principles

### 1. Pattern as representation as something simpler

**Source:** Goertzel 2006 *The Hidden Pattern* App. A p.356 (verified verbatim from PDF). Lineage: Goertzel attributes the basic concept to algorithmic information theory (Chaitin 1987) + his own previous work *From Complexity to Creativity*.

**Formulation:** A process p is a pattern in entity e iff (a) the result rp of p is a good approximation of e, AND (b) p is simpler than e.

**Application to lex:** Each lex atom IS (or should be) a candidate pattern in this formal sense. The atom NAME + canonical-instances + critical-questions are a "process" that, when fired correctly on a prompt, produces a recognized cognitive phenomenon (the entity e) as something simpler. Atom QUALITY = quality of pattern-fit.

**Operational test:** for each atom, ask:
- Does the atom NAME + lead-canonical-instance reliably produce the cognitive phenomenon it names? (accuracy)
- Is the atom-encoding actually simpler than the phenomenon it names? (compression)
- If either fails, the atom is not a pattern in Goertzel's sense — it's noise, decoration, or premature formalization.

### 2. Pattern intensity = accuracy × compression

**Source:** Goertzel 2006 App. A p.361 (verbatim).

**Formulation:** `IN(p,e) = (1 - d(rp,e)/c(e)) × ((c(e) - c(p))/c(e))^+`. The two terms multiply. The superscript `+` clips negative values to zero.

**Application to lex:** Quantitative evaluation of atom quality. The lens-confidence value (currently 0-1.12 in observed metrics) is a coarse proxy for accuracy. Compression is qualitative — atoms that pack a recognized cognitive phenomenon into a small canonical-instance + name are higher-quality patterns than verbose atoms that re-state the phenomenon at full length.

**Operational test:** the multiplicative structure captures a necessary trade-off. A tiny atom (high compression) that doesn't reliably fire on the named phenomenon (low accuracy) is not a pattern. An atom that exactly reproduces the phenomenon at full length (high accuracy, no compression) is also not a pattern. Past atom-sharpening work reordering lead-canonical-instances (lex-89rjr among others) was implicitly an accuracy × compression optimization.

### 3. Relative pattern (intensity given background knowledge K)

**Source:** Goertzel 2006 App. A p.361-362 (verbatim).

**Formulation:** `IN(p, e | K)` = pattern intensity for process p in entity e, GIVEN background knowledge K. With K available, p only needs to encode what K doesn't already encode.

**Application to lex:** This is the formal name for what the lens architecture already does. The lens evaluates new prompts RELATIVE TO the existing 114+ atom elements K. A new prompt-pattern that's already covered by an existing atom should not become a new atom; the lens should pick the existing atom. A new pattern that ISN'T covered by K is a candidate for new atom creation.

**Operational test:** before drafting any new atom, ask: relative to the existing elements K, what does this atom add that K doesn't already encode? If the answer is "nothing new" or "minor variation on existing X", DON'T add it. Add only patterns whose `IN(p, e | K)` is meaningfully > 0.

### 4. Lakatos progressive vs regressive research-program criterion

**Source:** Lakatos 1970 (canonical primary), Goertzel 2006 Ch.13 p.170-171 supplies a clean operational restatement.

**Formulation (Goertzel's restatement):** A research program is PROGRESSIVE if, when confronted with a significant amount of new data added to the Master Observation Set (MOS), it can predict this data either (a) without modification, OR (b) with modifications that are relatively simple compared to the complexity of the new data added. A research program is REGRESSIVE if it deals with qualitatively novel datasets by modifications that are equal-or-greater in complexity than the new data itself, OR by tactically decreasing its scope to avoid the problems encountered.

**Application to lex:** Lexicon-as-research-program self-assessment. Apply at every mining-pass close.

**Operational test:** at every mining-pass close, ask:
- Did this pass's new atoms/molecules ASSIMILATE new prompt-shapes simply? (progressive)
- Did this pass require retro-fitting many existing atoms to accommodate the new ones? (regressive)
- Did this pass produce molecules that are SIMPLER than the prompt-shapes they describe? (progressive)
- Did this pass produce molecules at the same complexity as the prompts they describe (no actual compression)? (regressive)
- Did this pass tactically narrow lex's scope to avoid problems? (regressive)

If three or more answers tilt regressive, elements/ is in a regressive phase and the next pass should be a sharpening/consolidation pass, not an additive-atom pass.

### 5. Diagnostic trio: conservatism + irrelevance + negligible-predictive-power

**Source:** Goertzel 2006 Ch.18 p.281 (verbatim). Goertzel synthesizes from his own analysis of Jane the paranoid schizophrenic in *Chaotic Logic*.

**Formulation:** A pathological belief-system displays three failure-modes:
- **Conservatism:** explanations are stereotyped — same explanation for every phenomenon regardless of structure.
- **Irrelevance:** the explanation doesn't track event-structure; the patterns connecting event and explanation are surprisingly small.
- **Negligible-predictive-power:** the system generates post-hoc explanations of past events but cannot make priors on new events.

The three compound: conservatism + irrelevance produces low predictive power as a NECESSARY consequence.

**Application to lex:** for each elements atom, periodically test:
- Does the atom fire the same explanation regardless of phenomenon? (conservatism failure — the atom is too coarse or too vague)
- Does the atom's canonical-instance track the actual structure of the phenomena it claims to name? (irrelevance failure — the atom is decorative, not load-bearing)
- Does the atom generate priors on new prompt-shapes, or only post-hoc rationalizations of past fires? (predictive-power failure — the atom is description-after-the-fact, not navigation-before-the-fact)

**Operational test:** an atom failing all three should be a retirement candidate. An atom failing one or two might just need sharpening. Apply at every mining-pass close + at least once per quarter as an elements-self-audit.

### 6. Combinatorial-explosion-avoidance as essence of modest-resources mind

**Source:** Goertzel 2006 Ch.2 p.19 (verbatim).

**Formulation:** "The essence of a modest-resources mind consists of a set of strategies for avoiding combinatorial explosions."

**Application to lex:** This is the elements-meta organizing claim for what lex IS. Each lex atom is (or should be) a combinatorial-explosion-avoidance heuristic — a named pattern that collapses an exponential prompt-space into a small set of recognized possibilities.

**Operational test:** for each atom, ask: what combinatorial explosion does this atom avoid? If the answer is "no specific explosion — it's just a useful concept," the atom may not belong in lex (it might belong in a different cognitive-aids artifact). Lex's distinct value-proposition is combinatorial-explosion-avoidance for prompt-recognition.

### 7. Mind-scope-defining claim

**Source:** Goertzel 2006 Ch.3 p.40 (verbatim, but with the universal-mind framing stripped).

**Formulation:** Mind = the set of patterns associated with a system that achieves complex goals in a complex environment.

**Application to lex:** Scope-discipline. Lex's domain is patterns-in-cognitive-work-by-systems-achieving-complex-goals — specifically humans (and human-AI pairs) doing software work, decision-making, learning, communication. Lex's domain is NOT patterns-in-the-universe (cosmic-mind framings), patterns-in-arbitrary-formal-systems (mathematical-pattern framings), or patterns-in-physical-self-organization (biological/chemical-pattern framings).

**Operational test:** when considering a candidate atom, ask: does this pattern operate within a system achieving complex goals in a complex environment, where the system can recognize the pattern and use it for navigation? If no (e.g., "the pattern of crystal lattices" or "the pattern of stellar nucleosynthesis"), the atom is out-of-scope for lex even if Goertzel-style patternism would include it.

### 8. Graduation criterion: stable-verifiable primary, not refs/-only

**Source:** Convention distilled from observing several graduation batches in practice.

**Formulation:** An atom flips `status: under-review → active` when at least ONE lineage entry has a primary articulation that is BOTH:
- **Verbatim staked** (the `quote:` field is populated, not `[MISSING]`)
- **Stably verifiable** by either:
  - (a) **refs/-grounded** — PDF / epub / txt present , page-cited, re-readable offline at any later time; OR
  - (b) **externally cross-attested** — ancient text fragment, RFC, canonical mathematical statement, foundational public document with multiple independent stable attestations that can be re-verified at any later time (a WebSearch confirmation counts when the underlying text is a fixed-historical-artifact, NOT when it's a recent claim about a living subject)

Secondary lineage entries may remain `[MISSING]` or be re-marked as Tier-3 deepening / Tier-4 adjacent — they do NOT block graduation when the primary anchor is solid. **Cross-checking secondary entries against refs/ or stable-external sources when possible IS the default discipline; "un-cross-checked yet" is not the same as "cannot be cross-checked."**

**Excluded as load-bearing primary:** bootstrap-originating-conversation, sibling-project attestation (score / drivermap / effigy — github.com/justinstimatze/score, /drivermap, /effigy), discovery-loop chat-snippets from other model outputs, mining-pass-self-attestation. These can appear in lineage as supplementary context but cannot bear graduation weight alone — they pin the atom to lexicon's own internal history rather than to externally-verifiable articulation.

**Why this rule and not "refs/-grounded only":** Provenance integrity is the load-bearing concern; refs/-grounding is the strongest path to it but not the only path. Ancient fragments (Archilochus), foundational engineering documents (RFCs), canonical math (Newton's laws, Cauchy-Weierstrass limits), and well-cross-attested historical claims are stably re-verifiable from multiple independent sources without requiring a specific PDF . Restricting to refs/-only would block graduation of well-grounded atoms without provenance gain. The convention is about VERIFIABILITY-PROMISE, not about a specific storage location.

**Application to lex:** This is the procedural rule for the operational `status: under-review → status: active` flip, complementing principles 1-7 (which evaluate atom QUALITY) with provenance discipline (which evaluates atom STAKING).

**Operational test (at each graduation candidate):**
- Identify the lineage entry serving as primary anchor
- Confirm: `quote:` field is verbatim (not `[MISSING]`)
- Confirm: provenance is refs/-grounded OR stable-external (per (b) above)
- Cross-check secondary lineage entries against refs/ when refs/ has the source; re-mark unverifiable secondaries as Tier-3/4 deepening with a note explaining what carries the operational claim
- If no entry meets the criterion: atom stays under-review; log the blocking source so procurement can unblock it later

### 9. Atom-vs-molecule-vs-no-mint rubric

**Source:** Convention distilled from the Pierson 2015 *Power and Path Dependence* no-mint decision, after noticing drift toward molecule-shaped atoms in several recent candidates. The rubric formalizes a discrimination that principles 1-3 implicitly governed but did not state.

**Formulation:** Three operational tests determine the correct response to an elements-candidate. Apply in order; first PASS wins.

**Test 1 — Atomic-shape test (most stringent, default-preference):**
A candidate is ATOMIC iff it satisfies BOTH:
- (a) **Single-claim shape**: the elements-novel content fits in a single descriptive-mechanism name without requiring enumeration of N sub-types in the definition. Lead canonical-instance reliably reproduces the claim. Example: lex-3fzvf sequence-position-determines-process-effect is a single claim about relative-ordering, not a typology.
- (b) **Distinctness vs K (Principle 3)**: relative to existing elements, the candidate adds elements-information that is not domain-elaboration of an existing primitive. Example: lex-3fzvf is distinct from lex-mwgep because sequencing-of-two-processes ≠ within-process-positive-feedback; the test fails for "Pierson's 5 political-power-feedback-mechanisms" because they ARE within-domain elaboration of lex-mwgep.

If PASS: mint as `_tier: atomic`.

**Test 2 — Molecule-shape test:**
A candidate is MOLECULAR iff it satisfies ALL:
- (a) **2-3 existing-atom parents**: the candidate decomposes-into 2-3 EXISTING atoms in a way that produces a recognized cognitive phenomenon that the parts alone don't produce. Examples: lex-mmq8x threat-prior decomposes-into [lex-rn9ht, lex-we98d]; lex-3ydmv fermi-estimation decomposes-into [lex-yg484, lex-axa6h].
- (b) **Compositional, not enumerative**: the molecule names a SPECIFIC COMPOSITION of named parents, not a TYPOLOGY of N items where the items are not themselves atoms. A "five mechanisms by which X happens" enumeration where the mechanisms are NOT atoms fails this test.
- (c) **Distinctness vs K**: the compositional combination is itself elements-novel (the parents alone, without the composition, don't capture the phenomenon).

If PASS: mint as `_tier: molecule` with `decomposes-into: [lex-A, lex-B, ...]`. If failed because the parents needed don't yet exist as atoms, FIRST mint the parents, THEN return to the molecule candidate.

**Test 3 — No-mint / instance-elevation:**
If both atomic and molecule tests FAIL, the candidate is likely INSTANCE-ELEVATION of an existing atom in a new domain. Document the mining-pass as a NO-MINT pass with one of:
- (a) **Lineage addition** to the existing atom: only if the new domain-instance ADDS elements-information (e.g., a new well-attested canonical instance, a new lineage tradition not yet represented). Avoid if the addition would be lineage-bloat (re-stating already-covered mechanisms in new vocabulary).
- (b) **Pure decision-document**: write the mining-pass markdown documenting that the source was surveyed and the elements-decision is no-mint, with rationale citing this rubric. This is a first-class outcome, not a failure. See Veblen Ch.III-IV (no-mint as instance-elevation of lex-5kjya) and Pierson 2015 (no-mint as instance-elevation of lex-mwgep) for canonical exemplars.

**Anti-patterns this rubric flags:**

1. **Typology-shaped atom**: name implies a SINGLE claim but definition enumerates N sub-types as constitutive parts (e.g., "five mechanisms by which X happens", "four conditions that generate Y"). This is molecule-shaped; mint as molecule with the N sub-types as decomposes-into IF they exist as atoms, else no-mint and document the typology in the source's mining-pass markdown.
2. **Composite-name-as-atom**: name has internal-conjunction-structure ("X amplifies Y into Z", "X depends on Y given Z", "X and Y are distinct properties"). May be valid as molecule IF the parts are existing atoms. If not, consider whether the COMPOUND CLAIM is itself a single primitive (some are — lex-2jaf6 resilience-and-stability-are-distinct is a single foundational distinction-claim) vs. a composition that should decompose (some are not).
3. **Domain-specific re-statement of existing primitive**: candidate is "X in domain Y" where X is already an atom. Default to no-mint + lineage-addition; only mint if the domain-specificity itself is elements-novel (e.g., coerced-political-asymmetry vs voluntary-market-exchange is a genuine domain-distinction Moe 2005 establishes that path-dependence-in-general doesn't capture).

**Why this rule:**
Principles 1-3 govern atom-quality (pattern-as-compression, accuracy × compression intensity, relative-to-K novelty). Principle 9 governs the SHAPE of the structure-decision: atomic / molecule / no-mint. Without an explicit rubric, elements/ drifted toward minting molecule-shaped or typology-shaped candidates as atoms. Principle 9 is the procedural complement to principles 1-3, the way principle 8 is the procedural complement for graduation.

**Operational test (at each candidate-atom decision):**
- Run Test 1; if PASS, mint atomic and stop.
- Else run Test 2; if PASS, mint molecule with decomposes-into and stop.
- Else run Test 3; document no-mint with rationale citing rubric and stop.
- If the candidate FEELS elements-novel but fails all three tests, that's a signal that a missing-parent-atom needs to be minted first; the candidate becomes pending-on-parent.

**Apply at:**
- Every mining-pass close, BEFORE drafting the candidate-atom YAML
- Retroactively, when a tier-misclassification is spotted
- During elements-self-audit passes

### 10. CQ-terminus audit + frame-status as standing oracle-risk discipline

**Source:** a 2026-06-04 discriminator inquiry and a full-corpus CQ-terminus sweep. Distinct axis from Principle 5: the diagnostic trio audits whether an atom is a *pattern* (accuracy × compression, predictive power); Principle 10 audits whether an atom, **if surfaced as a finding, would overstate its warrant** — its oracle-risk / crypto-tarot risk. An atom can be a perfectly good pattern (passes 5) and still be constitutive (must not be surfaced as a finding) — the two are orthogonal.

**Formulation:** Every CQ-bearing atom's critical-question chain terminates somewhere. Code the terminus — and code it **blind** (coder not told the hypothesis or any predicted direction; the blinding is what catches author-bias, per the Round-1 falsification this method survived). Map terminus → `frame-status`:
- **CHECK → `navigational`** — some CQ bottoms out in a content-specific external check (measurement, count, formal/derivational step, manipulable do-X-observe-Y intervention). Render as an operative finding the user can verify.
- **INTERPRETATION → `constitutive`** — no CQ bottoms out in an external check; the chain grounds entirely in contestable judgment. Render as an **offered lens, never as a finding.** Surfacing-a-constitutive-atom-as-a-finding is *the* crypto-tarot failure.
- **MIXED → `mixed`** (the default; ~61% of elements/) — carries a checkable toe. Render operatively but **lead with the named checkable handle** (the specific CQ that grounds externally), and mark the rest interpretive.

Two facts the elements-wide sweep established, both load-bearing:
- **CLASS ⊥ TERMINUS** — both procedures and lenses appear in every terminus bucket, so **oracle-risk cannot be read off `type-out`.** It must be traced per-atom; there is no structural shortcut.
- **Non-certification ≠ refutation.** A constitutive atom is not false; its warrant simply doesn't transfer to "finding." Remediation is honest labelling (offered-lens), never forced checkability (that would be the gap-destroyer error — manufacturing a fake handle is worse than an honest constitutive label).

**The chisel-deeper rule (operational prescription).** Depth-work — building out an atom's checkable handles, canonical instances, CQ chain — should deepen the **navigational and mixed** pools and **explicitly not the constitutive pool.** Deepening a constitutive atom adds *coherent elaboration with no added correspondence*: it makes the lens feel more authoritative without making it more checkable — crypto-tarot elaboration, the apophenia-feels-better failure operating on elements/ itself. The register is the map of where deeper is honest: when you want to deepen elements/, pull targets from the **mixed pool's handle column**, not the constitutive pool.

**Epistemic status (stated honestly, as with principles 4-5).** The CHECK/INTERPRETATION criterion rests on the coherence/correspondence argument (Tarski-flavored: you cannot certify correspondence from inside a symbolic system) — a **conjecture, not a theorem**. And `frame-status` is **unvalidatable-by-construction for the constitutive pool** (no outcome signature exists to test it against), so its justification there is **honesty-as-terminal-value, not felt-usefulness.** This is deliberate: the felt-usefulness metric is *rigged against* honest framing, because apophenia feels better than honest abstention. Outcome-testing is available only for the mixed pool's handles (see the iteration-efficiency experiment design for how that testing is kept navigational).

**Operational test:**
- At each mint: blind-code the new atom's CQ-terminus; set `frame-status` accordingly (default `mixed`, and name its handle). An atom whose chain has no external check at all is `constitutive` — that's allowed, but it must never be rendered as a finding.
- At each depth/sharpening pass: confirm the target atom is `navigational`/`mixed`, not `constitutive`. If you find yourself wanting to deepen a constitutive atom, stop — that's the failure mode this principle names.
- Periodically: re-sweep as elements/ grows. The register covers the 356 CQ-bearing atoms as of 2026-06-04; new atoms (and the ~53 empty-CQ atoms once filled) need coding before they inherit a frame-status.

**Apply at:**
- Every mint (set `frame-status` from the blind terminus-code; name the handle if mixed)
- Every depth/sharpening pass (the chisel-deeper rule: deepen mixed/navigational, never constitutive)
- Every elements-self-audit (re-sweep new atoms; spot-check that constitutive atoms aren't leaking into finding-shaped renders)

## How to use these principles

Ugly intermediate artifacts with inline findings are preferred over polished output. These principles are tools for the elements-quality discipline — not aesthetic prescriptions.

**Apply at:**
- Every mining-pass close (assessment of pass-progressiveness via principle 4 + diagnostic trio via principle 5)
- Every candidate-atom decision (atom-vs-molecule-vs-no-mint rubric via principle 9 + relative-pattern test via principle 3 + scope-defining test via principle 7 + combinatorial-explosion-avoidance test via principle 6)
- Every atom-quality-audit pass (pattern definition + intensity tests via principles 1 + 2)
- Every elements-self-reflection (the diagnostic trio, principle 5, applied to lex itself as a belief-system)
- Every under-review → active status flip (graduation criterion via principle 8)
- Every mint + every depth/sharpening pass (CQ-terminus blind-code → frame-status, and the chisel-deeper rule, via principle 10)

**Don't apply mechanically.** The principles are heuristics, not algorithms. Goertzel himself concedes (App. A p.365) that finding minimal pattern-structure is uncomputable; pragmatic approximations are the only available tool. The same applies here: the principles guide judgment, they don't replace it.

**Cite when communicating elements-design decisions.** When a mining-pass produces an atom or rejects a candidate, citing the relevant principle makes the decision-rationale legible to future-me and to user.

## Lineage / attestation summary

| principle | primary source | verified verbatim |
|-----------|----------------|-------------------|
| 1. Pattern as simpler representation | Goertzel 2006 App. A p.356 | Y ((local copy) 2026-05-04) |
| 2. Pattern intensity = accuracy × compression | Goertzel 2006 App. A p.361 | Y (same source) |
| 3. Relative pattern given K | Goertzel 2006 App. A p.361-362 | Y (same source) |
| 4. Lakatos progressive/regressive | Lakatos 1970 (canonical primary, not yet procured); Goertzel 2006 Ch.13 p.170-171 (restatement, verified) | Y for Goertzel restatement; primary Lakatos still wanted |
| 5. Diagnostic trio | Goertzel 2006 Ch.18 p.281 + *Chaotic Logic* (referenced, not yet procured) | Y for Hidden Pattern; Chaotic Logic still wanted |
| 6. Combinatorial-explosion-avoidance | Goertzel 2006 Ch.2 p.19 | Y |
| 7. Mind-scope-defining claim | Goertzel 2006 Ch.3 p.40 | Y |
| 8. Graduation criterion | distilled from observed graduation-batch convention | n/a (procedural rule, not a substantive claim with a primary source) |
| 9. Atom-vs-molecule-vs-no-mint rubric | observed tier-discipline drift 2026-05-26 + observed convention from existing molecules (lex-mmq8x, lex-3ydmv, lex-a9wpd) + no-mint exemplars | n/a (procedural rule, distilled from elements-observation) |
| 10. CQ-terminus audit + frame-status | a 2026-06-04 discriminator inquiry (356 atoms blind-coded) | n/a (elements-quality discipline; the CHECK/INTERPRETATION criterion rests on the coherence/correspondence conjecture, not a proven theorem — flagged in-principle) |

Three primary-source PDFs would deepen this artifact:
- Lakatos 1970 *Falsification and the Methodology of Scientific Research Programmes*
- Goertzel *Chaotic Logic* (year? — likely 1994)
- Chaitin 1987 *Algorithmic Information Theory*

Logged to the procurement queue (separate commit).

## Connection to elements atoms (for cross-reference)

The principle most directly mappable to an elements atom is principle 5 (diagnostic trio) — but the trio's natural use is elements-self-evaluation, not prompt-firing. The atom that DID survive the Goertzel mining as elements-content (lex-e5des dialogical-vs-monological-belief-system) operates AT the elements-meta layer too: it fires on prompts about belief-system architecture, but its elements-meta application is the lexicon-as-belief-system question (is lex dialogical or monological?).

These principles, together with lex-e5des, supply the elements-meta foundation lex hasn't formally had before now.

## Principle 11 (2026-06-05): interpreter-aware framing

**Statement.** Any consumer of lexicon's pattern-catalog — human user, LLM agent, downstream tool — is running a left-brain interpreter (lex-yh672, Gazzaniga & LeDoux 1978; canonical statement Gazzaniga 1998 *Sci Am* "The Split Brain Revisited") that constructs confident causal narratives without access to the actual cause of the behavior or judgment being explained. Lexicon supplies the interpreter with HIGH-QUALITY pattern-material to confabulate with. This entangles lexicon's success and failure modes.

**Failure mode (the worst case).** A reader uses a sophisticated named-pattern (Walton-scheme, defeasibility-attach, cognitive-bias-substrate-instance, label-molecule, etc.) as the causal explanation for their behavior or decision — when the actual driver was a different, unnamed mechanism the interpreter could not access. The pattern fits-the-narrative without fitting-the-cause. The resulting account sounds rigorous BECAUSE the pattern is rigorous, but the rigor is decorative, not load-bearing. lex-yh672 canonical-instance #6 documents this as lexicon's project-specific structural risk.

**Success mode (the best case).** A reader uses a named-pattern as a HYPOTHESIS to test against ground-truth — checking whether the pattern's premises and CQs actually hold in the case at hand, surfacing falsifying observations, treating the pattern as an instrument for inquiry rather than a verdict. lex-yh672 CQ1-CQ5 are the harness for distinguishing the two cases.

**Operational consequence (elements-meta).** 
1. Every atom's `critical-questions` field exists in part to disrupt interpreter-fluency — the CQs force the reader to look for ground-truth distinguishing evidence rather than accepting the pattern as already-explanatory. This is now a first-class design constraint: CQs should be HARD for the interpreter to pass without genuine engagement with the case.
2. The `evokes` and `canonical-instances` fields should anchor in CONCRETE cases that can be checked, not in abstract restatements that the interpreter can paraphrase as agreement.
3. `lineage` provenance is part of the interpreter-discipline: a pattern with named-author + named-source + named-page is harder to deploy as decorative-rigor than a pattern asserted as common-knowledge.
4. When pattern-deployment is suspiciously fluent (CQ feels obvious, conclusion feels predetermined), check lex-yh672's CQs explicitly.

**Why this matters for the project's stakes.** Lexicon's project-stakes are "personal-use primary, eventual non-embarrassing OSS" (per memory). The personal-use case is reasonably safe — a single user who knows the tool can deploy patterns with appropriate skepticism. The OSS case raises the failure-mode risk: a downstream user encountering lexicon for the first time will run their interpreter on the patterns by default. The principle is: design for the OSS case from the personal-use case forward, by making interpreter-discipline a first-class affordance, not an external responsibility.

**Elements atom that grounds this principle.** lex-yh672 left-brain-interpreter-constructs-confident-causal-narrative-without-access-to-actual-cause-of-behavior. Active on Gazzaniga 1998 *Sci Am* + substrate-attestation. The atom itself sits at the META-mechanism layer; this principle is the elements-design consequence of taking the mechanism seriously.
