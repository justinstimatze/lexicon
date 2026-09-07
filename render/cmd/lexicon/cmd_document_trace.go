package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/lens"
	pkglexicon "github.com/justinstimatze/lexicon/render/pkg/lexicon"
)

// cmdDocumentTrace walks one or more whole documents paragraph by paragraph
// and, for each paragraph, records which corpus atoms it instantiates — an
// ORDERED trace of pattern hits across a document, as opposed to
// `lexicon read`'s single-passage snapshot. Built for the web/ SPA's
// precomputed "Trace" tab: run once at build time against a fixed manifest
// of demo documents, never called live from the browser.
//
// Scoring is Corpus.TracePassage (pkg/lexicon/passage.go), not ScoreRaw:
// a wider embed-gate funnel, a passage-specific lens prompt that returns a
// verbatim evidence span per pick, ranking by lens confidence only, and
// zero hits allowed. A passage the pipeline could not judge is written as
// untraced with the reason — never filled with keyword collisions.
//
// Usage:
//
//	lexicon document-trace -manifest documents/manifest.json -out web/src/data/document-traces.json
type docManifestEntry struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Year      int    `json:"year"`
	SourceURL string `json:"source_url,omitempty"`
	TextFile  string `json:"text_file"`
	// MinWords overrides the global -min-words flag for this document only.
	// nil means "use the flag's value" -- most documents don't need this;
	// it exists for a source whose natural paragraphing doesn't suit the
	// default floor (e.g. a single continuous paragraph split into
	// sentences instead, which need a near-zero floor to survive as
	// separate chunks).
	MinWords *int `json:"min_words,omitempty"`
	// ChunkingNote, when set, discloses that this document's chunk
	// boundaries were imposed by this tool rather than being the source's
	// own paragraphing -- e.g. a speech transcribed as one continuous
	// paragraph, split here by sentence so the trace has more than one
	// step. Surfaced in the frontend so a reader isn't misled into
	// thinking the author wrote it with these breaks.
	ChunkingNote string `json:"chunking_note,omitempty"`
}

type docManifest struct {
	Documents []docManifestEntry `json:"documents"`
}

type docTraceChunk struct {
	Index int `json:"index"`
	// CharStart/CharEnd are rune counts (not byte offsets) into the
	// document's full_text, so a frontend can slice them directly with
	// JavaScript's UTF-16-code-unit string indexing -- see the conversion
	// at the call site in cmdDocumentTrace.
	CharStart int    `json:"char_start"`
	CharEnd   int    `json:"char_end"`
	Excerpt   string `json:"excerpt"`
	// Traced is true when the lens judged this passage, whether or not
	// it found anything. False means the pipeline could not judge it;
	// TraceNote says why.
	Traced    bool   `json:"traced"`
	TraceNote string `json:"trace_note,omitempty"`
}

type docTraceHit struct {
	ChunkIndex int     `json:"chunk_index"`
	AtomID     string  `json:"atom_id"`
	Name       string  `json:"name"`
	Tier       string  `json:"tier"`
	Confidence float64 `json:"confidence"`
	// Evidence is the lens's verbatim quote; EvidenceStart/End are rune
	// offsets into full_text (same space as chunk char_start/char_end),
	// absent when the quote could not be anchored to the passage.
	Evidence        string `json:"evidence,omitempty"`
	EvidenceStart   *int   `json:"evidence_start,omitempty"`
	EvidenceEnd     *int   `json:"evidence_end,omitempty"`
	EvidencePartial bool   `json:"evidence_partial,omitempty"`
	Why             string `json:"why,omitempty"`
	// GateRank is the atom's position in the embed gate's ranking — the
	// recall diagnostic for -candidates.
	GateRank int `json:"gate_rank"`
	// Agreement is how many of the run's candidate pools produced this
	// hit (see mergeTraces). 1 in a single-pool run.
	Agreement int `json:"agreement"`
}

type docTraceDoc struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	Year         int    `json:"year"`
	SourceURL    string `json:"source_url,omitempty"`
	ChunkingNote string `json:"chunking_note,omitempty"`
	// FullText is the same trimmed text splitParagraphs chunked -- shipped
	// so a frontend can render the whole document with chunks highlighted
	// inline, rather than only ever seeing a truncated excerpt.
	FullText string          `json:"full_text"`
	Chunks   []docTraceChunk `json:"chunks"`
	Hits     []docTraceHit   `json:"hits"`
}

type docTraceOutput struct {
	GeneratedAt string `json:"generated_at"`
	Model       string `json:"model"`
	// Candidates lists the embed-gate pool sizes the lens was run with —
	// one entry per pool. More than one means the output is a merge (see
	// mergeTraces) and each hit's agreement says how many pools chose it.
	Candidates    candidateList `json:"candidates"`
	MaxPicks      int           `json:"max_picks"`
	MinConfidence float64       `json:"min_confidence"`
	Documents     []docTraceDoc `json:"documents"`
}

// candidateList reads both the current array form and the single number
// earlier outputs carried, so -merge and -reanchor accept old files.
type candidateList []int

func (c *candidateList) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*c = candidateList{n}
		return nil
	}
	var arr []int
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*c = candidateList(arr)
	return nil
}

// mergeTraces folds several runs over the same documents into one. The
// embed gate's ranking carries little information past its first ten or
// so (an 80-candidate run drew picks uniformly from ranks 11-80), and two
// runs over different pools agreed on only 142 of 247 top picks — so a
// pick that the lens made against two different sets of alternatives is
// better evidence than one it made once. Per chunk, hits are keyed by
// atom: agreement counts the runs that produced it, confidence is the
// max, evidence and why come from the most confident record, gate_rank
// is the min. A hit only one pool produced must clear singleFloor (a
// pick made once needs more confidence than a pick made twice; a plain
// union filled nearly every passage to the cap). Ordered by agreement
// then confidence, capped at maxPicks. Runs must cover the same
// documents with identical chunking; the first run's documents, chunks
// and full_text are kept.
func mergeTraces(runs []docTraceOutput, maxPicks int, singleFloor float64) docTraceOutput {
	base := runs[0]
	out := docTraceOutput{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Model:         base.Model,
		MaxPicks:      maxPicks,
		MinConfidence: base.MinConfidence,
	}
	for _, r := range runs {
		out.Candidates = append(out.Candidates, r.Candidates...)
	}
	for di, bd := range base.Documents {
		merged := docTraceDoc{
			ID: bd.ID, Title: bd.Title, Author: bd.Author, Year: bd.Year, SourceURL: bd.SourceURL,
			ChunkingNote: bd.ChunkingNote, FullText: bd.FullText, Chunks: bd.Chunks, Hits: []docTraceHit{},
		}
		type key struct {
			chunk int
			atom  string
		}
		acc := map[key]*docTraceHit{}
		var order []key
		for _, r := range runs {
			if di >= len(r.Documents) || r.Documents[di].ID != bd.ID || len(r.Documents[di].Chunks) != len(bd.Chunks) {
				fatal("document-trace: merge: run documents differ at %q — same manifest and -min-words required", bd.ID)
			}
			for _, c := range r.Documents[di].Chunks {
				// A chunk untraced in any run stays traced if some run judged it;
				// its note is dropped once a judgment exists.
				if c.Traced && !merged.Chunks[c.Index].Traced {
					merged.Chunks[c.Index].Traced = true
					merged.Chunks[c.Index].TraceNote = ""
				}
			}
			for _, h := range r.Documents[di].Hits {
				k := key{h.ChunkIndex, h.AtomID}
				if cur, ok := acc[k]; ok {
					cur.Agreement++
					if h.GateRank > 0 && (cur.GateRank == 0 || h.GateRank < cur.GateRank) {
						cur.GateRank = h.GateRank
					}
					if h.Confidence > cur.Confidence {
						rank, agree := cur.GateRank, cur.Agreement
						*cur = h
						cur.GateRank, cur.Agreement = rank, agree
					}
					continue
				}
				hh := h
				hh.Agreement = 1
				acc[k] = &hh
				order = append(order, k)
			}
		}
		byChunk := map[int][]*docTraceHit{}
		for _, k := range order {
			h := acc[k]
			if len(runs) > 1 && h.Agreement < 2 && h.Confidence < singleFloor {
				continue
			}
			byChunk[k.chunk] = append(byChunk[k.chunk], h)
		}
		for _, c := range merged.Chunks {
			hs := byChunk[c.Index]
			sort.SliceStable(hs, func(i, j int) bool {
				if hs[i].Agreement != hs[j].Agreement {
					return hs[i].Agreement > hs[j].Agreement
				}
				return hs[i].Confidence > hs[j].Confidence
			})
			if len(hs) > maxPicks {
				hs = hs[:maxPicks]
			}
			for _, h := range hs {
				merged.Hits = append(merged.Hits, *h)
			}
		}
		out.Documents = append(out.Documents, merged)
	}
	return out
}

func mergeTraceFiles(paths []string, outPath string, maxPicks int, singleFloor float64) {
	var runs []docTraceOutput
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			fatal("document-trace: read %s: %v", p, err)
		}
		var r docTraceOutput
		if err := json.Unmarshal(data, &r); err != nil {
			fatal("document-trace: parse %s: %v", p, err)
		}
		runs = append(runs, r)
	}
	if len(runs) == 0 {
		fatal("document-trace: -merge needs at least one file")
	}
	if maxPicks <= 0 {
		maxPicks = runs[0].MaxPicks
	}
	merged := mergeTraces(runs, maxPicks, singleFloor)
	writeTrace(merged, outPath)
	printTraceSummary(merged, outPath)
}

func writeTrace(out docTraceOutput, outPath string) {
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fatal("document-trace: marshal: %s", err)
	}
	if outPath == "" {
		os.Stdout.Write(data)
		os.Stdout.Write([]byte("\n"))
		return
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatal("document-trace: mkdir: %s", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		fatal("document-trace: write: %s", err)
	}
}

func parseCandidateList(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			fatal("document-trace: -candidates: %q is not a positive integer", part)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		fatal("document-trace: -candidates: empty")
	}
	return out
}

var blankLineRun = regexp.MustCompile(`\n\s*\n+`)

// paragraphSpan is one candidate paragraph before floor-merge, with its byte
// offsets into the original (trimmed) document text.
type paragraphSpan struct {
	text       string
	start, end int
}

// splitParagraphs splits text on runs of blank lines, trims each candidate,
// drops empty ones, then merges any paragraph under minWords forward into
// the next paragraph (or backward into the last emitted one, if it's the
// trailing paragraph with nothing to merge forward into). Spans carry byte
// offsets into the original text -- Go string indexing is byte-based, and
// this function needs that for strings.Index. The caller converts to rune
// counts before serializing (see cmdDocumentTrace), since a frontend slicing
// full_text with these offsets does so with JavaScript's UTF-16-code-unit
// string indexing, not bytes.
func splitParagraphs(text string, minWords int) []paragraphSpan {
	var raw []paragraphSpan
	pos := 0
	for _, part := range blankLineRun.Split(text, -1) {
		start := strings.Index(text[pos:], part) + pos
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			// Recompute start/end against the trimmed text within this part.
			leadingTrim := strings.Index(part, trimmed)
			s := start + leadingTrim
			e := s + len(trimmed)
			raw = append(raw, paragraphSpan{text: trimmed, start: s, end: e})
		}
		pos = start + len(part)
	}

	var merged []paragraphSpan
	for i := 0; i < len(raw); i++ {
		p := raw[i]
		for len(strings.Fields(p.text)) < minWords && i+1 < len(raw) {
			i++
			next := raw[i]
			p = paragraphSpan{text: p.text + "\n\n" + next.text, start: p.start, end: next.end}
		}
		merged = append(merged, p)
	}
	// A trailing short paragraph that had nothing left to merge forward into
	// (the loop above only merges forward) gets folded backward instead,
	// rather than shipped as its own noise chunk.
	if len(merged) >= 2 {
		last := merged[len(merged)-1]
		if len(strings.Fields(last.text)) < minWords {
			prev := merged[len(merged)-2]
			merged[len(merged)-2] = paragraphSpan{text: prev.text + "\n\n" + last.text, start: prev.start, end: last.end}
			merged = merged[:len(merged)-1]
		}
	}
	return merged
}

// reanchorTrace re-locates every hit's evidence quote inside its chunk's
// slice of full_text and rewrites evidence_start/evidence_end. It exists
// because the quotes are the durable part of a run and the offsets are
// derived: a fix to the anchoring (or to the frontend's expectations)
// should not cost another pass through the lens.
func reanchorTrace(inPath, outPath string) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		fatal("document-trace: read %s: %v", inPath, err)
	}
	var out docTraceOutput
	if err := json.Unmarshal(data, &out); err != nil {
		fatal("document-trace: parse %s: %v", inPath, err)
	}
	moved, anchored, lost := 0, 0, 0
	for di := range out.Documents {
		d := &out.Documents[di]
		runes := []rune(d.FullText)
		chunkAt := make(map[int]docTraceChunk, len(d.Chunks))
		for _, c := range d.Chunks {
			chunkAt[c.Index] = c
		}
		for hi := range d.Hits {
			h := &d.Hits[hi]
			if h.Evidence == "" {
				continue
			}
			c, ok := chunkAt[h.ChunkIndex]
			if !ok || c.CharStart < 0 || c.CharEnd > len(runes) || c.CharStart > c.CharEnd {
				continue
			}
			slice := string(runes[c.CharStart:c.CharEnd])
			s, e, partial, found := lens.LocateEvidence(slice, h.Evidence)
			if !found {
				lost++
				h.EvidenceStart, h.EvidenceEnd, h.EvidencePartial = nil, nil, false
				fmt.Fprintf(os.Stderr, "%s chunk %d: %s evidence not found: %q\n", d.ID, h.ChunkIndex, h.AtomID, h.Evidence)
				continue
			}
			s, e = c.CharStart+s, c.CharStart+e
			if h.EvidenceStart == nil || h.EvidenceEnd == nil || *h.EvidenceStart != s || *h.EvidenceEnd != e || h.EvidencePartial != partial {
				moved++
			}
			h.EvidenceStart, h.EvidenceEnd, h.EvidencePartial = &s, &e, partial
			anchored++
		}
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fatal("document-trace: marshal: %s", err)
	}
	if outPath == "" {
		outPath = inPath
	}
	if err := os.WriteFile(outPath, enc, 0o644); err != nil {
		fatal("document-trace: write: %s", err)
	}
	fmt.Printf("reanchored %s -> %s: %d spans anchored, %d moved, %d not found\n", inPath, outPath, anchored, moved, lost)
}

func excerpt(s string, maxLen int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return strings.TrimSpace(string(r[:maxLen])) + "…"
}

func cmdDocumentTrace(renderDir string, args []string) {
	fl := flag.NewFlagSet("document-trace", flag.ExitOnError)
	manifestPath := fl.String("manifest", "", "path to a JSON manifest listing {id,title,author,year,source_url,text_file}")
	out := fl.String("out", "", "output path for the document-trace JSON (default: stdout)")
	candidates := fl.String("candidates", "50,80", "embed-gate pool sizes the lens is run with, comma-separated; more than one means each passage is judged once per pool and the picks merged by agreement")
	maxPicks := fl.Int("max-picks", lens.MaxPassagePicks, "most patterns recorded per passage")
	mergeFiles := fl.String("merge", "", "comma-separated existing output files to fold into one by agreement (see -candidates), written to -out; no manifest, no API calls")
	minConfidence := fl.Float64("min-confidence", 0.6, "drop lens picks below this confidence")
	singleFloor := fl.Float64("single-floor", 0.75, "with several candidate pools, a pick only one pool made must reach this confidence to ship")
	model := fl.String("model", client.Model, "lens model for the passage judgment")
	retries := fl.Int("lens-retries", 2, "retries per passage on a failed lens call")
	workers := fl.Int("workers", 2, "passages judged concurrently (each is one local embed plus one lens call)")
	minWords := fl.Int("min-words", 40, "paragraphs shorter than this get merged into a neighbor before matching")
	reanchor := fl.String("reanchor", "", "re-locate every hit's evidence span in an existing output file against its own full_text and write it to -out (no manifest, no API calls)")
	if err := fl.Parse(args); err != nil {
		fatal("parse flags: %s", err)
	}
	if *reanchor != "" {
		reanchorTrace(*reanchor, *out)
		return
	}
	if *mergeFiles != "" {
		mergeTraceFiles(strings.Split(*mergeFiles, ","), *out, *maxPicks, *singleFloor)
		return
	}
	pools := parseCandidateList(*candidates)
	if *manifestPath == "" {
		fatal("document-trace: -manifest is required")
	}
	// Both stages' defaults are hook-latency guards: a live turn must
	// never hang on them. This is a batch precompute, where a slow answer
	// costs nothing and a timed-out one costs the passage (it ships as
	// untraced — never as a keyword fallback, but still a gap). The
	// 2026-09-06 run lost 159 of 247 chunks to the 6s gate budget on a
	// host that was swapping.
	if os.Getenv("LEXICON_LENS_TIMEOUT_MS") == "" {
		_ = os.Setenv("LEXICON_LENS_TIMEOUT_MS", "90000")
	}
	if os.Getenv("LEXICON_EMBED_GATE_BUDGET_MS") == "" {
		_ = os.Setenv("LEXICON_EMBED_GATE_BUDGET_MS", "60000")
	}

	manifestData, err := os.ReadFile(*manifestPath)
	if err != nil {
		fatal("document-trace: read manifest %s: %v", *manifestPath, err)
	}
	var manifest docManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		fatal("document-trace: parse manifest %s: %v", *manifestPath, err)
	}
	if len(manifest.Documents) == 0 {
		fatal("document-trace: manifest %s lists no documents", *manifestPath)
	}
	manifestDir := filepath.Dir(*manifestPath)

	if lens.Disabled() {
		fatal("document-trace: the lens is disabled (no ANTHROPIC_API_KEY, or LEXICON_LENS_DISABLED=1) — there is no keyword-only mode for this command; run it from render/ with .env present")
	}

	corp := loadCorpusOrFatal(renderDir)

	// One run per candidate pool, then a merge by agreement. A single
	// pool is the degenerate case: the merge of one run is that run with
	// agreement 1 everywhere.
	var runs []docTraceOutput
	for _, pool := range pools {
		opts := pkglexicon.TraceOptions{
			Candidates:    pool,
			MaxPicks:      *maxPicks,
			MinConfidence: *minConfidence,
			Model:         *model,
			LensRetries:   *retries,
		}
		var docs []docTraceDoc
		for _, m := range manifest.Documents {
			textPath := filepath.Join(manifestDir, m.TextFile)
			raw, err := os.ReadFile(textPath)
			if err != nil {
				fatal("document-trace: read %s (doc %s): %v", textPath, m.ID, err)
			}
			text := strings.TrimSpace(string(raw))
			if text == "" {
				fatal("document-trace: %s is empty", textPath)
			}
			effectiveMinWords := *minWords
			if m.MinWords != nil {
				effectiveMinWords = *m.MinWords
			}
			fmt.Fprintf(os.Stderr, "%s: pool of %d candidates\n", m.ID, pool)
			chunks, hits := traceDocument(corp, m, text, splitParagraphs(text, effectiveMinWords), opts, *workers)
			docs = append(docs, docTraceDoc{
				ID: m.ID, Title: m.Title, Author: m.Author, Year: m.Year, SourceURL: m.SourceURL,
				ChunkingNote: m.ChunkingNote, FullText: text, Chunks: chunks, Hits: hits,
			})
		}
		runs = append(runs, docTraceOutput{
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			Model:         *model,
			Candidates:    candidateList{pool},
			MaxPicks:      *maxPicks,
			MinConfidence: *minConfidence,
			Documents:     docs,
		})
	}
	output := mergeTraces(runs, *maxPicks, *singleFloor)
	writeTrace(output, *out)
	if *out != "" {
		printTraceSummary(output, *out)
	}
}

// traceDocument judges every span of one document under one option set.
// Passages are independent; a few are judged at once. Results land in
// index order regardless of completion order, so the output is
// byte-stable across worker counts.
func traceDocument(corp *pkglexicon.Corpus, m docManifestEntry, text string, spans []paragraphSpan, opts pkglexicon.TraceOptions, workers int) ([]docTraceChunk, []docTraceHit) {
	chunks := make([]docTraceChunk, 0, len(spans))
	hits := []docTraceHit{}
	meta := lens.PassageMeta{Title: m.Title, Author: m.Author, Year: m.Year}

	results := make([]pkglexicon.TraceResult, len(spans))
	sem := make(chan struct{}, max(1, workers))
	var wg sync.WaitGroup
	var stderrMu sync.Mutex
	for i, span := range spans {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, passage string) {
			defer wg.Done()
			defer func() { <-sem }()
			r := corp.TracePassage(context.Background(), passage, meta, opts)
			stderrMu.Lock()
			for _, d := range r.Diagnostics {
				fmt.Fprintf(os.Stderr, "%s chunk %d: %s\n", m.ID, i, d)
			}
			stderrMu.Unlock()
			results[i] = r
		}(i, span.text)
	}
	wg.Wait()

	{
		for i, span := range spans {
			res := results[i]
			charStart := utf8.RuneCountInString(text[:span.start])
			// span.text is what the lens read: trimmed paragraphs joined with
			// "\n\n". The document's own text between span.start and
			// span.end has CRLFs, indentation and trailing spaces at those
			// joins, so an offset into span.text is NOT an offset into
			// full_text — every merged boundary shifted everything after it
			// by a few characters (51 of 432 spans in the first run ended
			// mid-word: "pr|ure", "outc|ry"). Anchor against the original
			// slice, whose offsets are the ones the frontend uses.
			rawSlice := text[span.start:span.end]
			chunks = append(chunks, docTraceChunk{
				Index:     i,
				CharStart: charStart,
				CharEnd:   utf8.RuneCountInString(text[:span.end]),
				Excerpt:   excerpt(span.text, 160),
				Traced:    res.Traced,
				TraceNote: res.Note,
			})
			for _, h := range res.Hits {
				tier := h.Entry.Tier
				if tier == "" {
					tier = "atomic"
				}
				hit := docTraceHit{
					ChunkIndex:      i,
					AtomID:          h.Entry.ID,
					Name:            h.Entry.Name,
					Tier:            tier,
					Confidence:      h.Confidence,
					Evidence:        h.Evidence,
					EvidencePartial: h.EvidencePartial,
					Why:             h.Why,
					GateRank:        h.GateRank,
				}
				if h.Evidence != "" {
					if s, e, partial, ok := lens.LocateEvidence(rawSlice, h.Evidence); ok {
						s, e = charStart+s, charStart+e
						hit.EvidenceStart, hit.EvidenceEnd, hit.EvidencePartial = &s, &e, partial
					} else {
						hit.EvidencePartial = false
						fmt.Fprintf(os.Stderr, "%s chunk %d: %s evidence not found in original slice: %q\n", m.ID, i, h.Entry.ID, h.Evidence)
					}
				}
				hits = append(hits, hit)
			}
		}
	}
	return chunks, hits
}

// printTraceSummary is the stderr/stdout report a run ends with. The
// gate-rank histogram answers "is the pool wide enough": a last bucket
// that hasn't fallen off means the lens was still finding picks at the
// pool's edge. The agreement line is the reliability read for a merged
// run — hits every pool chose versus hits only one did.
func printTraceSummary(out docTraceOutput, outPath string) {
	totalHits, totalChunks, tracedChunks, emptyChunks, anchored, partial := 0, 0, 0, 0, 0, 0
	buckets := [5]int{} // 1-10, 11-20, 21-35, 36-50, 51+
	agreement := map[int]int{}
	for _, d := range out.Documents {
		nTraced, nEmpty := 0, 0
		perChunk := map[int]int{}
		for _, h := range d.Hits {
			perChunk[h.ChunkIndex]++
			agreement[h.Agreement]++
			switch {
			case h.GateRank <= 10:
				buckets[0]++
			case h.GateRank <= 20:
				buckets[1]++
			case h.GateRank <= 35:
				buckets[2]++
			case h.GateRank <= 50:
				buckets[3]++
			default:
				buckets[4]++
			}
			if h.EvidenceStart != nil {
				anchored++
				if h.EvidencePartial {
					partial++
				}
			}
		}
		for _, c := range d.Chunks {
			if c.Traced {
				nTraced++
				if perChunk[c.Index] == 0 {
					nEmpty++
				}
			}
		}
		totalHits += len(d.Hits)
		totalChunks += len(d.Chunks)
		tracedChunks += nTraced
		emptyChunks += nEmpty
		fmt.Fprintf(os.Stderr, "%s: traced %d/%d passages, %d hits, %d passages with nothing\n", d.ID, nTraced, len(d.Chunks), len(d.Hits), nEmpty)
	}
	fmt.Printf("wrote %s (%d documents, %d passages, %d hits; traced %d/%d, %d traced with nothing)\n",
		outPath, len(out.Documents), totalChunks, totalHits, tracedChunks, totalChunks, emptyChunks)
	fmt.Fprintf(os.Stderr, "gate rank of hits: 1-10: %d · 11-20: %d · 21-35: %d · 36-50: %d · 51+: %d (pools=%v)\n",
		buckets[0], buckets[1], buckets[2], buckets[3], buckets[4], out.Candidates)
	if len(out.Candidates) > 1 {
		var parts []string
		for a := len(out.Candidates); a >= 1; a-- {
			parts = append(parts, fmt.Sprintf("%d pools: %d", a, agreement[a]))
		}
		fmt.Fprintf(os.Stderr, "agreement: %s\n", strings.Join(parts, " · "))
	}
	fmt.Fprintf(os.Stderr, "evidence anchored: %d/%d hits (%d partial)\n", anchored, totalHits, partial)
	if tracedChunks < totalChunks {
		fmt.Fprintf(os.Stderr, "WARNING: %d of %d passages are untraced — read the diag lines above before committing this output\n",
			totalChunks-tracedChunks, totalChunks)
	}
}
