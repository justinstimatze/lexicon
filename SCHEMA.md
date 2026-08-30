# Lexicon entry schema

## Format

YAML frontmatter; minimum fields. Optimized for LLM operations, not
human readability. Raw entries aren't meant for direct reading; the
surfacing function is the user-facing layer.

```yaml
---
id: lex-2b3c4 # stable unique ID, never changes — 5-char opaque code, non-sequential
name: catuskoti-2 # canonical short name (LLM-friendly)
type-in: frame # input concept type (what the primitive operates on)
type-out: posture # output concept type (what it produces)
formal-if-any: ¬assert(p) # formal spec where one exists; else omit
related: [lex-6d7e8, lex-9fg2h] # type-compatible neighbors (composition candidates)
evokes: [refusal, suspended-judgment] # optional; gestural near-synonyms not chosen as canonical name
_tier: atomic # internal tier marker; open vocabulary (see below)
lineage: # one or more provenance entries
  - source: primary           # quality enum: primary | practitioner | discovery-loop | cross-attestation | secondary
    tradition: madhyamaka     # optional canon/school/work-cluster grouping (open vocab); only set when ≥2 atoms share it
    text: mmk                 # work-slug (e.g. nagarjuna-mmk, walton-reed-macagno-2008)
    citation: 13.8
    quote: "..." # optional; verbatim source text. Until populated, lineage is provisional.
  - source: primary
    tradition: madhyamaka
    text: vigrahavyavartani
    citation: 29
canonical-instances: # examples; either inline or corpus-pointers
  - excerpt-id-1
  - excerpt-id-7
severity-tier: info # info / warning / critical (downstream surfacing weight)
status: active # active / under-review / deprecated
---
```

### Molecule-tier additional fields

Entries with `_tier: molecule` (or higher) carry three additional
fields that capture the assembly-structure that named schemes need to
remain operationally useful — collapsing schemes into a single in→out
signature loses what makes the scheme worth naming.

```yaml
---
# (atomic fields above as normal)
_tier: molecule
decomposes-into: # required on molecules
  - lex-j4k5m # atomic constituent (id when present)
  - "[MISSING: case-similarity-mapping]" # bracketed flag when constituent isn't yet mined
premises: # numbered premise structure
  - source E is an expert in domain D containing proposition A
  - E asserts A
  - therefore, presumptively, A is true
critical-questions: # Walton-style defeaters; if unanswered, retract conclusion
  - is E really an expert in D?
  - is A in D?
  - "..."
---
```

(The body is optional reference prose — not part of the operating
contract; not surfaced to readers; useful for the maintainer drilling
in.)

## Field semantics

- **id**: `lex-NNNNN`, a 5-character opaque code (alphabet
  `23456789abcdefghjkmnpqrstuvwxyz`, excludes `0/o/1/l/i` for hand-typing
  safety). Non-sequential by design — carries no ingestion order or other
  meaning. Stable across renames; never reused, never changed. Pre-2026-08-20
  ids were sequential 4-digit numbers; `docs/id-migration-map.csv` holds the
  permanent old→new mapping for anything still citing an old id.
- **name**: short canonical label. May change if dedup with another entry
  forces it; id stays stable.
- **common-name** *(optional)*: jargon-free, lay-readable label
  (~2-5 words) for human-facing consumers (e.g., reveal-card
  titles, end-user UIs). Descriptive-of-the-pattern, NOT prescriptive —
  must not telegraph the "right answer" when used to hint at a pattern
  the reader is meant to surface. Consumers fall back to humanizing
  `name` when absent.
- **type-in / type-out**: both required. Define the input/output concept
  type. Used for type-compatible composition. The vocabulary of types
  is bounded — see [Types vocabulary](#types-vocabulary) below.
- **formal-if-any**: optional. Formal spec (logic, math, type theory) if
  the primitive has one. Many won't; that's fine.
- **related**: list of `lex-NNNNN` ids that are type-compatible neighbors
  (suitable for composition). Bidirectional — symmetric clustering /
  co-firing semantics. Distinct from `scaffolds-from:` below, which is
  directed and pedagogical.
- **scaffolds-from** *(optional)*: list of `lex-NNNNN` ids that prime your
  grasp of this atom — soft pedagogical scaffolding, not strict
  prerequisite. Reading: "this atom scaffolds from <these earlier
  atoms>" — exposure to them helps you build understanding faster,
  but isn't required for the atom to land. Directed; cycles ARE
  allowed (mutually-priming pairs are meaningful and lint surfaces
  them as info, not warning). Soft semantic is deliberate: stronger
  than `related:` (directed) but softer than a strict prerequisite
  (which would imply blocking).

  Lint rules:
  - error: self-reference
  - warning: dangling ref (target not in elements/)
  - info: same-tradition same-tier (probably belongs in `related:`)
  - info: cycle (mutually-priming pair — surfacing as data, not flaw)

  Reverse direction is NEVER stored. Use `lexicon scaffolded-by <id>`
  for derived inverse-traversal.
- **encounter-tier-override** *(optional, integer 1-5)*: escape hatch
  for the rare Hofstadter-translation case where the *derived* tier
  (computed by `lexicon tier-derive` from lineage tradition + source +
  in-degree) is wrong. Absent for the common case where the derived
  value is right. Lint emits info when override diverges by more than
  1 from the derived value. There is NO stored `encounter-tier`
  field — tier is a derived view, not a stored property. See
  `lexicon tier-derive` and the encounter-tier table below.
- **lineage**: required. At least one source entry. `source:` is the
  quality enum documented above (`primary | practitioner |
  discovery-loop | cross-attestation | secondary`). `tradition:`
  (optional) names the canon/school/work-cluster grouping — current
  values in use include `madhyamaka`, `nsm`, `image-schema`, `walton`,
  `cbt`, `tversky-kahneman`.
- **canonical-instances**: at least one example required. Either inline
  excerpt or a `excerpt-id-NNNN` pointer to a corpus.
- **severity-tier**: `info` / `warning` / `critical`. Determines the
  weight downstream consumers give the tale when surfacing.
- **status**: lifecycle marker. `active` is normal; `under-review` for
  candidates; `deprecated` for retired entries (kept for provenance).
- **evokes** *(optional)*: list of gestural / near-synonymous handles
  that aren't the canonical `name:` (collision, precision, or other
  reasons blocked them) but are useful as intuition-pumps for "what
  shape is this?". Used by LLM for fuzzy retrieval and by humans for
  shape-grasping at a glance. Preserves conceptual handles through
  renames (when `name:` changes, the displaced handle moves to
  `evokes:`). Grounded in `upāya` (lex-nahg9) — names as useful
  imprecise vehicles, not accurate-portrayals of the referent.
- **_tier** *(internal, open vocabulary)*: tier marker for the
  chemistry-book hierarchy (atoms / molecules / compounds / ...). Not
  user-facing; used internally by the surfacing function to pick a
  resolution appropriate to the context being rendered into. Values
  in current use — descriptive, not enum:
    - `sub-atomic` — semantic substrate (NSM primes); paraphrase test,
      not user-facing primitives
    - `atomic` — cognitive moves at lexicon's primary tier (image
      schemas, Madhyamaka primaries, defeasible-presumption)
    - `molecule` — named assemblies of atoms with established
      practitioner use (Walton schemes, mental models when mined)
    - `compound`, higher — kept open for future passes; no point
      naming speculatively (3-occurrence rule applies)
- **agent-instruction** *(optional)*: single imperative
  one-liner — the operational "when you see this pattern, do this" decision
  rule. Distinct from **critical-questions** in shape and use:

  | Field | Shape | Used to |
  |---|---|---|
  | `critical-questions` | questions, plural, diagnostic | confirm or reject that the pattern fires |
  | `agent-instruction` | imperative, single, prescriptive | act, once the pattern is confirmed firing |

  Concrete contrast for lex-znaau *premature-closure*:
  - CQ: *"Have you tested your hypothesis against an obvious alternative?"*
  - agent-instruction: *"Surface the alternative explicitly and name what
    observation would distinguish them before committing."*

  The two overlap by design (a good CQ implies the action), but the discipline
  that keeps them distinct: CQs end with `?`, agent-instruction is imperative.
  Coverage is incremental — most atoms will gain it via dedicated authoring
  passes; the `db lint no-agent-instruction` gate tracks remaining coverage.
- **lineage.quote** *(optional)*: verbatim source text from the cited
  work. Until populated, the entry's lineage is provisional. Provisional
  entries with only training-data-recall lineage stay `status:
  under-review` until quotes are populated.

  **Unstaked sentinels.** A quote field may instead carry a bracketed
  marker saying no verbatim span is staked. The recognised prefixes are
  `[MISSING …]`, `[NOT VERIFIED …]` (including `[NOT VERIFIED
  REFS-GROUNDED …]`), `[paraphrase, not verbatim …]`, and
  `[MEMORY-LEVEL …]`. Use one of these — do not invent a fifth. They are
  matched as a *prefix* of the quote's leading bracketed segment by
  `types.LineageEntry.QuoteStaked()`, which is the single definition of
  "is this citation staked" and the thing every badge, gate and graduation
  check must ask rather than re-deriving from a string test.

  Two rules follow from how the matching works. A sentinel only counts at
  the *start* of the field, so a staked quote may contain the word
  "missing" in its running prose without being reclassified. And an
  editorial prefix on a real quote (`[p.156:]`, `[Scott 1998
  Introduction:]`) must not begin with a sentinel word.

  This list exists because renderers once tested for `MISSING` alone,
  so entries staked with a different sentinel prefix displayed as
  VERIFIED while explicitly marked unverified — the signal inverted on
  precisely the axis the quote-fidelity audit protects. Adding a new
  convention without adding it here recreates that bug.

### Molecule-tier fields (required on `_tier: molecule` and higher)

- **decomposes-into**: required. List of constituents — `lex-NNNNN` ids
  for atoms already in the library, `[MISSING: name]` flags for atoms
  the decomposition surfaces but the library doesn't yet contain.
  `[MISSING:]` flags are forcing functions for the next mining pass.
- **premises**: numbered list of the premise structure the molecule
  operates on. Most of what makes a named scheme reusable lives here.
- **critical-questions**: defeaters that, if unanswered, retract the
  conclusion. Walton-style; carrying it through to lexicon retains
  operational value for downstream practitioners.
- **assembly** *(optional)*: single string in the
  composition-operations vocabulary (see `docs/principles/composition-operations.md`)
  describing how the atoms in `decomposes-into:` combine. Grammar:
  `op(arg1, arg2, ...)` with nesting; ops are sequential, parallel,
  defeasibility-attach, choice, iteration, conditional, scoping;
  named-arg form for keyword args (defeaters, selector, until, if,
  within). Example: `"sequential(lex-q9asc, lex-dm5te) →
  defeasibility-attach(lex-af9ax, defeaters=lex-th68b)"`. Optional
  for v0 — molecules without assembly still valid; the field adds
  bond information that `decomposes-into:` flat-list lacks. Not yet
  formally parsed; introspection mode surfaces it verbatim.

## Types vocabulary

Bounded set of input/output concept types primitives operate on. This
is lint-enforced (`render/cmd/lexicon/cmd_lint_taxonomy.go` against
`PivotRowOrder`/`PivotColOrder` in `render/internal/viz/pivot.go`) —
that code is the source of truth; this list is kept in sync with it,
not the other way around. `type-in` and `type-out` draw from
overlapping but not identical sets:

Valid for both `type-in` and `type-out`:

- `state` — a snapshot of cognitive content at a moment
- `process` — an unfolding cognitive activity
- `posture` — a stance toward a claim (assert / negate / both / neither / suspend)
- `frame` — a structural template for organizing content
- `claim` — a proposition with truth-evaluable content
- `question` — an open-ended interrogative
- `composition` — a sequence of primitive applications
- `structure` — a structural/relational arrangement

`type-in` only:

- `situation` — a concrete circumstance the primitive operates on

`type-out` only:

- `typology` — a classification scheme as output
- `warning` — a flagged defeater/caveat as output

`view`, `scope`, and `perspective` were an earlier proposed vocabulary
and are NOT valid values — `lexicon lint` rejects them. Composition is
the operation that combines two primitives into a new effective
primitive.

## Discovery-loop integration

Primitives discovered via the dense-style compress+verify loop are
stored alongside hand-curated entries with `lineage.source: discovery-loop`
and `status: under-review` until the maintainer promotes them to
`active`. Composition-recovery rate is the metric for promotion.

## Render function contract

```
render(matched_primitives: List[Lex], context: Substrate, working_vocab: List[str]) → str
```

Takes a list of primitive IDs that fired against current content, the
substrate they fired in (downstream pattern-detector graph, claim graph,
or raw transcript), and the reader's recent working vocabulary. Returns
a single prose paragraph in the reader's register that reflects the
primitive matches without naming them.

The render function is the only user-facing component. It does not
expose entry IDs, type signatures, or formalisms unless the reader
explicitly drills in.

## Encounter-tier (derived, 1-5)

The encounter-tier scores "how much prerequisite cognitive scaffolding
does this atom need before it lands as true for a literate adult
reading it cold, no domain expertise." It is **derived**, not stored.
Computed by `lexicon tier-derive` from `lineage[0].tradition`,
`lineage[0].text` (source name), and in-degree.

Baseline reader (the schema fixes this once so the field is
comparable across atoms): a literate adult who's read trade
non-fiction, no specific domain expertise in any given atom's field.
Cross-cultural inversion (non-duality lands tier-1 for a Mahayana
reader, tier-5 for an analytic philosopher) is acknowledged and is NOT
fixed by the schema — atoms with strong cultural-context dependence
should surface a note in `agent-instruction`, not fork the tier.

| tier | name              | description                                                                |
|------|-------------------|----------------------------------------------------------------------------|
| 1    | proverbial        | lands without framing; could say to a stranger ("don't piss in the wind")  |
| 2    | plain             | lands after one sentence of framing (sunk cost; confirmation bias)         |
| 3    | structural        | needs a paragraph or domain knowledge (Goodhart's Law; path dependence)    |
| 4    | counter-intuitive | requires surrendering a strong prior (bitter lesson; treacherous turn)     |
| 5    | esoteric          | needs sustained training to land (non-duality; upāya; showing/saying)      |

`lexicon tier-derive` heuristics (lineage tradition / source signal /
in-degree pull); when an atom's derived tier is genuinely wrong (the
Hofstadter-translation case — tier-5 content compressed to tier-2
prose), set `encounter-tier-override: N` to the correct integer. Lint
emits info when override diverges from derived by more than 1.

**What the tier system does:**
- Elements-coverage diagnostic — `lexicon tier-derive -skew`
  surfaces tier-distribution skew, which drives mining-queue priority.
- Reader-level filtering — "show me the atoms anyone
  could understand" vs "show me the deep cuts."

**What the tier system does NOT do:**
- It does not by itself implement folk-cousin discovery (pairing a
  tier-4 atom with its tier-1 vernacular sibling). That capability
  needs either an explicit pointer field or a tradition × tier
  cross-product query — both deferred.
- It does not optimize for desirable-difficulty learning. A tier
  optimized for "lands cold" selects against the productive friction
  that durable learning depends on. Elements/ is a reference, not
  a curriculum.

Design shape: derived view (not stored), single optional override
field, shipped alongside `scaffolds-from[]` and `lexicon scaffolded-by`.

