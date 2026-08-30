# Composition operations — atomic bonds for the chemistry-book frame

*Per the 2026-05-02 mining-strategy reframe (see `pass-1-corrections.md`):
the chemistry-book frame names atoms (single cognitive moves) and
molecules (named assemblies) as first-class. Until now the BONDS
between atoms inside a molecule were implicit — `decomposes-into:` is
a flat list with no specification of how the atoms combine. This
artifact names the bonds as a small bounded vocabulary so molecules
can describe their assembly explicitly, supporting remix and
classification of new patterns.*

## Why bonds matter

Walton's argument-from-expert-opinion (lex-kebfa) has four atoms:
source-attribution (lex-q9asc), domain-credibility (lex-dm5te),
defeasible-presumption (lex-af9ax), critical-question-checklist
(lex-th68b). Just listing them flattens real structure: the atoms
combine in a specific shape (you can't credibility-judge before
you've attributed; the presumption holds *with* the checklist as
defeaters, not *alongside* it). The shape is the interesting part —
it's what makes lex-kebfa a coherent molecule rather than four
unrelated atoms.

Naming the bonds enables three downstream moves:
1. **Recognize variant molecules.** Argument-from-witness has the
   same bond pattern as argument-from-expert-opinion (sequential
   prep then defeasibility-attach) but swaps out the credibility-
   judging atom for a witness-reliability atom. Bond-pattern
   identity at the structural level, atom-set difference at the
   content level.
2. **Predict missing molecules.** If `sequential(X, Y) →
   defeasibility-attach(Z, defeaters=W)` recurs across multiple
   molecules, encountering a new context (e.g., LLM-output as
   source) lets you construct `sequential(LLM-output-attribution,
   LLM-domain-coverage) → defeasibility-attach(presumption,
   defeaters=LLM-defeater-set)` as a predicted molecule, then
   verify.
3. **Diagnose flawed molecules.** A real-world argument that has
   source-attribution and defeasible-presumption but skips
   domain-credibility is a known failure mode (citing a source
   out of their domain). The named bond pattern makes the
   missing piece visible.

## The operation vocabulary (v0 — bounded set)

Seven operations, deliberately bounded. The 3-occurrence rule
applies to additions: don't extend until 3+ molecules surface a
bond shape that doesn't fit one of these.

### sequential(A, B, ...)

A then B then ... — each operates on the previous one's output.
Type-checked: `type-out(A) ≡ type-in(B)`. Example: in lex-kebfa,
`sequential(source-attribution, domain-credibility)` —
domain-credibility takes the attributed-source frame from
source-attribution and produces a credibility posture.

### parallel(A, B, ...)

A and B both fire on the same input independently; their outputs
are then aggregated (aggregation rule depends on use). Useful when
multiple atomic perspectives apply to the same content without
ordering. Example: a hypothetical molecule `parallel(emotional-
salience, logical-validity)` for assessing a persuasive claim
along two axes simultaneously.

### defeasibility-attach(A, defeaters=D)

A holds presumptively unless any of D's questions answer
affirmatively. The atomic operation that turns a posture into a
*retractable* posture. Example: in lex-kebfa, the molecule's
output stance is `defeasibility-attach(defeasible-presumption,
defeaters=critical-question-checklist)` — the presumption holds
unless one of the 7 critical-questions retracts it.

### choice(A, B, ...; selector=S)

A or B based on a context-classifier S. Mutually exclusive — the
selector picks one to fire. Example: a molecule `choice(narrative,
algebraic; selector=user-stance)` where the render mode depends
on whether the user is in deployment vs design stance.

### iteration(A; until=T)

A applied repeatedly until termination condition T holds.
Examples: socratic-dialogue iterates question-then-answer until
contradiction or convergence; gradient-descent iterates
take-step until improvement falls below threshold.

### conditional(A; if=X)

A applies only when condition X holds; otherwise no-op. Distinct
from choice (which picks one of multiple) — conditional is a
single optional fire. Example: `conditional(escalate-to-source-
verification; if=primary-source-disputed)`.

### scoping(A; within=S)

A operates only on entities matching scope S. The scope itself
may be a typed predicate. Example: `scoping(case-similarity-
mapping; within=legal-precedent-set)` — the mapping atom only
operates on entries that count as legal precedent, not on
arbitrary historical examples.

## How operations appear in elements/

Molecules with explicit assembly carry an optional `assembly:`
field — a single string in a small grammar:

```yaml
assembly: "sequential(lex-q9asc, lex-dm5te) → defeasibility-attach(lex-af9ax, defeaters=lex-th68b)"
```

The grammar is intentionally small for v0:
- `op(arg1, arg2, ...)` for nested operations
- Args are `lex-NNNN` ids OR other operation expressions
- Named-arg form `name=value` for keyword args (defeaters, selector,
  until, if, within)
- `→` is purely cosmetic punctuation between top-level expressions
  for readability; it has no semantic content (the sequential() op
  carries the actual ordering)

The grammar is NOT yet formally parsed by render code. v0 stores
the assembly as a string, surfaced verbatim by the introspection
mode. Future passes may add a parser if cross-molecule analysis
needs it.

## Backwards compatibility

`assembly:` is optional. Molecules without it remain valid; their
atoms-as-flat-list-via-`decomposes-into:` continues to work. The
two fields are deliberately redundant for atom listing — `assembly:`
adds bond information, but the flat list remains the canonical
machine-readable atom set (and what loader.LoadAll iterates).

This is the v0 trade: lightweight, additive, parseable-later. If
the parsed-tree form (option A I considered) becomes necessary for
downstream tooling — combinatorial enumeration, automated
substitution, type-checked composition — that's a future pass.

## Operations promoted to first-class entries (2026-05-02)

Per elements/'s own 3-occurrence rule, five of the seven
operations have been used enough across actual molecules to warrant
promotion as `lex-NNNN` entries: **sequential** (9 attestations
across lex-kebfa, 0045, 0046, 0047, 0048, 0070, 0078, 0088, 0095),
**defeasibility-attach** (4 attestations across lex-kebfa, 0045,
0047, 0048 — the four "sequential-then-defeasibility" Walton
meta-pattern molecules), **parallel** (3 attestations across
lex-z8m97, 0071, 0087 — promoted in `physics-decomposition-pass.md`
via the lex-3ydmv fermi-estimation molecule), **choice** (3
attestations across lex-bpr6b, 0089, 0095 — promoted in
`tumbler-remixer-meta-pattern.md` after lex-mnxhs in
`triz-decomposition-pass.md` pushed it past threshold), and
**iteration** (3 attestations across lex-0046 slippery-slope,
lex-h7vet depart-trial-return-arc, lex-y6pqz toy-model-then-perturb
— promoted in `verification-pass-5.md` Task 2 after recognizing
that lex-y6pqz's perturbation step is intrinsically iterative
[perturbation theory is a series expansion ψ = ψ₀ + λψ₁ + λ²ψ₂ +
..., terminating when corrections become small relative to current
state]; assembly updated from `sequential(lex-kkr43, solve-toy-
version, lex-x9pxs)` to `sequential(lex-kkr43, solve-toy-version,
iteration(lex-x9pxs; until=corrections-small))` to make the
iteration explicit).

### assembly: convention — names canonical, ids reachable via lex-NNNN

Operation names (`sequential`, `parallel`, `defeasibility-attach`,
`choice`, `iteration`, `conditional`, `scoping`) are the canonical
form in the `assembly:` field — not lex-NNNN ids. Rationale: the
human-readability gain from `sequential(lex-q9asc, lex-dm5te)` over
`lex-2sx7q(lex-q9asc, lex-dm5te)` outweighs the consistency-with-id-
elsewhere gain. The promoted lex-NNNN entries (lex-2sx7q
sequential, lex-hesjt defeasibility-attach, lex-eg9zm parallel)
provide the canonical definitions reachable by name lookup; the
operation name in an `assembly:` string IS the reference. Future
passes that promote more operations should preserve the same
convention.

Tier decision: `_tier: atomic` rather than a new `_tier: operation`.
Operations are unitary cognitive primitives at the lowest tier;
their distinction-from-other-atoms is encoded in the
`composition-operation` evokes tag. New tier value waits for 3+
operations to need it (the chemistry-book vocabulary expansion
criterion).

### lex-2sx7q — sequential

```yaml
---
id: lex-2sx7q
name: sequential
type-in: composition
type-out: composition
_tier: atomic
agent-instruction: "When ordering steps, surface that some orderings are sequential-only (B requires A finished) — name the dependencies explicitly so parallelization opportunities are visible, and so the critical path is identified rather than guessed."
related: [lex-hesjt, lex-eg9zm, lex-ucrqj, lex-da3g3, lex-qhrpv]
evokes: [composition-operation, then, A-then-B, type-flow, pipeline, function-composition]
formal-if-any: "sequential(A, B) where type-out(A) ≡ type-in(B); generalizes to sequential(A1, A2, ..., An) for n-ary chains"
lineage:
  - source: practitioner
    text: composition-operations-design-note
    citation: "v0 composition-operations vocabulary (lexicon, 2026-05-02); defined as the canonical sequential bond between atoms in a molecule"
  - source: practitioner
    text: hoare-csp-1978
    citation: "Communicating Sequential Processes — sequential composition as primitive"
  - source: practitioner
    text: substrate-attestation
    citation: "promoted to first-class entry from substrate use: one of the most frequently-used bonds across lexicon's molecule-tier assemblies (e.g. lex-kebfa argument-from-expert-opinion, lex-bpr6b polya-work-backwards-via-related-problem). Not enumerating the full membership here — it has grown well past the original handful this citation was written against, and a hardcoded list only goes stale again at the next mint. Query `assembly:.*sequential\\(` across substrate/ for the current count."
canonical-instances:
  - "in lex-kebfa argument-from-expert-opinion: sequential(source-attribution, domain-credibility) — domain-credibility takes the attributed-source frame as input"
  - "the right-arrow → in functional programming's function composition: g . f means 'apply f, then apply g to f's output'"
  - "the cognitive shape of 'first do A, then do B' where B's input semantically depends on A's output (distinct from parallel(A, B) where A and B both consume the same input)"
  - "type-checked: type-out(A) must equal type-in(B); when types don't match, an explicit conversion atom is needed in between"
critical-questions:
  - "is the type-match exact, or is implicit coercion happening? sequential(A, B) requires type-out(A) ≡ type-in(B). If they almost match but require a coercion, the coercion atom should be explicit — implicit coercion hides type-violations"
  - "is B's input semantically dependent on A's output, or are they just adjacent in time? sequential is type-flow composition, not temporal ordering. If B's input doesn't actually consume A's output (could equally well precede A), the move is parallel-then-merge, not sequential"
  - "what happens when A fails? sequential composition propagates A's failure to B in some way — either B never runs, or B receives a failure-token, or the whole chain raises. The discipline: specify the failure-mode at attach-time"
  - "is this distinct from lex-eg9zm parallel? sequential threads output-to-input (A's result feeds B); parallel runs A and B on the same input independently. Composition shape differs even though both produce composed-result"
status: active
---
```

### lex-hesjt — defeasibility-attach

```yaml
---
id: lex-hesjt
name: defeasibility-attach
type-in: composition
type-out: posture
_tier: atomic
agent-instruction: "When stating a conclusion that depends on a defeasible premise, explicitly note the defeaters that would retract it. Don't pretend the inference is unconditional when it isn't."
related: [lex-af9ax, lex-th68b, lex-2sx7q, lex-eg9zm, lex-ucrqj, lex-da3g3, lex-fpvsu, lex-xhwqt, lex-utn22, lex-aut4h, lex-zd7vh, lex-kh9cr, lex-hhx7u, lex-rnwph, lex-38ygs, lex-7t3wr, lex-y7p3z, lex-tpv83, lex-chtk9, lex-9uac6, lex-xqamc, lex-jafkg, lex-ftu47, lex-vx867, lex-ht9ud]
evokes: [composition-operation, presumption-with-defeaters, retract-on-evidence, walton-style-defeasibility, hold-lightly]
formal-if-any: "defeasibility-attach(A, defeaters=D) — A holds presumptively as a posture; if any d ∈ D answers affirmatively the posture retracts"
lineage:
  - source: practitioner
    text: composition-operations-design-note
    citation: "v0 composition-operations vocabulary (lexicon, 2026-05-02); defined as the bond that turns a presumption-atom into a retractable posture"
  - source: primary
    tradition: walton
    text: walton-reed-macagno-2008
    citation: "Douglas Walton, Chris Reed, and Fabrizio Macagno, Argumentation Schemes (Cambridge University Press 2008), Ch.1 §3 'Critical Questions,' p.15. Refs/ PDF (isbn13 9780521723749), verbatim 2026-05-21."
    quote: "One of the features of argumentation schemes that is key to evaluating whether an argument fitting a scheme should be judged strong or weak is the list of associated critical questions — questions that can be asked (or assumptions that are held) by which a nondeductive argument based on a scheme might be judged to be (or presented as being) good or fallacious. The critical questions form a vital part of the definition of a scheme, and are one of the benefits of adopting a scheme-based approach."
  - source: practitioner
    text: substrate-attestation
    citation: "promoted to first-class entry from substrate use: originally observed as the closing bond across the Walton meta-pattern molecules (e.g. lex-kebfa argument-from-expert-opinion); the four-molecule count this citation was first written against is stale — only lex-kebfa of the original set survived, and the bond has since grown to appear across substrate/'s molecule-tier assemblies well beyond that original cluster. Query `assembly:.*defeasibility-attach\\(` across substrate/ for the current count rather than trust a hardcoded list here."
canonical-instances:
  - "in lex-kebfa argument-from-expert-opinion: defeasibility-attach(defeasible-presumption, defeaters=critical-question-checklist) — the presumption holds unless one of the 7 critical questions retracts it"
  - "the cognitive operation that turns 'X is true' into 'X is true unless conditions Q1...Qn obtain', making the conclusion inspectable rather than dogmatic"
  - "operationally distinct from lex-af9ax defeasible-presumption (the atom): defeasibility-attach is the OPERATION that produces the defeasible posture; lex-af9ax is the kind of POSTURE that gets produced when a presumption is held with defeaters"
  - "the recurring closing-bond in Walton's argumentation schemes — what makes them defeasible-arguments rather than dogmatic-arguments"
critical-questions:
  - "is the attach-operation present, or is the inference dogmatic? without defeasibility-attach the scheme is treated as conclusive — the operation is what converts 'A is true' into 'A is true unless D'. Diagnostic: can the conclusion be retracted at all? If not, the attach didn't happen"
  - "is the defeater-set named at attach-time, or implicit? a defeasibility-attach with no specified D leaves the inference notionally-retractable but operationally dogmatic (no one knows what would force retraction). The discipline: specify the defeater-set explicitly at the attach"
  - "is this distinct from lex-af9ax defeasible-presumption? defeasibility-attach is the OPERATION; lex-af9ax is the kind of POSTURE produced. The operation produces the posture; the posture is what's held. Conflating misses that the attach can be analyzed (was it performed correctly?) independently of the posture"
  - "is the attach essential, or decorative? if the inference would be held with the same confidence even without the defeater-list (because the practitioner doesn't expect any defeater to fire), the attach is theater. Test: under what conditions would you actually retract?"
status: active
---
```

### lex-eg9zm — parallel

```yaml
id: lex-eg9zm
name: parallel
type-in: composition
type-out: composition
_tier: atomic
agent-instruction: "When subtasks don't depend on each other, run them in parallel. Sequential execution costs the sum of times; parallel execution costs the max. Identify the actual dependency structure before defaulting to serial."
related: [lex-2sx7q, lex-hesjt, lex-3ydmv, lex-ucrqj, lex-da3g3, lex-brcgz, lex-3fzvf, lex-mmety, lex-4e6ur, lex-c646m, lex-g9b38, lex-gu8st, lex-cat29, lex-uxqdz, lex-s7qxv, lex-wgd7u, lex-2kjmb, lex-46n25, lex-m9hc3, lex-ggpqt, lex-2vpet, lex-urwym, lex-dwey5, lex-p77e7, lex-y5jm9]
evokes: [composition-operation, simultaneous-application, independent-fire-then-aggregate, fan-out-then-merge, no-shared-state-between-arms]
formal-if-any: "parallel(A, B, ..., aggregator=φ) where each arm A, B, ... operates independently on the shared input; outputs combined via aggregator φ. Distinct from sequential(A, B): parallel arms do not consume each other's output"
lineage:
  - source: practitioner
    text: composition-operations-design-note
    citation: "v0 composition-operations vocabulary (lexicon, 2026-05-02); defined as the bond between atoms whose outputs are aggregated rather than chained"
  - source: practitioner
    text: hoare-csp-1978
    citation: "Communicating Sequential Processes — parallel composition (P || Q) as primitive, alongside sequential composition; the canonical CS formalization of independent-then-aggregated processes"
  - source: practitioner
    text: substrate-attestation
    citation: "promoted to first-class entry from substrate use: appears as the bond in 3 lexicon molecules — lex-z8m97 gof-decorator (parallel responsibility-addition arms), lex-a9wpd gof-composite (parallel children of a composite node), lex-3ydmv fermi-estimation (parallel order-of-magnitude estimates of multiplicative factors)"
canonical-instances:
  - "in lex-3ydmv fermi-estimation: parallel(estimate-factor-1, estimate-factor-2, ..., estimate-factor-n; aggregator=multiplicative-product) — each factor estimate is independent of the others (that's the whole point of choosing factors to be uncorrelated); they are then multiplied"
  - "MapReduce: the map step is parallel() over input partitions; the reduce step is the aggregator. The single most-deployed instance of this composition operation in modern computing"
  - "ensemble methods in ML: parallel(model-1, model-2, ..., model-k; aggregator=majority-vote-or-mean) — each base model is trained independently; predictions aggregated"
  - "operationally distinct from lex-2sx7q sequential: sequential(A, B) requires type-out(A) ≡ type-in(B) and B depends on A's output; parallel(A, B) requires both to consume the same input type, with no information flowing between arms — the aggregator is where the arms reunite"
  - "operationally distinct from lex-bpr6b's choice() composition: choice picks ONE arm to fire based on a selector; parallel fires ALL arms regardless and aggregates"
critical-questions:
  - "do the arms actually share no state? Apparent parallelism with hidden coupling — cache contention, shared lock, network rate-limit, single-writer database, GIL — reduces to sequential plus contention overhead. Discriminator: if you ran each arm alone with the same input, would total wall-time scale linearly? If not, parallel() is mis-named for what's happening."
  - "is the aggregator function well-defined on the union of arm output types? majority-vote presupposes outputs in the same category space; mean presupposes numeric; max presupposes ordered; concatenation presupposes serializability. An incompatible aggregator is a type error that the parallel-composition vocabulary alone won't catch."
  - "does the orchestration cost (fork, sync, network round-trip, serialization) exceed the serial baseline for the given N? Below some N or above some setup-cost, parallel costs more than it saves. The MapReduce-canonical-instance only pays off when data-volume × per-record-cost dominates fan-out overhead by a wide margin."
  - "are arm errors statistically independent? For ensemble methods (parallel(model-1, ..., model-k; majority-vote)), correlated errors don't average out — k highly-correlated estimates ≈ 1 estimate. The variance-reduction benefit assumes independence; without it, parallel is theater. Test: train arms on disjoint data subsets and measure inter-arm error correlation."
  - "is early-termination acceptable once one arm produces a sufficient answer? If yes, you may want speculative-execution / first-acceptable patterns rather than pure parallel — both fire all arms but only consume the first complete one. Pure parallel(...) commits to waiting for and aggregating ALL arms, which is sometimes the costly mistake."
status: active
```

### lex-ucrqj — choice

```yaml
---
id: lex-ucrqj
name: choice
type-in: composition
type-out: composition
_tier: atomic
agent-instruction: "When deciding feels mechanical or forced, surface that choice itself is the structural primitive — the moment where alternatives are weighable and one is selected; do not collapse choice into its inputs."
related: [lex-2sx7q, lex-hesjt, lex-eg9zm, lex-da3g3, lex-jy4aq]
evokes: [composition-operation, branching, mutual-exclusion-then-pick, switch-statement, dispatch-by-selector]
formal-if-any: "choice(A, B, ...; selector=S) where S(input) picks one arm to fire; arms are mutually exclusive — exactly one fires per input. Distinct from parallel (all fire) and sequential (chain)"
lineage:
  - source: practitioner
    text: composition-operations-design-note
    citation: "v0 composition-operations vocabulary (lexicon, 2026-05-02); defined as the bond that picks one of multiple arms based on a selector function"
  - source: primary
    text: hoare-1978-communicating-sequential-processes
    citation: "C.A.R. Hoare, 'Communicating Sequential Processes,' Communications of the ACM 21(8):666-677 (August 1978), p.669 — the alternative-command construct, built on Dijkstra's guarded commands. (Earlier lineage cited the □/⊓ operator notation from Hoare's 1985 CSP monograph; the 1978 CACM paper actually on disk uses the guarded-command formulation instead, so the citation is corrected to match the source that was actually read.)"
    quote: "'An alternative command specifies execution of exactly one of its constituent guarded commands. Consequently, if all guards fail, the alternative command fails. Otherwise an arbitrary one with successfully executable guard is selected and executed.'"
  - source: practitioner
    text: substrate-attestation
    citation: "promoted to first-class entry from substrate use: appears as the bond in 3 lexicon molecules — lex-bpr6b polya-work-backwards-via-related-problem (choose between work-backwards and find-related-problem based on recall), lex-sjsxx physics-model-regime-selection (choose framework based on identified regime), lex-mnxhs contradiction-resolution-via-parameter-decomposition (choose principle from 40 inventive principles via contradiction-matrix lookup)"
canonical-instances:
  - "in lex-sjsxx physics-model-regime-selection: choice(newtonian-framework, relativistic-framework, quantum-framework, quantum-field-theory-framework; selector=identified-scale-regime) — exactly one framework is selected based on scale analysis; using two frameworks together is a different (multi-physics) molecule"
  - "switch / case statement in any programming language: choice over a finite set of branches selected by a discriminator value; the canonical computational instance"
  - "tumblers/remixers (Tarot, Oblique Strategies, I Ching) are choice() operations with random selectors; the random-selection is the distinctive feature, not the choice-shape itself"
  - "operationally distinct from lex-2sx7q sequential: sequential CHAINS arms (each takes the previous output); choice PICKS ONE arm to fire"
  - "operationally distinct from lex-eg9zm parallel: parallel FIRES ALL arms in parallel and aggregates; choice fires exactly one"
  - "operationally adjacent to (but distinct from) Walton's defeasibility-attach (lex-hesjt): defeasibility-attach can be modeled as choice(retract, hold; selector=any-defeater-fires?) but the named operation captures the asymmetric default-vs-defeater structure that pure choice loses"
critical-questions:
  - "do the arms ACTUALLY mutually exclude on the input distribution? Choice() presupposes the selector partitions input space cleanly. Soft-exclusion (probabilistic, fuzzy match, multi-label) isn't choice — it's weighted aggregation. Discriminator: can two arms ever both have a legitimate claim on the same input? If yes, you have a different composition."
  - "is the selector decidable in bounded time on every input? Ambiguous inputs that hit two cases reveal an under-specified selector, not a working choice() operation. Test: pick three pathological inputs (edge values, missing fields, type-mismatches) and trace which arm the selector picks. Tie-breaking rules are part of the composition, not an afterthought."
  - "is there a default arm, and is its absence informative? Choice without a default returns no-output for unmatched inputs. In some contexts (switch-statements with explicit fall-through) that's a bug; in others (regime-detection with explicit no-match-found) the silence IS the signal. Decide which before deploying — silent unmatched-input is a foot-gun in the wrong context."
  - "is this choice() or choice()-with-fallback? Pure choice commits to its first pick. If the system retries the next arm on first-arm-failure, that's choice+defeasibility (lex-hesjt), not pure choice. The retry-on-failure pattern is so common it's easy to mistake for the primitive — but the type-signature is different and the failure semantics matter for reasoning about the whole molecule."
  - "what does the selector itself cost? Choice presupposes free access to selector-relevant input. When selector-evaluation is itself expensive (regime identification in physics requires substantial measurement; routing in a microservices mesh requires service-discovery), the cost-of-selection must be counted against the operation's total cost. A choice-composition where selecting which arm to fire takes longer than firing any arm is structurally wrong."
status: active
---
```

### lex-da3g3 — iteration

```yaml
id: lex-da3g3
name: iteration
type-in: composition
type-out: composition
_tier: atomic
agent-instruction: "When a problem doesn't yield to the obvious first move, build an iteration loop: small step, check, adjust. Iteration is the default move under uncertainty; jumping to the answer presumes information you may not have."
related: [lex-2sx7q, lex-hesjt, lex-y6pqz, lex-eg9zm, lex-ucrqj, lex-h7vet, lex-ts4qp, lex-3qnq3, lex-mj7x2, lex-58gqb, lex-xxmeh, lex-rxvus, lex-xu8k9, lex-svcmp, lex-3fzvf, lex-exrwy, lex-druv9, lex-w5qfe, lex-t5drx, lex-w6v3m, lex-jkpj6, lex-qbuw5, lex-ubyz3, lex-ct5ux, lex-b2c6b, lex-mqvyk, lex-4d53x, lex-a5jhg, lex-hsntu, lex-3haas, lex-jyzng, lex-vs2t5, lex-5a87w, lex-gvwvx, lex-kfdep, lex-v95t7, lex-d2tsa, lex-btfca, lex-zpfv9, lex-5qv7q, lex-c9xba, lex-555ug, lex-qg753, lex-wgd7u, lex-2kjmb, lex-zwnfz, lex-sp7fa, lex-46n25, lex-a55ep, lex-73wdz, lex-rqbsr, lex-k4ent, lex-ggpqt, lex-58ynk, lex-94nsz, lex-jd7fe, lex-9zjpf, lex-raydk, lex-d234k, lex-kykwm, lex-b6gyc, lex-2apyb, lex-697ka, lex-qa6hc, lex-kj6tj, lex-d73nj, lex-x9sda, lex-egkjk, lex-3656j, lex-cqvnu, lex-5tpy8, lex-cgbrt, lex-afzzp, lex-ktpxs, lex-m8j35, lex-tqd9x, lex-7xz6k, lex-f2ktw, lex-2kbpp, lex-kxk99, lex-2vpet, lex-hsnup, lex-urwym, lex-pn7cf, lex-6xw55, lex-dwey5, lex-p77e7, lex-xuna9, lex-y5jm9]
evokes: [composition-operation, repeated-application, until-condition, fixed-point-iteration, recurrence, loop, series-expansion]
formal-if-any: "iteration(A; until=T) where A is applied repeatedly to its own output until termination predicate T holds. T may be implicit (e.g., 'until convergence' or 'until corrections small'). Distinct from sequential (one chain, fixed length) and choice (one pick from many). Variable-length operation"
lineage:
  - source: practitioner
    text: composition-operations-design-note
    citation: "v0 composition-operations vocabulary (lexicon, 2026-05-02); defined as the bond that wraps an atom for repeated application until a termination predicate holds"
  - source: primary
    text: turing-1936-computable-numbers
    citation: "Alan Turing, 'On Computable Numbers, with an Application to the Entscheidungsproblem,' Proceedings of the London Mathematical Society s2-42(1):230-265 (received 28 May 1936, read 12 November 1936). The canonical formalization of iteration as a primitive of computability: behaviour at each moment is fixed entirely by the pair (m-configuration, scanned symbol), and the machine re-determines that pair step after step — state-transition-then-loop is the template for iteration() as a composition operation. The sharper contribution to this atom is Turing's circular/circle-free distinction, which inverts the naive reading of a termination predicate: a CIRCLE-FREE machine is one that never stops producing output of the first kind, and computability is defined in terms of it, so here the success condition is non-termination and halting is the failure. That is a live counterexample to the assumption inside this atom's own first critical-question — 'state T explicitly' does not mean 'make sure it stops.' Equivalent formalizations: the while-loop in structured programming (Dijkstra 1968), the recurrence relation in mathematics, fixed-point iteration in numerical analysis (Banach 1922). (The procured copy is a scan whose text layer is usable but imperfect — the running head renders the author as 'TUKING' — so both passages below were checked against the page images; obvious OCR corruptions were normalised and are listed in the quote entry.)"
    quote: "[§1 'Computing machines,' p.231 — the loop body, behaviour as a function of the current pair:] 'We may compare a man in the process of computing a real number to a machine which is only capable of a finite number of conditions q1, q2, ..., qI which will be called \"m-configurations\". The machine is supplied with a \"tape\" (the analogue of paper) running through it, and divided into sections (called \"squares\") each capable of bearing a \"symbol\". ... The possible behaviour of the machine at any moment is determined by the m-configuration qn and the scanned symbol S(r). This pair qn, S(r) will be called the \"configuration\": thus the configuration determines the possible behaviour of the machine.' [§2 'Definitions,' p.233 — the termination predicate, and the inversion:] 'If a computing machine never writes down more than a finite number of symbols of the first kind, it will be called circular. Otherwise it is said to be circle-free. A machine will be circular if it reaches a configuration from which there is no possible move, or if it goes on moving, and possibly printing symbols of the second kind, but cannot print any more symbols of the first kind.' [and the consequence that makes non-termination the success case:] 'A sequence is said to be computable if it can be computed by a circle-free machine.' [OCR normalisations applied to the p.231 passage, checked against the page image: ';i machine' → 'a machine'; 'q1: q2. .... qI;' → 'q1, q2, ..., qI'; '<S (r)' and '© (r)' → 'S(r)'. The p.233 passage came through the text layer clean.]"
  - source: practitioner
    text: substrate-attestation
    citation: "promoted to first-class entry from substrate use: appears as the bond in lex-y6pqz toy-model-then-perturb (`iteration(lex-x9pxs; until=corrections-small)` over perturbation orders), lex-h7vet depart-trial-return-arc (`iteration(lex-hng49, lex-zgmyw)` over trial cycles in narrative arc), and lex-w5qfe iterative-refinement-of-a-provisional-target-against-feedback (`iteration(lex-exrwy; until=corrections-small)` over refinement passes). A fourth planned molecule, slippery-slope, was reserved during early planning but never minted. Crosses distinct domains (physics methodology / narrative-arc / general iterative-refinement), satisfying the cross-domain attestation discipline"
canonical-instances:
  - "in lex-y6pqz toy-model-then-perturb: iteration(lex-x9pxs; until=corrections-small) — perturbation theory is structurally a series expansion (ψ = ψ₀ + λψ₁ + λ²ψ₂ + ...) where each order's correction depends on lower orders. Termination is when the next-order correction becomes negligible relative to current state. The iteration is mechanical, not stylistic — quantum perturbation theory, celestial mechanics, classical-field perturbation all share the shape"
  - "in lex-h7vet depart-trial-return-arc: iteration(lex-hng49, lex-zgmyw) — Propp's morphology empirically established the multi-trial structure of folktale narrative as critical (the hero faces multiple difficult-tasks, not a single one). Same shape recurring in Campbell's monomyth, the bildungsroman, the bodhisattva path, engineering career arc"
  - "in the planned-but-unminted slippery-slope molecule (reserved during early planning, never built): iteration would chain-project each consequence step (if A then B; if B then C; if C then D...), with the iteration's length equal to the chain-length"
  - "gradient-descent: iteration(take-step; until=improvement-below-threshold) — the canonical numerical optimization shape; foundational across machine learning and scientific computing. ψₙ₊₁ = ψₙ - α∇L(ψₙ) is the recurrence form"
  - "socratic-dialogue: iteration(question-then-answer; until=contradiction-or-convergence) — the structural form of dialectical inquiry; each question takes the prior answer's commitments as input and probes them"
  - "operationally distinct from lex-2sx7q sequential: sequential CHAINS distinct atoms in fixed order (sequential(A, B, C) has length exactly 3); iteration REPEATS a single atom with termination predicate (iteration(A; until=T) has variable length determined by T). The two compose: sequential(A, iteration(B), C) means 'do A, then iterate B until T, then do C'"
  - "operationally distinct from lex-eg9zm parallel: parallel fires arms simultaneously and aggregates; iteration applies one arm repeatedly with each cycle's output feeding the next. Parallel is breadth (one shot, many arms); iteration is depth (one arm, many cycles)"
  - "operationally distinct from lex-ucrqj choice: choice picks ONE arm from many to fire once; iteration repeats ONE arm until termination. Both differ from sequential in arm-count and from parallel in cardinality. The four operations (sequential, parallel, choice, iteration) span the basic shapes of compose-multiple-atoms; the remaining two (conditional, scoping) handle modifier cases (optional fire, scoped fire)"
critical-questions:
  - "the termination-predicate test: is T well-defined? Implicit-T iterations ('iterate until done') are operational anti-patterns — the iteration may not converge, may oscillate, may diverge. Practitioner-discipline: state T explicitly pre-deployment; the discipline catches these cases before they ship"
  - "the convergence-vs-divergence check: does each iteration provably bring the system closer to the termination state, or could it move away? Convergent iteration has a Lyapunov-like property (each step reduces distance to T); divergent iteration accumulates error. Practitioner-question: what's the convergence-rate, and would small perturbations destroy it?"
  - "the iteration-step-cost vs payoff: how expensive is each iteration relative to the marginal-payoff? Some iterations (gradient descent in low-loss neighborhoods; perturbation series at high order) have diminishing returns. Practitioner-discipline: budget the iteration explicitly (max-N or max-time) even when T is well-defined, so divergence-by-cost-not-content is bounded"
  - "the side-effect accumulation: does each iteration's side-effects accumulate (state changes, resource usage, observability cost)? Pure-functional iteration has no side-effect concern; impure iteration can accumulate state that affects later iterations in non-obvious ways. Practitioner-discipline: name the side-effects per iteration and verify they don't compound unexpectedly"
status: active
```

### Remaining operations (not yet promoted)

- **conditional**, **scoping** — 0 attestations.

These stay definitional-only in this artifact until the 3-occurrence
threshold is reached.

#### V6/inding: Path A — conditional/scoping may stay definitional-only

The ttestation hunt (`conditional-scoping-attestation-hunt.md`)
scanned all 10 elements molecules with explicit `assembly:` plus the
5 molecules in `molecules-pass-1.md` and found **0 hidden invocations
for both operations**. This is not because the operations are unused —
elements/'s *convention* uses other constructs to handle the same
semantic ground:

| Semantic role | Current convention | Operation-level alternative |
|---|---|---|
| Molecule entry condition | `premises:` field | `scoping(.; within=premises)` wrapping the assembly |
| Optional fire on context | `defeasibility-attach` (presumption with defeaters) OR `choice() with selector=` (mutually-exclusive dispatch) | `conditional(A; if=X)` |
| Per-arm scope restriction | `choice() with selector=` already encodes per-arm scope | `scoping(arm; within=S)` inside choice arms |

ccepts **Path A**: keep conditional() and scoping() as part of the
bounded vocabulary for *future* situations where the existing constructs
genuinely don't fit, but do not force-promote them by rewriting existing
atoms to expose the operations. This preserves elements/'s
authorial conventions (premises-as-entry-scope is well-established
through Walton schemes; defeasibility-attach handles retract-on-defeater
cleanly; choice-with-selector handles n-way dispatch including per-arm
scope).

Path B (convention shift — move scope conditions out of `premises:` and
into `scoping()` wrappers; rewrite optional-fires as `conditional()`)
remains available if the loader/render code grows behavioral consequences
for these operations (e.g., automatic defeater-checking gated on scope
membership). Until then, definitional-only is the right resting point.

This is documented per the 3-occurrence rule's spirit: the rule
*permits* promotion at 3+ attestations; it does not *require* it. And
the absence of attestations after methodical search is itself a
finding — elements/'s compositional vocabulary is richer than the
7 operations alone would suggest.

## What this artifact closes / leaves open

**Closes:**
- The implicit-bond gap in the chemistry-book frame. Molecules can
  now describe their assembly explicitly.
- The first concrete deliverable of the 2026-05-02 mining-strategy
  reframe (Pilot C from `pass-1-corrections.md`).

**Leaves open:**
- Pilot A (GoF decomposition pass) — uses these operations to
  decompose 23 GoF patterns into cross-attested atoms. Immediate
  next move.
- Formal grammar / parser for the assembly string. v0 is
  human-readable only.
- Type-checking via composition rules (does `sequential(A, B)`
  actually type-check given A's type-out and B's type-in?). Useful
  diagnostic; not v0.
- Whether operations become first-class lex-NNNN entries (pending
  3-occurrence rule on composition shapes recurring).
- Backfilling existing molecules (lex-0045 through lex-0048 — the
  other Walton molecules) with assembly fields. Demonstrative;
  worth doing but not strictly required for this artifact to ship.

## Reference example (worked)

lex-kebfa argument-from-expert-opinion:

```yaml
decomposes-into: [lex-q9asc, lex-dm5te, lex-af9ax, lex-th68b]
assembly: "sequential(lex-q9asc, lex-dm5te) → defeasibility-attach(lex-af9ax, defeaters=lex-th68b)"
```

Read: *first attribute the source, then judge domain credibility,
then hold the resulting presumption with the critical-question
checklist as the bound defeater set.* The bond structure is what
distinguishes argument-from-expert-opinion from a flat
"these-four-atoms-fired" non-pattern.

## Update — 2026-06-05: `classification` operator added

Per ROADMAP #15 (metapattern labels): elements/ gains the ability
for one molecule to LABEL a set of other atoms-or-molecules sharing a
structural signature, by introducing a new assembly operator
`classification(...)`. Labels are not a new tier — they ARE molecules,
with `_tier: molecule` and a `classification(...)` assembly. The
semantic difference between a label-molecule and a composition-
molecule lives in the operator, not in the tier.

**Operator:** `classification(A, B, C, ..., "<shape description>")`

Semantics:
- Members A, B, C, ... are UNORDERED — there is no "first then second"
  composition; each member independently instantiates the shape.
- The trailing string is a short human-readable description of the
  shape (the same way `iteration(A; until=X)` carries a stopping
  condition string).
- Type-checking is permissive: no constraint that members share
  type-in or type-out, because label-members can be heterogeneous
  primitives that happen to share a structural signature (e.g.
  Walton schemes share the `sequential(prep) → defeasibility-attach`
  shape but their preparation-stages differ in type).
- Type-in / type-out of the label-molecule pass through the first
  arm (same convention as `defeasibility-attach`), so the molecule
  can carry its own type signature for consumers that need one.

**Bar for minting a label-molecule** (per ROADMAP #15 pinned decisions):
Principle-9 cross-source attestation applied to the metapattern claim
itself. The label is justified when ≥1 refs-grounded primary source
explicitly identifies the shape as a coherent kind (e.g. Walton 2008
names "argumentation scheme" and catalogs 60+ instances; that names
the kind). Two methodologically-distinct primaries for graduation.
Looser bars reinvent the informal `evokes:` field.

**First instance** — `walton-style-defeasible-scheme` ():
```yaml
_tier: molecule
decomposes-into: [lex-af9ax, lex-kebfa, lex-th68b, lex-hesjt,
                  lex-xhwqt, lex-utn22, lex-aut4h, lex-zd7vh, lex-kh9cr, lex-hhx7u]
assembly: 'classification(lex-xhwqt, lex-utn22, lex-aut4h, lex-zd7vh, lex-kh9cr, lex-hhx7u, "Walton-scheme shape — sequential prep → defeasibility-attach with CQ-checklist defeaters")'
```

Read: *these six argumentation-scheme atoms (sign, position-to-know,
commitment, slippery-slope, analogy, ignorance) all instantiate the
same defeasible-presumptive scheme-shape — they share lex-af9ax +
lex-th68b + lex-hesjt as compositional infrastructure and were
catalogued by Walton-Reed-Macagno 2008 as a coherent kind.* The
membership list is elements/'s first machine-checkable assertion
of metapattern structure; previously this claim lived only in prose
inside `docs/principles/`.

Composes with:
- `decomposes-into:` — the label-molecule's `decomposes-into:` lists
  both the shape-infrastructure atoms AND the member atoms; the
  `assembly: classification(...)` distinguishes them.
- `pedagogy-gloss:` — optional one-line human-facing description for
  render output / Anki / hook surface (introduced alongside
  the label-molecule).
- Existing `related:` reciprocation discipline — members carry the
  label-molecule's ID back, exactly like ordinary `related:` edges.
