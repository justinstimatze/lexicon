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
