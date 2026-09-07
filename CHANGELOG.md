# Changelog

Releases are git tags; the tag is the only version string. Earlier tags
(v0.1.2 through v0.1.5, all 2026-08-31) predate this file and were
Pages deploys of the web app with no separate notes.

## v0.2.0 — 2026-09-07

### Added

- **Trace tab** in the web app: five public-domain texts (Federalist
  No. 10, Common Sense, A Modest Proposal, the Gettysburg Address, the
  Cross of Gold speech) read passage by passage against the whole
  catalog. Three coordinated columns: the text with the words that
  carry each match marked in place, a panel for the selected passage
  (pattern, confidence, one sentence on why, where else it fires), and
  a graph of the passage's neighbourhood or an arc diagram of the whole
  text where every dot is one pattern firing in one passage. Selection
  lives in the URL.
- **`lexicon document-trace`**, the precompute behind that tab. Each
  passage is judged by a passage-specific lens that returns a
  confidence, a verbatim evidence span, and a one-sentence why per
  pick; ranking is by confidence alone, a passage may carry nothing,
  and there is no keyword fallback. Each passage is judged against two
  embed-gate pools (50 and 80 candidates) and the picks are merged by
  agreement. `-reanchor` re-locates quotes in an existing output and
  `-merge` folds outputs, neither needing an API call.
- **Catalog loading with progress.** The web app fetches the catalog as
  a JSON asset with a progress bar instead of bundling it into a JS
  chunk, and says what it is waiting on.
- This changelog.

### Changed

- **`lexicon read` and the MCP `lexicon_read` tool** no longer apply the
  name-token keyword boost once the lens has ranked candidates. The
  boost predates the lens and was letting a word collision outrank the
  lens's own confidence; it still applies on the no-lens path, where it
  is the only signal. Every consumer's ranking changes with this.
- The hook lens prompt's JSON example uses placeholder ids rather than
  live atom ids, which a model could echo into a pick.
- The embed gate keeps its parsed prototype file in memory for the
  process instead of re-reading it on every call.
- The Greene power-tactics label (`lex-ppm3j`) now says what the family
  is for: recognizing a catalog tactic in deployment is evidence about
  the deployer, and the tell is legibility.
- `scripts/build-web.sh` never regenerates the Trace data on its own;
  the committed file ships unless `LEXICON_REGEN_TRACE=1`.

### Corpus

- 3696 → 3983 atoms since v0.1.5, across roughly ninety mining and
  promotion passes: Lakoff's *Moral Politics* chapter by chapter, Réti's
  *Modern Ideas in Chess*, *The Dictator's Handbook*, Altshuller,
  Merton's strain typology, the Arthashastra's weak-king section and the
  Thirty-Six Stratagems as molecules, Arrow's theorem and Sen's liberal
  paradox as molecules, Boyd, Chomsky on Skinner, and a run of
  experience-design sources (Es Devlin, Mars College, the Latitude
  Society, Wasteland Weekend).
