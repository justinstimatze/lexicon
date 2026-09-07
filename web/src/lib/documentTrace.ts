// Mirrors render/cmd/lexicon/cmd_document_trace.go's docTraceOutput/
// docTraceDoc/docTraceChunk/docTraceHit. `lexicon document-trace` is the
// source of truth — every document is walked paragraph by paragraph at
// build time, never hand-annotated here.

export interface DocumentTraceChunk {
  index: number
  char_start: number
  char_end: number
  excerpt: string
  lens_used: boolean
}

export interface DocumentTraceHit {
  chunk_index: number
  atom_id: string
  name: string
  tier: string
  score: number
  lexical_match: boolean
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
  top_k: number
  no_lens: boolean
  documents: DocumentTraceDoc[]
}

// One chunk's hits, sorted strongest first — the shape every view in this
// tab actually wants, derived here once rather than re-sorted per render.
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
  for (const arr of byChunk.values()) arr.sort((a, b) => b.score - a.score)
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

// Three of the five source texts are Gutenberg-style hard-wrapped at ~70
// columns (federalist-10, common-sense, modest-proposal: every line 63–74
// chars, CRLF). Rendered verbatim those wraps read as ragged, arbitrary
// line breaks. A blank line is the author's paragraph break; a lone line
// break inside a paragraph is the transcriber's, and gets unwrapped to a
// space. A chunk may hold several paragraphs (the 40-word floor-merge folds
// short masthead blocks into the first real paragraph), so this returns a
// list. Display-only: never touches full_text or the char offsets.
export function paragraphsOf(text: string): string[] {
  // Normalise line endings first. Splitting on an alternation like
  // (\r\n|\r|\n){2,} lets the regex engine backtrack a single CRLF into
  // "\r" + "\n" and count it as a blank line — every hard-wrapped line
  // then becomes its own paragraph.
  return text
    .replace(/\r\n?/g, "\n")
    .split(/\n(?:[ \t]*\n)+/)
    .map((p) => p.replace(/[ \t]*\n[ \t]*/g, " ").trim())
    .filter((p) => p.length > 0)
}

// What a hit's score actually rests on, in words a reader can use. The Go
// side records two booleans per hit: whether the semantic lens ran for the
// chunk at all (lens_used — when the embed gate fails to narrow the pool
// the lens is skipped and scoring falls back to surface-token overlap
// across the whole catalog, see pkg/lexicon/read.go), and whether the atom
// had a surface-token match. A "keyword" hit at the 1.60 ceiling is the
// weakest kind of evidence this tab shows and should look like it.
export type MatchKind = "keyword" | "semantic" | "both"

export function matchKind(hit: DocumentTraceHit, chunk: DocumentTraceChunk): MatchKind {
  if (!chunk.lens_used) return "keyword"
  return hit.lexical_match ? "both" : "semantic"
}

export const MATCH_KIND_LABEL: Record<MatchKind, string> = {
  keyword: "keyword match only",
  semantic: "semantic match",
  both: "semantic + keyword",
}

// Highest score the pipeline emits (lexical boost saturates here); used to
// draw scores as a proportional bar rather than a bare decimal.
export const SCORE_CEILING = 1.6

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

export interface TraceGraphNode {
  id: string
  name: string
  tier: string
  hitCount: number
}

export interface TraceGraphLink {
  source: string
  target: string
  weight: number
  // "transition": this document's TOP hit in one chunk was followed by a
  // different atom as the top hit of the next chunk — directed, read as
  // "tends to lead to." "co-occurrence": two atoms both fired within the
  // same chunk — undirected, read as "shows up alongside."
  kind: "transition" | "co-occurrence"
}

export interface TraceGraphData {
  nodes: TraceGraphNode[]
  links: TraceGraphLink[]
}

// Derives a small per-document network from the same flat `hits` list the
// linear-strip view uses — no backend change needed for this view, by
// design (see project_document_pattern_trace_feature_idea.md). Transition
// edges come from consecutive chunks' TOP hits; co-occurrence edges from
// hits sharing one chunk. Self-loops (an atom transitioning to or
// co-occurring with itself) are dropped — they're not informative here.
export function traceGraph(doc: DocumentTraceDoc): TraceGraphData {
  const chunks = chunksWithHits(doc)
  const nodeMeta = new Map<string, { name: string; tier: string }>()
  const hitCount = new Map<string, number>()
  for (const h of doc.hits) {
    nodeMeta.set(h.atom_id, { name: h.name, tier: h.tier })
    hitCount.set(h.atom_id, (hitCount.get(h.atom_id) ?? 0) + 1)
  }

  const edgeWeight = new Map<string, { source: string; target: string; kind: TraceGraphLink["kind"]; weight: number }>()
  const bump = (a: string, b: string, kind: TraceGraphLink["kind"]) => {
    if (a === b) return
    const key = kind === "co-occurrence" ? [a, b].sort().join("|") + "|co" : `${a}|${b}|tr`
    const existing = edgeWeight.get(key)
    if (existing) {
      existing.weight += 1
    } else {
      edgeWeight.set(key, { source: a, target: b, kind, weight: 1 })
    }
  }

  let prevTop: string | undefined
  for (const c of chunks) {
    if (c.hits.length === 0) continue
    const [top, ...rest] = c.hits
    if (prevTop) bump(prevTop, top.atom_id, "transition")
    prevTop = top.atom_id
    for (const other of rest) bump(top.atom_id, other.atom_id, "co-occurrence")
  }

  const nodes: TraceGraphNode[] = [...nodeMeta.entries()].map(([id, meta]) => ({
    id,
    name: meta.name,
    tier: meta.tier,
    hitCount: hitCount.get(id) ?? 1,
  }))
  const links: TraceGraphLink[] = [...edgeWeight.values()].map((e) => ({
    source: e.source,
    target: e.target,
    weight: e.weight,
    kind: e.kind,
  }))
  return { nodes, links }
}

export interface LocalTraceGraph extends TraceGraphData {
  // Atom ids that fired in the selected passage itself, as opposed to its
  // neighbours — drawn emphasised so the reader can tell which is which.
  focus: Set<string>
}

// The neighbourhood of one passage: its own atoms plus the previous and
// next passages' atoms, with the same transition/co-occurrence edges the
// whole-document graph uses but restricted to that window. Obsidian's
// local-graph pane is the model — a whole-document force graph of 114
// nodes (Common Sense) is a hairball, while 4–6 nodes leave room to draw
// every label in full. hitCount stays document-wide so node size still
// says "how often this fires in the whole text".
export function localTraceGraph(doc: DocumentTraceDoc, chunks: ChunkWithHits[], activeIndex: number): LocalTraceGraph {
  const hitCount = new Map<string, number>()
  for (const h of doc.hits) hitCount.set(h.atom_id, (hitCount.get(h.atom_id) ?? 0) + 1)

  const window = chunks.filter((c) => Math.abs(c.index - activeIndex) <= 1 && c.hits.length > 0)
  const nodeMeta = new Map<string, { name: string; tier: string }>()
  const links = new Map<string, TraceGraphLink>()
  const bump = (a: string, b: string, kind: TraceGraphLink["kind"]) => {
    if (a === b) return
    const key = kind === "co-occurrence" ? [a, b].sort().join("|") + "|co" : `${a}|${b}|tr`
    const existing = links.get(key)
    if (existing) existing.weight += 1
    else links.set(key, { source: a, target: b, weight: 1, kind })
  }

  let prevTop: string | undefined
  for (const c of window) {
    for (const h of c.hits) nodeMeta.set(h.atom_id, { name: h.name, tier: h.tier })
    const [top, ...rest] = c.hits
    if (prevTop) bump(prevTop, top.atom_id, "transition")
    prevTop = top.atom_id
    for (const other of rest) bump(top.atom_id, other.atom_id, "co-occurrence")
  }

  const active = chunks.find((c) => c.index === activeIndex)
  return {
    nodes: [...nodeMeta.entries()].map(([id, meta]) => ({ id, name: meta.name, tier: meta.tier, hitCount: hitCount.get(id) ?? 1 })),
    links: [...links.values()],
    focus: new Set(active?.hits.map((h) => h.atom_id) ?? []),
  }
}
