// Mechanically sliced from render/internal/viz/shell.go's gapSuggestions
// (lines 415-586 as of the 2026-08-20 gap-triage pass) — hand-authored
// candidate hypotheses for the pivot's empty (type_in x type_out) cells.
// Exact text, not retyped: a citation-bearing content asset, ported by
// direct extraction to avoid transcription drift. Keep in sync with the
// Go source if either side is edited; the two aren't currently linted
// against each other.

export interface GapSuggestion {
  name: string
  why: string
}

export const GAP_SUGGESTIONS: Record<string, GapSuggestion[]> = {
    'question__posture': [
      { name: 'aporia-from-elenchus', why: 'Plato — Socratic question induces the perplexed-suspended stance' },
      { name: 'premortem-question-shifts-stance', why: 'Klein — "imagine this has failed" question shifts the asker from optimist to forensic posture' },
      { name: 'reductio-question-forces-disavowal', why: '"does X really imply Y?" locks the interlocutor into either rejecting X or accepting Y — the question shapes their stance' },
      { name: 'naive-question-permits-explanation', why: 'Feynman — asking the dumb question opens a stance in which expertise can be talked about without performing it' },
    ],
    'frame__process': [
      { name: 'paradigm-determines-method', why: 'Kuhnian — the frame dictates what counts as a legitimate experimental process' },
      { name: 'diagnosis-determines-treatment', why: 'medical-reasoning canonical: the diagnostic frame entails the treatment process' },
      { name: 'threat-frame-triggers-fight-flight-freeze', why: 'Cannon — appraising a stimulus as threat selects the SNS-mediated mobilization process' },
      { name: 'design-brief-shapes-search', why: 'the framing of the design problem determines which solution-search procedure makes sense' },
    ],
    'frame__frame': [
      { name: 'aspect-flip', why: 'Wittgenstein duck-rabbit — one frame transforms into another via gestalt switch' },
      { name: 'metaphor-extension', why: 'extending an existing frame as a metaphor for a new domain produces a new frame' },
      { name: 'zoom-out-reframes-political-as-structural', why: 'change of time-horizon transforms one frame to another (politics ↔ economics, mood ↔ physiology)' },
      { name: 'perspective-swap-via-role-taking', why: 'Mead — taking the role of the other generates a new interpretive frame' },
    ],
    'frame__claim': [
      { name: 'abductive-inference', why: 'Peirce — the frame ("there must be a reason") yields the best-explanation claim' },
      { name: 'theory-laden-observation', why: 'Hanson — the frame determines what claims are observable as data' },
      { name: 'framing-effect-shifts-decision', why: 'Tversky-Kahneman — the same data produces different claims when the frame (gain vs loss) shifts' },
      { name: 'analogy-transfers-claims-along-mapped-structure', why: 'analogical inference — claims from the source frame inherit into the target via the mapping' },
    ],
    'posture__state': [
      { name: 'stoic-equanimity-from-acceptance', why: 'Aurelius — amor-fati posture induces the equanimity state' },
      { name: 'embodiment-shapes-physiology', why: 'a sustained posture (slow breath, upright stance) shifts the autonomic state' },
      { name: 'anxious-posture-amplifies-threat-perception', why: 'bodily tension biases the perceived state — the world reads as more threatening' },
      { name: 'savoring-stance-extends-pleasure-state', why: 'deliberately attending to enjoyable feeling extends the subjective duration of the state' },
    ],
    'posture__process': [
      { name: 'growth-mindset-yields-iteration', why: 'Dweck — the I-can-improve posture commits you to the practice process' },
      { name: 'curiosity-yields-investigation', why: 'the curious stance licenses (and structures) the investigation process' },
      { name: 'humility-licenses-revision', why: 'the humble stance enables update-iteration; certainty forecloses it' },
      { name: 'inquiry-stance-yields-search', why: '"I don\'t know" as a stance structures the seeking process' },
    ],
    'posture__frame': [
      { name: 'standpoint-yields-perspective', why: 'standpoint-epistemology — social position (posture) shapes interpretive frame' },
      { name: 'embodied-perspective', why: 'Merleau-Ponty — the bodily stance constitutes the perceptual frame' },
      { name: 'outsider-stance-foregrounds-implicit-norms', why: 'naive-eye reveals what insiders take for granted — stance opens the frame' },
      { name: 'marginal-stance-yields-double-vision', why: 'being inside-and-outside enables a comparative frame the dominant cannot see' },
    ],
    'posture__claim': [
      { name: 'bullshit-as-truth-indifferent-stance', why: 'Frankfurt — the careless posture produces claims without truth-tracking' },
      { name: 'commitment-precedes-evidence', why: 'belief-stance generates assertions in advance of supporting evidence' },
      { name: 'principled-stance-locks-in-claims', why: 'committed advocacy produces high-conviction claims even on weak evidence' },
      { name: 'ironic-distance-undercuts-own-claims', why: 'the posture itself disclaims the claim — every assertion is held with a wink' },
    ],
    // Below: 5-cell triage sample (2026-08-20) out of 28 uncovered gap
    // cells, picked to demonstrate the triage bar (a real, exactly-fitting
    // canonical source, not a strained post-hoc rationalization — the
    // type-signature grid is coarse enough that almost any cell CAN be
    // rationalized, which is not the same as being a genuine gap) rather
    // than to cover the grid. 23 uncovered cells remain untriaged.
    'state__question': [
      { name: 'felt-information-gap-generates-question-asking', why: "Loewenstein 1994 — the felt state of an information gap is itself the mechanism that produces question-asking behavior; distinct from lex-dvydj's uncertainty-reduction *process* (that cell's output is the reduction process, this one's is the question artifact itself)" },
      { name: 'anomaly-state-prompts-explanatory-question', why: 'Kuhnian anomaly — noticing an anomalous state ("this doesn\'t fit") is the trigger that produces a why-question, prior to and separate from any investigation process' },
    ],
    'process__typology': [
      { name: 'ideal-type-construction-yields-typology', why: "Weber's method — selective accentuation of features across cases (a comparative-abstraction process) is how an ideal-type typology gets constructed, not discovered ready-made" },
      { name: 'free-listing-elicitation-yields-folk-taxonomy', why: "cognitive anthropology (D'Andrade) — a structured elicitation procedure run across informants is the process that produces a folk-taxonomy as its output" },
    ],
    'question__warning': [
      { name: 'loaded-question-triggers-presupposition-warning', why: 'the classical complex-question/loaded-question fallacy — a question that smuggles an unproven premise is the input, and the correct response is a warning flagging the presupposition rather than answering directly' },
      { name: 'leading-question-in-testimony-triggers-reliability-warning', why: "eyewitness-testimony research (Loftus) — a question's leading form, independent of its content, is what should trigger a reliability warning about the answer it elicits" },
    ],
    'composition__claim': [
      { name: 'assembled-scale-produces-emergent-claim', why: '"More Is Different" (Anderson 1972) — a composition of many simple assembled parts, at sufficient scale, yields a claim not derivable from any single part\'s properties; the elements already has this idea from the claim-in side (lex-eybad), this is the composition-as-input framing specifically' },
      { name: 'gestalt-whole-yields-claim-parts-cant', why: "Wertheimer — a perceptual composition (the assembled whole) supports a claim about its own organization that no enumeration of its parts individually would license" },
    ],
    'structure__posture': [
      { name: 'panoptic-structure-induces-self-disciplining-posture', why: "Foucault, Discipline and Punish — the architectural structure of visibility (real or merely possible observation) produces the self-monitoring posture in the observed, independent of whether anyone is actually watching" },
      { name: 'built-environment-shapes-behavioral-stance', why: 'Churchill\'s dictum ("we shape our buildings, and afterwards our buildings shape us") — a structural/spatial configuration is the mechanism, the posture it produces is the dependent variable' },
    ],
    // Below: the remaining 23 cells from the same 2026-08-20 triage pass,
    // same bar (an exactly-fitting canonical source, checked against the
    // nearest existing elements atom by type-signature, not just by
    // shared author/topic). All 31 empty cells are now triaged.
    'state__structure': [
      { name: 'far-from-equilibrium-state-organizes-into-dissipative-structure', why: "Prigogine — a sustained non-equilibrium state (energy/matter flux) is the mechanism that produces an ordered spatial or temporal structure (convection cells, chemical oscillators), not an external designer" },
      { name: 'supersaturated-state-precipitates-lattice-structure', why: 'basic crystallization chemistry — a state past its solubility threshold is the direct mechanism producing a specific crystal-lattice structure, no equilibrium-thermodynamics apparatus required' },
    ],
    'situation__composition': [
      { name: 'messy-situation-decomposes-into-interacting-sub-system-composition', why: "Checkland's Soft Systems Methodology — a real-world situation too ill-defined to solve directly is rendered tractable by representing it as a composition of interacting sub-systems" },
      { name: 'a-mess-is-a-system-of-interacting-problems', why: "Ackoff — a situation of many entangled difficulties resolves into a composition once its component problems and their interactions are made explicit, rather than being solved problem-by-problem" },
    ],
    'process__question': [
      { name: 'iterated-why-process-generates-successive-questions', why: "Toyota's Five Whys — the procedure itself, not any single answer, is what generates the next diagnostic question, terminating only when a process step stops producing a new one" },
      { name: 'bisection-debugging-process-emits-a-question-per-step', why: "binary-search debugging (e.g. git bisect) — each step of the process is structured as a single yes/no diagnostic question narrowing the search space, the question is the process's per-step output" },
    ],
    'question__composition': [
      { name: 'posed-question-decomposes-into-sub-problem-composition', why: "Pólya, How to Solve It — a problem stated as a question is worked by decomposing it into a composition of more tractable sub-problems, then re-assembling their solutions" },
      { name: 'requirement-question-decomposes-into-functional-composition', why: 'systems-engineering functional decomposition — a top-level "what must this do" question is answered by decomposing it into a composition of discrete sub-requirements' },
    ],
    'question__typology': [
      { name: 'dichotomous-key-question-sequence-partitions-into-typology', why: "field-biology identification keys — a fixed sequence of yes/no questions is the mechanism that sorts specimens into a typology; close neighbor lex-ts4qp (diairesis, question→process) is the same recursive-bisection idea aimed at a definition rather than a classification output" },
      { name: 'optimal-question-sequence-partitions-search-space', why: "Shannon-style twenty-questions — an information-theoretically optimal sequence of binary questions is what produces a classification tree of the domain" },
    ],
    'frame__question': [
      { name: 'how-might-we-reframing-generates-investigation-questions', why: '"How Might We" (IDEO) — deliberately recasting a problem into this specific frame is what generates the divergent-search questions a design team investigates next' },
      { name: 'legal-characterization-frame-determines-discovery-questions', why: 'characterizing a dispute under one legal frame rather than another (contract vs. tort) is what determines which discovery and precedent questions become relevant to ask' },
    ],
    'frame__warning': [
      { name: 'detecting-a-loaded-frame-triggers-persuasion-warning', why: "Lakoff — noticing that a debate has already been framed a specific way (e.g. 'tax relief' presupposes taxation is an affliction) is itself the mechanism that should trigger a warning that persuasive work is happening invisibly" },
      { name: 'named-propaganda-device-triggers-recognition-warning', why: "the Institute for Propaganda Analysis's classic device checklist (bandwagon, glittering generalities, etc.) — recognizing a rhetorical frame by name is the warning-generating mechanism, prior to evaluating its content" },
    ],
    'claim__structure': [
      { name: 'shared-derived-character-claims-assemble-into-cladogram-structure', why: "Hennig's cladistics — claims of shared derived characters across taxa are the input, and systematizing them produces a branching-tree structure (the cladogram) as output; near neighbor lex-eadyj (claim→frame) fossilizes descent into an interpretive frame rather than a structural diagram" },
      { name: 'case-holdings-synthesize-into-doctrinal-structure', why: 'common-law doctrinal synthesis — a body of individual case-claims (holdings), once systematized by commentators, is what produces the structure of a legal doctrine' },
    ],
    'posture__question': [
      { name: 'methodological-doubt-posture-generates-systematic-questioning', why: "Descartes' Meditations — deliberately sustaining the posture of doubt toward everything is the mechanism that produces the systematic sequence of questions, not any single doubted belief" },
      { name: 'professional-skepticism-posture-generates-audit-questions', why: 'auditing standards (ISA 200 "professional skepticism") — a formally required skeptical stance is what an auditor is expected to hold as the generative mechanism for probing questions, independent of any specific red flag' },
    ],
    'posture__composition': [
      { name: 'improvisational-stance-assembles-real-time-composition', why: "jazz improvisation (Berliner, Thinking in Jazz) — the performer's real-time, uncertainty-tolerant stance is the generative mechanism that assembles the composition in the moment, not a pre-written score" },
      { name: 'adaptive-command-posture-assembles-ad-hoc-unit-composition', why: "Boyd's maneuver-warfare doctrine — a command posture built for rapid adaptation is what assembles ad hoc force compositions on the fly, rather than executing a fixed order of battle" },
    ],
    'posture__structure': [
      { name: 'defensive-posture-produces-alliance-structure', why: 'the security dilemma (Herz 1950, Jervis 1978) — a state\'s defensive posture (arming for its own security) is the mechanism that produces alliance and balance-of-power structure across a system of states' },
      { name: 'founding-posture-toward-power-produces-constitutional-structure', why: "Arendt, On Revolution — the founders' own stance toward the power they hold is what determines the constitutional structure (checks, balances) that gets built, not the other way around" },
    ],
    'posture__typology': [
      { name: 'splitter-lumper-disposition-determines-taxonomy-produced', why: "Mayr's systematics terminology — the same specimen data, classified by a splitting vs. lumping disposition, yields a different typology; the posture is upstream of the typology, not a comment on it" },
      { name: 'clinical-vs-actuarial-posture-determines-diagnostic-typology', why: "Meehl 1954 — a practitioner's clinical-judgment vs. statistical-prediction stance determines which diagnostic typology they end up applying to the same case data" },
    ],
    'posture__warning': [
      { name: 'institutionalized-dissent-posture-produces-warning', why: "Janis's groupthink research — a deliberately assigned devil's-advocate posture is the mechanism designed to produce a warning that would otherwise be suppressed by group cohesion pressure" },
      { name: 'surveillance-posture-produces-anomaly-warning', why: 'immunological surveillance — a constant background-monitoring posture (not a triggered response) is the mechanism that produces the inflammatory warning signal once something anomalous is detected' },
    ],
    'composition__state': [
      { name: 'team-composition-determines-group-state', why: "group-diversity/faultline research (Lau & Murnighan 1998) — a team's demographic and skill composition is a direct predictor of its resulting state (cohesion, psychological safety), independent of any single member" },
      { name: 'alloy-composition-determines-material-state', why: 'basic metallurgy — the specific composition of an alloy (e.g. carbon content in steel) is the direct mechanism determining its resulting physical state (brittle vs. ductile)' },
    ],
    'composition__process': [
      { name: 'assembled-composition-runs-as-emergent-process', why: "Herbert Simon, The Architecture of Complexity — a hierarchically composed system's higher-level process behavior emerges once its components are correctly assembled (near-decomposability), not from any single component" },
      { name: 'enzyme-composition-determines-active-metabolic-pathway', why: 'biochemistry — the specific composition of enzymes present in a cell is what determines which metabolic process (pathway) actually runs' },
    ],
    'composition__question': [
      { name: 'fragment-composition-generates-reconstructive-question', why: "Cuvier's principle of the correlation of parts — an assembled composition of skeletal fragments is what generates the reconstructive question (what animal produced this), not any single fragment" },
      { name: 'excavated-assemblage-generates-site-function-question', why: 'archaeological method — a composition of artifacts recovered together (an assemblage) is what generates the question of the site\'s function or dating' },
    ],
    'composition__structure': [
      { name: 'atomic-composition-determines-crystal-structure', why: "crystallography — the specific composition and arrangement of atoms is the direct mechanism determining the resulting lattice structure (unit cell, symmetry group)" },
      { name: 'material-composition-determines-load-bearing-structure', why: "structural engineering — the composition of a building's material elements (walls, trusses) is what determines its engineered structure, at a different scale than the crystallography candidate above" },
    ],
    'composition__warning': [
      { name: 'drug-composition-triggers-interaction-warning', why: 'pharmacology — a specific combination (composition) of substances triggers a contraindication warning that neither substance alone would produce' },
      { name: 'incompatible-chemical-composition-triggers-safety-warning', why: 'industrial chemical safety (e.g. bleach + ammonia) — the same combination-triggers-warning logic as the drug-interaction candidate, in a different domain' },
    ],
    'structure__state': [
      { name: 'spatial-structure-determines-physiological-recovery-state', why: "Ulrich 1984 (Science) — a hospital room's physical structure (window view onto nature vs. a wall) is the mechanism producing a measurably different physiological recovery state in patients" },
      { name: 'room-acoustic-structure-determines-intelligibility-state', why: "architectural acoustics — a room's physical structure (reverberation time, materials) directly determines the resulting state of speech intelligibility and listener stress" },
    ],
    'structure__question': [
      { name: 'classification-structure-gaps-generate-existence-question', why: "Mendeleev's periodic table — an empty cell in a classificatory structure is the mechanism generating a specific existence question (is there an element with this atomic weight); near neighbor lex-3d2he (state→state) treats the same mechanism as a state-to-state prediction rather than a structure-to-question move" },
      { name: 'space-group-enumeration-gaps-generate-existence-question', why: 'crystallography\'s 230 space groups — historically, gaps in the systematic enumeration of possible crystal structures generated the question of whether physical crystals with those specific symmetries actually existed' },
    ],
    'structure__structure': [
      { name: 'accommodation-transforms-cognitive-structure-into-new-structure', why: "Piaget — an existing cognitive structure (schema), when it fails to fit new experience, is transformed via accommodation into a new structure, not discarded and rebuilt from scratch" },
      { name: 'transformation-rules-map-deep-structure-to-surface-structure', why: "Chomsky's transformational grammar — a deep syntactic structure is mapped by transformation rules onto a distinct surface structure; ties to the linguistics-foundations cluster (lex-d6f8b/1113/1114)" },
    ],
    'structure__typology': [
      { name: 'comparative-grammatical-structure-yields-language-typology', why: "Greenberg's linguistic universals — comparing grammatical structures (word order) across languages is the mechanism that produces a typology of language types (SVO/SOV/VSO)" },
      { name: 'comparative-floor-plan-structure-yields-building-typology', why: 'architectural history — comparing building structures (floor plans) across a corpus is how architectural historians construct a building typology (e.g. the Palladian villa type)' },
    ],
    'structure__warning': [
      { name: 'tight-coupling-structure-produces-systemic-risk-warning', why: "Perrow's Normal Accident Theory — a system's structural property of tight coupling plus interactive complexity is the diagnostic input that produces a risk warning, independent of any single component's reliability" },
      { name: 'crack-pattern-structure-triggers-safety-warning', why: 'structural engineering inspection — a literal structural signature (crack pattern, deflection) is the direct diagnostic input engineers use to issue a safety warning or condemnation notice' },
    ],
  }
