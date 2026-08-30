# decomposes-into vs assembly: an elements-meta principle

*Lifted from assembly-typecheck-pass-1 closure. Recorded here because the
principle has been applied 4 times across 4 different mining-pass
genealogies and earned cross-domain support — past the threshold where
it should be discoverable independently of the lineage entries that first
articulated it.*

## The principle

> A molecule's `decomposes-into` field names every atom that participates
> in the cognitive content of the molecule. The `assembly:` field types
> only the **always-present constitutive transformation**. Co-occurring,
> intermittent, or motivating-stance constituents are recorded in
> `decomposes-into` (and named in `premises`) but live **outside** the
> typed assembly.

Equivalently: `decomposes-into ⊇ atoms(assembly)`, and the difference is
not noise — it is the set of constituents whose role is descriptive,
contextual, or stance-like rather than load-bearing in the typed flow.

## When this principle is the right tool

When you have a molecule that lists N atoms in `decomposes-into`, you'd
like to compose all N in the typed assembly, but doing so produces a
type-mismatch error AND one of these is true about at least one atom:

- It's a **motivating stance** that explains *why* you reach for the
  molecule, not a step *within* it (e.g. composition-over-inheritance for
  Decorator: lex-9nj6a in lex-z8m97).
- It's an **intermittent participant** that appears in some instances of
  the molecule but not every iteration (e.g. mentor-as-scaffolded-
  transmission in the trial-arc: lex-hng49 in lex-h7vet, where premises
  explicitly said "*possibly* receiving mentor-transmission").
- It's an **audience-side projection** or interpretive frame that
  describes how the molecule is perceived, not what it computes (e.g.
  archetype-as-projective-shorthand: lex-uz7g4 in lex-h7vet, where the
  archetypes are how the audience pattern-matches the figures, not how
  the protagonist transforms).
- It's a **paired prerequisite or sibling move** whose typed inclusion
  would force the assembly into a shape that distorts the always-present
  transformation (e.g. ad-hominem's claim-context: lex-0042 in lex-2rsad,
  where the speaker's claim is the context the ad-hominem attacks, not a
  step inside the attack).

In each case, the atom belongs in `decomposes-into` (it's part of the
cognitive content) but not in `assembly:` (it's not part of the typed
flow).

## What this principle is NOT

- It is **not** permission to omit constitutive steps from the assembly
  because they fail to typecheck. If an atom is genuinely part of the
  always-present transformation and its typing breaks the flow, the
  honest fix is to retype the atom (with a lineage entry recording why),
  not to demote it to "intermittent."
- It is **not** a license to leave `decomposes-into` and `assembly:`
  unrelated. The set of atoms in `assembly:` must be a subset of
  `decomposes-into`; the linter enforces this. The principle clarifies
  what the difference between the two sets means.
- It is **not** the same as "this molecule shouldn't be a molecule." If
  *most* of `decomposes-into` is outside `assembly:`, that's a signal
  the molecule might be a family of co-occurring atoms rather than a
  typed pipeline (cf. Polya's caveat on lex-bpr6b). Apply the principle
  judiciously; don't use it to rescue molecules that aren't really
  molecules.

## Companion principle: host type-out matches assembly output

Adjacent but distinct: a molecule's declared `type-out` should equal the
type produced by the outermost operator of its `assembly:`. The linter
does not yet enforce this consistency, and a one-shot audit across all
assembled molecules (assembly-host-type-audit-pass-1.md) found host
`type-out` fields drifting from assembly outputs **in both directions**:

- **Aspirational over-declaration** — host declares a richer type than
  the assembly produces. Pass-1 found this in lex-z8m97 (host `composition`,
  assembly delivered `frame`) and lex-h7vet (host `composition`, assembly
  delivered `state`). Both were retyped down to match the assembly.
- **Under-declaration** — host declares a type that the assembly's
  outermost operator overshoots. Audit found this in lex-3ydmv (host
  `claim`, assembly produces `posture` via terminal lex-ds73b) and
  lex-r3w25 (host `state`, assembly produces `claim` via terminal
  lex-hbgcb). Under-declaration is the *inverse* failure mode and
  requires a different fix: either retype the host up, or restructure
  the assembly so the terminal operator produces the declared type
  (e.g., move a sanity-check atom from terminal-sequential to
  defeasibility-attach so it doesn't bump the output type).

The two failure modes suggest a common underlying cause: host `type-in`
and `type-out` fields appear to have been filled in independently of the
assembly's actual flow, rather than mechanically derived from the
outermost operator. The cleanest long-term fix is a linter check that
computes the assembly's output type and warns when it diverges from the
declared host `type-out`. See substrate-grammar-extensions-scoping.md
for the proposal.

Until that check exists, when adding or editing a molecule's `assembly:`
field: compute the outermost operator's output type by hand and verify
it equals the host `type-out`. If not, decide whether to retype the
host or restructure the assembly — the right answer depends on what the
molecule's cognitive content actually delivers.

## Operational test

Before adding or fixing a molecule's `assembly:` field, for each atom in
`decomposes-into`:

1. **Is this atom present in every fire of the molecule?** If no → it
   doesn't belong in `assembly:` (intermittent participant).
2. **Is this atom what the molecule transforms, or how the molecule
   transforms?** If "what" (the context, the input-shape, the
   prerequisite stance) → it doesn't belong in `assembly:` (motivating
   stance / context).
3. **Is this atom audience-side or analyst-side?** If audience-side (the
   projection, the perceived archetype, the interpretive frame) → it
   doesn't belong in `assembly:` (audience-side projection).
4. **If you put it in `assembly:`, does the typed flow still describe
   the cognitive content?** If no → it doesn't belong in `assembly:`
   (shape-distorting inclusion).

If an atom fails all four tests, it belongs in `assembly:` as part of
the typed flow, and any type-mismatch is a real bug to fix at the atom
level (retyping with a lineage entry) or molecule level (restructuring
the assembly shape).

## Attestation (4 applications, 4 domains)

The principle was applied in assembly-typecheck-pass-1. Cross-domain
attestation surfaced naturally because the typecheck pass touched
molecules from independent mining-pass genealogies:

| Molecule | Domain | Atom held out of assembly | Reason |
|---|---|---|---|
| lex-2rsad identity-defensive cognitive shutdown | cognitive defense (Festinger / Kahan / Sloman & Fernbach) | lex-0042 ad-hominem | claim-context that ad-hominem attacks, not a step inside the defense |
| lex-z8m97 gof-decorator | software design patterns (Gamma et al. 1995) | lex-9nj6a composition-over-inheritance | motivating stance / why-you-reach-for-Decorator |
| lex-bpr6b polya-work-backwards-via-related-problem | problem-solving heuristics (Polya 1945, Newell GPS) | (sequential subcase: lex-89rjr anchoring is inside one choice arm only, not before the whole choice) | restructure-then-localize, not pure hold-out, but same principle motivates the move |
| lex-h7vet depart-trial-return-arc | narrative structure (Campbell 1949, Propp 1928, Schon 1983) | lex-hng49 mentor-as-scaffolded-transmission, lex-uz7g4 archetype-as-projective-shorthand | intermittent participant (Propp's function-14 donor; Schon's scaffolder) + audience-side projection |

The principle survived a four-panel cross-domain test:

- **Cognitive defense:** Festinger 1957 (cognitive dissonance); Kahan
  2010s (motivated reasoning); Sloman & Fernbach 2017 (illusion of
  explanatory depth)
- **Software design patterns:** Gamma / Helm / Johnson / Vlissides 1995
  (Design Patterns); Alexander 1977 (A Pattern Language)
- **Problem-solving heuristics:** Polya 1945 (How to Solve It); Newell
  1959+ (Logic Theorist / GPS); Wadler (sum-type vocabulary);
  McBride (dependent-type vocabulary)
- **Narrative structure:** Campbell 1949 (Hero with a Thousand Faces);
  Propp 1928 (Morphology of the Folktale); Schon 1983 (Reflective
  Practitioner); Dijkstra (loop-invariant discipline)

Within each domain, the relevant practitioners gave converging reasons
for holding the atom out of the typed flow: stance vs step (patterns),
optional vs constitutive (narrative), context vs operation (defense),
sub-arm-localization vs pipeline-prefix (heuristics).

## Boundary case: when to demote rather than apply this principle

Polya's caveat on lex-bpr6b deserves recording: the principle can rescue
a molecule, but if applying it requires holding *most* of
`decomposes-into` out of `assembly:`, the molecule may not be a typed
pipeline at all — it may be a family of co-occurring atoms with a
shared usage context.

Heuristic:
- If `|decomposes-into| - |atoms(assembly)|` is **0-1**, the molecule is
  a clean typed flow.
- If it's **2-3** with explicit role-labels (motivating stance, donor,
  audience-projection), the principle applies cleanly.
- If it's **most of `decomposes-into`**, the molecule should likely be
  demoted to "family" (a related-cluster of atoms) rather than rescued
  as a typed pipeline.

The threshold is judgment-bound, not algorithmic.

## Lineage of this principle's recognition

- 2026-05-14, assembly-typecheck-pass-1.md initial sweep: the principle
  was implicit in the first three decisions (lex-2rsad, lex-z8m97, lex-bpr6b)
  but not yet named.
- 2026-05-14, lex-h7vet closure: the 4th application surfaced cross-domain
  convergence; the lineage entry on lex-h7vet proposed promoting it from
  ad-hoc fix to documented principle.
- 2026-05-14, this document: the principle is lifted out of per-molecule
  lineage so future pass-authors can find it independently of any
  specific molecule's history.

## Cross-references

- `assembly-typecheck-pass-1.md` — the pass where this principle was
  applied 4 times
- `composition-operations.md` — the grammar against which the principle's
  "always-present typed flow" is defined
- `SCHEMA.md` — the atom / molecule field definitions
- `lex-2rsad.yaml`, `lex-z8m97.yaml`, `lex-bpr6b.yaml`, `lex-h7vet.yaml` —
  lineage entries with worked examples and panel critiques
- `lexicon-elements-design-principles.md` — the Goertzel-filtered
  elements-META principles (this principle is structural rather
  than evaluative, so it lives alongside that doc rather than inside it)
