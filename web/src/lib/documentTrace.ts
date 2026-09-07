// Mirrors render/cmd/lexicon/cmd_document_trace.go's docTraceOutput/
// docTraceDoc/docTraceChunk/docTraceHit. `lexicon document-trace` is the
// source of truth — every document is walked paragraph by paragraph at
// build time, never hand-annotated here.

export interface DocumentTraceChunk {
  index: number
  char_start: number
  char_end: number
  excerpt: string
  // The lens judged this passage (possibly finding nothing). False means
  // the pipeline could not judge it; trace_note says why. There is no
  // keyword fallback: an untraced passage has no hits, by design.
  traced: boolean
  trace_note?: string
}

export interface DocumentTraceHit {
  chunk_index: number
  atom_id: string
  name: string
  tier: string
  // The lens's estimate (0–1) that the pattern's mechanism is present in
  // the passage — the only ranking signal. Everything shipped is at or
  // above the run's min_confidence.
  confidence: number
  // The lens's verbatim quote of the words that carry the match, with
  // rune offsets into full_text when the quote anchored to the passage
  // (evidence_partial: only its leading words did).
  evidence?: string
  evidence_start?: number
  evidence_end?: number
  evidence_partial?: boolean
  // One sentence on how this passage does what the pattern names.
  why?: string
  // Position in the embed gate's candidate ranking — a recall diagnostic,
  // not something a reader needs.
  gate_rank: number
}

export interface DocumentTraceDoc {
  id: string
  title: string
  author: string
  year: number
  source_url?: string
  // Set only when this document's chunk boundaries were imposed by the
  // tool rather than being the source's own paragraphing (e.g. a speech
  // transcribed as one continuous paragraph, split by sentence here so
  // the trace has more than one step) — surfaced verbatim in the UI so a
  // reader isn't misled into thinking the author wrote it with these
  // breaks.
  chunking_note?: string
  // The same trimmed text `chunks[].char_start`/`char_end` index into —
  // rune counts, matching JS string-index semantics for this corpus.
  full_text: string
  chunks: DocumentTraceChunk[]
  hits: DocumentTraceHit[]
}

export interface DocumentTraceData {
  generated_at: string
  model: string
  candidates: number
  max_picks: number
  min_confidence: number
  documents: DocumentTraceDoc[]
}

// One chunk's hits, strongest first — the shape every view in this tab
// actually wants, derived here once rather than re-sorted per render.
export interface ChunkWithHits extends DocumentTraceChunk {
  hits: DocumentTraceHit[]
}

export function chunksWithHits(doc: DocumentTraceDoc): ChunkWithHits[] {
  const byChunk = new Map<number, DocumentTraceHit[]>()
  for (const h of doc.hits) {
    const arr = byChunk.get(h.chunk_index) ?? []
    arr.push(h)
    byChunk.set(h.chunk_index, arr)
  }
  for (const arr of byChunk.values()) arr.sort((a, b) => b.confidence - a.confidence)
  return doc.chunks.map((c) => ({ ...c, hits: byChunk.get(c.index) ?? [] }))
}

// Lives here rather than in TraceNetwork.tsx (which also uses it) because
// TraceNetwork is deliberately lazy()-loaded to keep react-force-graph-2d
// out of the eager Trace-tab bundle — DocumentTrace.tsx, which renders
// eagerly, needs this same color mapping for its full-text highlights and
// must not import anything from that lazy module to get it.
export const TIER_COLOR: Record<string, string> = {
  atomic: "#8a7a5c",
  molecule: "#c98a4b",
  reaction: "#7fb0d6",
}

// ---------------------------------------------------------------------------
// Display helpers for the Text column.

// A paragraph as displayed, plus the map from each displayed character
// back to its offset in the raw slice it came from. Three of the five
// source texts are Gutenberg-style hard-wrapped at ~70 columns with CRLF
// line ends; rendered verbatim those wraps read as ragged, arbitrary line
// breaks. A blank line is the author's paragraph break; a lone line break
// inside a paragraph is the transcriber's and becomes a space. The map is
// what lets an evidence span, recorded as offsets into the raw text, land
// on the right displayed characters after that unwrapping.
export interface DisplayParagraph {
  text: string
  // map[i] is the raw-slice offset of text[i]; a collapsed whitespace run
  // maps to its first raw character.
  map: number[]
}

export function displayParagraphs(raw: string): DisplayParagraph[] {
  const out: DisplayParagraph[] = []
  let text = ""
  let map: number[] = []
  let pendingSpace = -1
  let newlines = 0
  const flush = () => {
    if (text.length > 0) out.push({ text, map })
    text = ""
    map = []
  }
  for (let i = 0; i < raw.length; i++) {
    const ch = raw[i]
    if (ch === " " || ch === "\t" || ch === "\n" || ch === "\r") {
      // A CRLF pair is one line break; a lone CR is one; an LF is one.
      if (ch === "\n" || (ch === "\r" && raw[i + 1] !== "\n")) newlines++
      if (pendingSpace < 0) pendingSpace = i
      continue
    }
    if (pendingSpace >= 0) {
      if (newlines >= 2) flush()
      else if (text.length > 0) {
        text += " "
        map.push(pendingSpace)
      }
      pendingSpace = -1
      newlines = 0
    }
    text += ch
    map.push(i)
  }
  flush()
  return out
}

// The unwrapped paragraphs of a slice, text only.
export function paragraphsOf(text: string): string[] {
  return displayParagraphs(text).map((p) => p.text)
}

// A range in raw-slice offsets to mark, and the hit it belongs to.
export interface EvidenceMark {
  start: number
  end: number
  hit: DocumentTraceHit
}

export interface TextSegment {
  text: string
  // null for unmarked text; otherwise the strongest hit covering it.
  hit: DocumentTraceHit | null
}

// Splits a displayed paragraph into runs of marked and unmarked text.
// marks are in raw-slice offsets (the same space as DisplayParagraph.map).
// Where marks overlap, the earlier entry in `marks` wins, so pass them
// strongest first.
export function segmentParagraph(p: DisplayParagraph, marks: EvidenceMark[]): TextSegment[] {
  if (marks.length === 0) return [{ text: p.text, hit: null }]
  const out: TextSegment[] = []
  let cur: TextSegment | null = null
  for (let i = 0; i < p.text.length; i++) {
    const raw = p.map[i]
    let hit: DocumentTraceHit | null = null
    for (const m of marks) {
      if (raw >= m.start && raw < m.end) {
        hit = m.hit
        break
      }
    }
    if (cur && cur.hit === hit) {
      cur.text += p.text[i]
    } else {
      cur = { text: p.text[i], hit }
      out.push(cur)
    }
  }
  return out
}

// Evidence marks for a set of hits, relative to the chunk's own slice.
export function evidenceMarks(chunk: DocumentTraceChunk, hits: DocumentTraceHit[]): EvidenceMark[] {
  const out: EvidenceMark[] = []
  for (const h of hits) {
    if (h.evidence_start === undefined || h.evidence_end === undefined) continue
    const start = h.evidence_start - chunk.char_start
    const end = h.evidence_end - chunk.char_start
    if (end <= start) continue
    out.push({ start, end, hit: h })
  }
  return out
}

// Chunk indices where a given atom fires, in reading order.
export function atomPassages(doc: DocumentTraceDoc, atomId: string): number[] {
  return [...new Set(doc.hits.filter((h) => h.atom_id === atomId).map((h) => h.chunk_index))].sort((a, b) => a - b)
}

// Splits a kebab-case atom name into display lines for a canvas label:
// greedy word wrap at maxChars, capped at maxLines with an ellipsis.
export function wrapName(name: string, maxChars = 18, maxLines = 3): string[] {
  const words = name.split("-")
  const lines: string[] = []
  let cur = ""
  for (const w of words) {
    const next = cur ? `${cur} ${w}` : w
    if (next.length > maxChars && cur) {
      lines.push(cur)
      cur = w
    } else {
      cur = next
    }
  }
  if (cur) lines.push(cur)
  if (lines.length > maxLines) {
    const kept = lines.slice(0, maxLines)
    kept[maxLines - 1] = kept[maxLines - 1].replace(/\s+\S*$/, "") + "…"
    return kept
  }
  return lines
}

// ---------------------------------------------------------------------------
// Graph views. A node is one OCCURRENCE — one pattern firing in one
// passage, with its own quote behind it — not the pattern itself. That is
// what lets a click on a node mean "go to that passage", and what makes a
// pattern that fires in twelve passages twelve clickable places instead of
// one node with twelve destinations.

export interface TraceGraphNode {
  // `${chunk_index}:${atom_id}`
  id: string
  atomId: string
  name: string
  tier: string
  chunkIndex: number
  confidence: number
  evidence?: string
  // In how many passages of the whole document this pattern fires.
  recurrence: number
}

export interface TraceGraphLink {
  source: string
  target: string
  // "transition": the TOP hit of one passage was followed by the top hit
  // of the next hit-bearing passage — directed, read as "led to".
  // "co-occurrence": two hits in the same passage — undirected.
  // "recurrence": the same pattern firing again in a later passage.
  kind: "transition" | "co-occurrence" | "recurrence"
}

export interface TraceGraphData {
  nodes: TraceGraphNode[]
  links: TraceGraphLink[]
}

export function occurrenceId(chunkIndex: number, atomId: string): string {
  return `${chunkIndex}:${atomId}`
}

function recurrenceCounts(doc: DocumentTraceDoc): Map<string, number> {
  const m = new Map<string, number>()
  for (const i of new Set(doc.hits.map((h) => `${h.chunk_index}:${h.atom_id}`))) {
    const atomId = i.slice(i.indexOf(":") + 1)
    m.set(atomId, (m.get(atomId) ?? 0) + 1)
  }
  return m
}

// The neighbourhood of one passage: all of its hits, plus the top hit of
// the passage before and the passage after (transitions run between tops,
// so nothing an edge needs is lost, and the node count stays at four to
// six — the number whose full names fit in a 360px column). Obsidian's
// local-graph pane is the model.
export function localTraceGraph(doc: DocumentTraceDoc, chunks: ChunkWithHits[], activeIndex: number): TraceGraphData {
  const rec = recurrenceCounts(doc)
  const window = chunks
    .filter((c) => Math.abs(c.index - activeIndex) <= 1 && c.hits.length > 0)
    .map((c) => (c.index === activeIndex ? c : { ...c, hits: c.hits.slice(0, 1) }))

  const nodes: TraceGraphNode[] = []
  const links: TraceGraphLink[] = []
  const seen = new Set<string>()
  for (const c of window) {
    for (const h of c.hits) {
      const id = occurrenceId(c.index, h.atom_id)
      if (seen.has(id)) continue
      seen.add(id)
      nodes.push({
        id,
        atomId: h.atom_id,
        name: h.name,
        tier: h.tier,
        chunkIndex: c.index,
        confidence: h.confidence,
        evidence: h.evidence,
        recurrence: rec.get(h.atom_id) ?? 1,
      })
    }
  }
  let prevTop: string | undefined
  for (const c of window) {
    const [top, ...rest] = c.hits
    const topId = occurrenceId(c.index, top.atom_id)
    if (prevTop) links.push({ source: prevTop, target: topId, kind: "transition" })
    prevTop = topId
    for (const other of rest) links.push({ source: topId, target: occurrenceId(c.index, other.atom_id), kind: "co-occurrence" })
  }
  // The same pattern in two passages of the window: drawn as a recurrence
  // so the reader sees "this one again", not two unrelated nodes.
  for (let a = 0; a < nodes.length; a++) {
    for (let b = a + 1; b < nodes.length; b++) {
      if (nodes[a].atomId === nodes[b].atomId && nodes[a].chunkIndex !== nodes[b].chunkIndex) {
        links.push({ source: nodes[a].id, target: nodes[b].id, kind: "recurrence" })
      }
    }
  }
  return { nodes, links }
}

// Every pattern that fires in the document with the passages it fires
// in, most recurrent first — the whole-text arc view is drawn from this.
export interface AtomOccurrences {
  atomId: string
  name: string
  tier: string
  chunks: number[]
}

export function atomOccurrences(doc: DocumentTraceDoc): AtomOccurrences[] {
  const m = new Map<string, AtomOccurrences>()
  for (const h of doc.hits) {
    const o = m.get(h.atom_id) ?? { atomId: h.atom_id, name: h.name, tier: h.tier, chunks: [] }
    if (!o.chunks.includes(h.chunk_index)) o.chunks.push(h.chunk_index)
    m.set(h.atom_id, o)
  }
  const out = [...m.values()]
  for (const o of out) o.chunks.sort((a, b) => a - b)
  out.sort((a, b) => b.chunks.length - a.chunks.length || a.name.localeCompare(b.name))
  return out
}

