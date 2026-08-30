# Lexicon — domain boundary statement

*Winze-convention back-port (mirror-source-commitments family). Closes
the deferred-queue item from `winze-conventions-applied.md`. The
absence of an explicit boundary was making mining-pass decisions
ad-hoc; this artifact is the operational test for in-/out-of-domain.*

## The statement

> **Lexicon's domain is the typed catalog of cognitive operations —
> atomic moves a thinker performs on views, claims, frames, and
> processes, plus their named assemblies (molecules) and higher
> compositions when they emerge — that a context-aware surfacing
> function selects from to render context-appropriate prompts in the
> user's working vocabulary.**

Compare to BOOTSTRAP.md's existing intro: *"A typed cross-domain
catalog of low-level cognitive primitives, plus a context-aware
surfacing function that renders matched primitives into the user's
working vocabulary."* This statement extends BOOTSTRAP's intro by
making the chemistry-book frame (atoms + molecules + higher) explicit
in the boundary itself — the chemistry-book reframe was decided
post-BOOTSTRAP.

## Positive characterization (what counts as in-domain)

A candidate entry is in-domain if it satisfies **all three** tests:

1. **Cognitive-operation shape.** The candidate names something a
   thinker *does* (an operation/move) or a *typed input/output*
   shape such operations consume/produce (view, posture, claim, frame,
   process, etc.). Not: a fact, a state of the world, a person, a
   theory's content.

2. **Composability.** The candidate either is itself a composition-
   ready atom (in→out signature), or it's a named assembly of
   composition-ready atoms (a molecule, with `decomposes-into:`
   that's expressible in current atoms within fuzzy tolerance per
   chemistry-book rule).

3. **Surfaceable utility.** A reasonable rendering of the candidate
   in user-facing prompt-language is conceivable — i.e., the
   surfacing function could plausibly use it without the user
   needing to learn the entry's name. Pure-internal scaffolding
   (e.g., NSM primes as paraphrase-substrate) sits at `_tier:
   sub-atomic` and serves the test indirectly.

## Negative characterization (what's out-of-domain, and where it lives)

The following are out-of-domain, with pointers to where they belong
in Justin's sibling-project ecosystem:

- **Reasoning-topology detectors over runtime conversation traces.**
  E.g., load-bearing-vibes, fluency-trap-firings, never-challenged-
  claim flags. → **slimemold's territory** (github.com/justinstimatze/
  slimemold). Lexicon may name the *primitive cognitive moves* the
  detector watches for (e.g., the underlying coarse-graining when a
  claim is over-summarized), but the *detector itself* lives in slimemold.

- **Failure-pattern catalogs / bright-pattern recognition.** E.g.,
  cosmology-grandiosity-cluster, AlphaProof-analogizing, theater-of-
  rapport. → **ismyaialive's territory.** These are named clusters
  of failure-modes-in-deployed-AI. Lexicon may name the primitives
  the failure-modes deploy (e.g., spotlight-choice as the move behind
  one form of grandiosity), but the cluster-as-failure-mode lives in
  ismyaialive.

- **Cognitive-bias catalogs as such.** E.g., availability heuristic,
  representativeness, anchoring, framing effect. BOOTSTRAP rejected-
  paths explicitly excludes "Tversky-Kahneman bias catalog" as
  lexicon's center. Where a bias decomposes into atomic cognitive
  moves the catalog doesn't isolate, those *moves* may be in-domain.
  The bias-as-named-pattern lives in slimemold's detector machinery
  or in ismyaialive's pattern catalogs depending on use.

- **Bias auditors / topology probes.** E.g., winze's 9 bias-auditors;
  topology probes (single-source, uncontested, thin-provenance,
  bridge-entity, concentration-risk). → **winze's territory.**
  Lexicon may export atoms the auditors *use*; the auditor-as-
  operationalized-check belongs in winze.

- **Knowledge graphs of typed claims and relationships between them.**
  → **winze's territory** (Entity / Predicate / Provenance / Quote
  schema; StructurallyAnalogousTo and friends). Lexicon's molecules
  are *cognitive-operation entries*, not *claims-about-the-world
  entries*.

- **Domain-specific content catalogs.** E.g., Buddhist-philosophy
  catalog as such, Walton-canon as such, image-schemas-of-Lakoff as
  such. Lexicon mines *from* these as sources but doesn't contain
  them as catalogs. The domain test is operational, not
  source-loyalty.

- **Theory-content / school-of-thought summaries.** E.g., "what
  Madhyamaka says about emptiness", "Walton's argumentation
  framework as a school". Out-of-domain. The atoms/molecules a
  school operationalizes can be in-domain; the school-as-
  intellectual-position is not.

- **Render-function output / surfacing-function infrastructure.**
  Out-of-the-library; that's the surrounding system, not entries
  in the catalog.

## Borderline cases (decisions, with reasoning)

| candidate | call | reasoning |
|---|---|---|
| **NSM primes** (Wierzbicka semantic primitives) | **in-domain at `_tier: sub-atomic`** | They're not user-facing operations, but they are typed building-blocks lexicon uses for its paraphrase-test discipline. Bootstrap-internal infrastructure that the catalog cannot do without. |
| **Image schemas** (CONTAINER, PATH, FORCE, etc.) | **in-domain at `_tier: atomic`** | Wikipedia (verified): "recurring structure within our cognitive processes which establishes patterns of understanding and reasoning... embodied prelinguistic structure of experience." That's exactly atom-shape. |
| **Walton schemes** (expert-opinion, analogy, sign...) | **in-domain at `_tier: molecule`** | Named assemblies with explicit premise structure + critical questions. Practitioners use them by-name. Decomposable into atoms within fuzzy tolerance (chemistry-book rule). |
| **CBT cognitive distortions** (catastrophizing, all-or-nothing, mind-reading) | **in-domain at `_tier: molecule` (when mined)** | Same shape as Walton schemes — named assemblies practitioners use by-name. Each distortion decomposes into a cluster of atomic moves (e.g., catastrophizing = project-consequence-chain + extreme-evaluate-state-as-undesirable + collapse-probability-to-certainty). |
| **Mental models** (Farnam Street style: inversion, second-order-effects, etc.) | **in-domain at `_tier: molecule` (when mined)** | Same shape. Inversion = explicit atomic move (negate-and-trace-implications); second-order-effects = causal-projection chain + downstream-evaluate. |
| **Cognitive-bias names** (availability, anchoring, representativeness) | **OUT-of-domain as catalog; the underlying primitives may be in-domain** | BOOTSTRAP excludes the Tversky-Kahneman catalog. The bias-as-name belongs in slimemold/ismyaialive detectors. The atomic moves the bias deploys (e.g., availability = ease-of-recall-as-frequency-proxy) can be lexicon atoms when surfaced through mining. |
| **Madhyamaka primaries** (catuṣkoṭi, two-truths, upāya, lta-ba-med-pa) | **in-domain at `_tier: atomic`** | They name typed cognitive postures/moves. catuṣkoṭi-2 has type-out: posture; upāya operates on the using-of-vehicles. Operationally available. The Madhyamaka *school* is out-of-domain; the operational primitives it surfaced are in-domain. |
| **Dialectic / Socratic method** (as a named scheme) | **in-domain at `_tier: compound` when surfaced** | Higher-tier than Walton schemes — would be a cluster of molecules (catuṣkoṭi-positioning + premature-closure-detection + multiple Walton schemes interleaved). Not yet in library; expected to surface eventually. |
| **Personality types / Big Five / MBTI / Enneagram** | **OUT-of-domain** | Person-typing, not cognitive-operation-typing. Lexicon entries don't predicate over thinkers; they're operations any thinker could perform. |
| **Therapy frameworks as such** (CBT-as-framework, ACT-as-framework, IFS-as-framework) | **OUT-of-domain as frameworks; their operational primitives are in-domain** | Same shape as Madhyamaka call. The framework-as-school is out; its operational moves are in. |
| **Decision-theory / utility theory frameworks** | **OUT-of-domain as theory; the operational moves they isolate are in-domain** | Specifically: the goal-directed-reasoning cluster (currently queued for mining) will pull atoms from this literature without bringing the theory's normative content as catalog. |

## Test against the catalog as it stood at this pass (sanity check)

*The catalog held 56 entries when this test was run. It has grown by more
than an order of magnitude since; the finding below is a record of that
pass, not a claim about the current catalog.*

After the boundary statement, every current lex-NNNN entry should pass
the in-domain test. Spot-checks:

- **lex-0001 catuskoti-2** (Madhyamaka, atomic): named cognitive
  posture, type-in: view, type-out: posture, surfaceable. ✓ in.
- **lex-4yhqs coarse-graining** (statistical mechanics, atomic): named
  cognitive operation, surfaceable. ✓ in. (The fact that it's
  borrowed from physics doesn't matter — the operation is the move.)
- **lex-0021 nsm-because** (NSM, sub-atomic): elements primitive used
  for paraphrase-test discipline. ✓ in (sub-atomic).
- **lex-kebfa argument-from-expert-opinion** (Walton, molecule): named
  assembly, premises + critical-questions, fully decomposable into
  atoms after defeasible-inference cluster mining. ✓ in.
- **lex-th68b critical-question-checklist** (Walton, atomic): meta-
  shape that operates on molecules; surfaceable as "what would
  retract this conclusion?" ✓ in.

# FINDING (as of this pass, 56 entries): all of them pass the test. No reclassification or
# deprecation needed. The boundary statement is consistent with the
# existing accreted library — it doesn't force a retroactive cleanup.
# This is the right shape for a boundary written *after* the
# library has accreted: it should fit the existing entries naturally
# AND give a clear test for future candidates. If it failed either,
# it'd be wrong.

## What the boundary excludes that *might* feel borderline

Two cases that read as "borderline" but actually aren't, worth saying
explicitly:

- **The chess-centaur framing itself** (BOOTSTRAP's load-bearing
  framing). Out. It's a project-shape framing, not a cognitive
  operation. Belongs in BOOTSTRAP / project-level docs.
- **The chemistry-book frame** (the analogy structuring lexicon's
  tier model). Out. Same shape — project-shape framing, not entry.
  Belongs in pass-1-corrections.md / SCHEMA.md.

These framings are what *organizes* the catalog, not entries *in* the
catalog. Per the chess-centaur metaphor: Stockfish's evaluation
function is what produces the moves, but the evaluation function
itself isn't a chess move.

## Schema implication

No new schema field forced this pass. The boundary statement operates
*above* the schema — it's a filter on what gets a `lex-NNNN` id at
all, not a field on existing entries.

If a future candidate fails the in-domain test, it doesn't become
`status: rejected` (no such status exists, and adding it would be
schema-creep). Instead, it doesn't get an entry; reasoning for the
non-promotion can be logged in the relevant mining-pass file (or a
future `out-of-domain-log.md` if the count of these crosses the
3-occurrence threshold).

## Self-reference watch — non-instance

The domain-boundary statement is a project-shape framing (per the
two cases above), not a use of a lexicon primitive describing
lexicon's machinery. Not an instance of the-fold. Logged here so
future passes don't mistakenly count it.

## What this pass closes / leaves open

**Closes (queue):**
- Domain-boundary statement (was on deferred list since
  winze-conventions-applied.md was written)

**Leaves open:**
- Update BOOTSTRAP.md intro to reference this artifact (mechanical;
  do in this pass)
- Out-of-domain log (don't create until 3rd rejection; currently 0/3)

**State after pass:**
- 56 entries at the time (no change — this pass is structural, not content-additive)
- All entries pass the boundary test
- Future mining passes have an operational filter
