package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// cmdDocumentTrace walks one or more whole documents paragraph by paragraph
// and, for each paragraph, records which corpus atoms it matches — an
// ORDERED trace of pattern hits across a document, as opposed to
// `lexicon read`'s single-passage snapshot. Built for the web/ SPA's
// precomputed "Trace" tab: run once at build time against a fixed manifest
// of demo documents, never called live from the browser.
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
}

type docManifest struct {
	Documents []docManifestEntry `json:"documents"`
}

type docTraceChunk struct {
	Index     int    `json:"index"`
	CharStart int    `json:"char_start"`
	CharEnd   int    `json:"char_end"`
	Excerpt   string `json:"excerpt"`
	LensUsed  bool   `json:"lens_used"`
}

type docTraceHit struct {
	ChunkIndex   int     `json:"chunk_index"`
	AtomID       string  `json:"atom_id"`
	Name         string  `json:"name"`
	Tier         string  `json:"tier"`
	Score        float64 `json:"score"`
	LexicalMatch bool    `json:"lexical_match"`
}

type docTraceDoc struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Author    string          `json:"author"`
	Year      int             `json:"year"`
	SourceURL string          `json:"source_url,omitempty"`
	Chunks    []docTraceChunk `json:"chunks"`
	Hits      []docTraceHit   `json:"hits"`
}

type docTraceOutput struct {
	GeneratedAt string        `json:"generated_at"`
	TopK        int           `json:"top_k"`
	NoLens      bool          `json:"no_lens"`
	Documents   []docTraceDoc `json:"documents"`
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
// trailing paragraph with nothing to merge forward into). Byte offsets are
// tracked against the original text so the frontend can anchor a chunk back
// to its source position.
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

func excerpt(s string, maxLen int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxLen {
		return s
	}
	return strings.TrimSpace(s[:maxLen]) + "…"
}

func cmdDocumentTrace(renderDir string, args []string) {
	fl := flag.NewFlagSet("document-trace", flag.ExitOnError)
	manifestPath := fl.String("manifest", "", "path to a JSON manifest listing {id,title,author,year,source_url,text_file}")
	out := fl.String("out", "", "output path for the document-trace JSON (default: stdout)")
	topK := fl.Int("top-k", 2, "atoms surfaced per chunk")
	minScore := fl.Float64("min-score", 0, "drop hits below this score (0 = no filtering; the right value is unverified until run once and eyeballed)")
	noLens := fl.Bool("no-lens", false, "skip the LLM-backed semantic lens (lexical-only on full pool)")
	minWords := fl.Int("min-words", 40, "paragraphs shorter than this get merged into a neighbor before matching")
	if err := fl.Parse(args); err != nil {
		fatal("parse flags: %s", err)
	}
	if *manifestPath == "" {
		fatal("document-trace: -manifest is required")
	}
	if os.Getenv("LEXICON_LENS_TIMEOUT_MS") == "" {
		_ = os.Setenv("LEXICON_LENS_TIMEOUT_MS", "30000")
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

	corp := loadCorpusOrFatal(renderDir)

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

		spans := splitParagraphs(text, *minWords)
		chunks := make([]docTraceChunk, 0, len(spans))
		var hits []docTraceHit
		for i, span := range spans {
			picked, scores, lexMatch, lensUsed, diag := corp.ScoreRaw(context.Background(), span.text, *topK, *noLens)
			for _, d := range diag {
				fmt.Fprintf(os.Stderr, "%s chunk %d: %s\n", m.ID, i, d)
			}
			chunks = append(chunks, docTraceChunk{
				Index:     i,
				CharStart: span.start,
				CharEnd:   span.end,
				Excerpt:   excerpt(span.text, 160),
				LensUsed:  lensUsed,
			})
			for _, e := range picked {
				score := scores[e.ID]
				if score < *minScore {
					continue
				}
				tier := e.Tier
				if tier == "" {
					tier = "atomic"
				}
				hits = append(hits, docTraceHit{
					ChunkIndex:   i,
					AtomID:       e.ID,
					Name:         e.Name,
					Tier:         tier,
					Score:        score,
					LexicalMatch: lexMatch[e.ID],
				})
			}
		}
		if hits == nil {
			hits = []docTraceHit{}
		}
		docs = append(docs, docTraceDoc{
			ID: m.ID, Title: m.Title, Author: m.Author, Year: m.Year, SourceURL: m.SourceURL,
			Chunks: chunks, Hits: hits,
		})
	}

	output := docTraceOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		TopK:        *topK,
		NoLens:      *noLens,
		Documents:   docs,
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fatal("document-trace: marshal: %s", err)
	}
	if *out == "" {
		os.Stdout.Write(data)
		os.Stdout.Write([]byte("\n"))
		return
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fatal("document-trace: mkdir: %s", err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fatal("document-trace: write: %s", err)
	}
	totalHits, totalChunks := 0, 0
	for _, d := range docs {
		totalHits += len(d.Hits)
		totalChunks += len(d.Chunks)
	}
	fmt.Printf("wrote %s (%d documents, %d chunks, %d hits)\n", *out, len(docs), totalChunks, totalHits)
}
