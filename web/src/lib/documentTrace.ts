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
