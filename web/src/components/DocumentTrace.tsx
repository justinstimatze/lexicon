import { lazy, Suspense, useEffect, useMemo, useState } from "react"
import graphData from "@/data/graph.json"
import type { LexGraph } from "@/lib/graph"
import documentTraceData from "@/data/document-traces.json"
import type { ChunkWithHits, DocumentTraceData, DocumentTraceDoc, DocumentTraceHit } from "@/lib/documentTrace"
import { chunksWithHits, tierTint } from "@/lib/documentTrace"
import { AtomCard } from "@/components/AtomCard"
import { Dialog, DialogContent, DialogBody } from "@/components/ui/dialog"
import { fetchAtomDetail, type AtomDetail } from "@/lib/atomDetail"

// force-graph's canvas renderer pulls in its own physics engine — worth
// splitting out of the tab's initial bundle the same way Graph3D is split
// out of the app shell, even though this one is far lighter (Canvas2D,
// no three.js).
const TraceNetwork = lazy(() => import("@/components/TraceNetwork").then((m) => ({ default: m.TraceNetwork })))

const graph = graphData as unknown as LexGraph
const nodesById = new Map(graph.nodes.map((n) => [n.id, n]))

const data = documentTraceData as unknown as DocumentTraceData

type ViewMode = "text" | "network"

interface TextSegment {
  key: string
  text: string
  chunk: ChunkWithHits | null
}

// Walks full_text once, splitting it at each chunk's (now rune-indexed,
// JS-string-safe) char_start/char_end boundaries. Any gap between chunks
// — normally just the blank-line separator the paragraph splitter
// stripped — renders as plain inert text alongside them.
function buildSegments(doc: DocumentTraceDoc, chunks: ChunkWithHits[]): TextSegment[] {
  const segments: TextSegment[] = []
  let cursor = 0
  for (const c of chunks) {
    if (c.char_start > cursor) {
      segments.push({ key: `gap-${c.index}`, text: doc.full_text.slice(cursor, c.char_start), chunk: null })
    }
    const hasHit = c.hits.length > 0
    segments.push({ key: `chunk-${c.index}`, text: doc.full_text.slice(c.char_start, c.char_end), chunk: hasHit ? c : null })
    cursor = c.char_end
  }
  if (cursor < doc.full_text.length) {
    segments.push({ key: "gap-end", text: doc.full_text.slice(cursor), chunk: null })
  }
  return segments
}

const TRUNCATE_AT = 200

function truncate(text: string, max: number) {
  if (text.length <= max) return text
  return text.slice(0, text.lastIndexOf(" ", max)) + "…"
}

// The source texts carry runs of 3+ line breaks between title/dateline/
// byline blocks (each its own blank-line-separated "paragraph" before the
// floor-merge folds them together) -- rendered verbatim via
// white-space:pre-wrap, that reads as several empty lines stacked up.
// Collapsing all the way to a single line break, not just to one blank
// line, because a highlighted <mark> still draws its own padded box
// decoration around an empty line -- one blank row inside a hit-bearing
// passage still shows as a thin floating highlight sliver with nothing in
// it. Zero blank lines trivially satisfies "no more than one" while
// actually looking clean. Display-only: operates on a segment's own
// extracted text, never on full_text or the char_start/char_end offsets
// used to extract it, so it can't drift the indices other segments
// depend on.
function collapseBlankLines(text: string): string {
  return text.replace(/(\r\n|\r|\n){2,}/g, "\n")
}

// Shows the atom's own general-mechanism explanation next to the quote
// that triggered it — a real "why this matches" note, though an honest
// one: agent_instruction explains the pattern in the abstract, not a
// bespoke justification for this specific passage. Fetched via the same
// per-atom cache AtomCard itself uses, so opening the AtomCard dialog
// right after costs nothing extra.
function HitRow({ hit, lensUsed, onAtomClick }: { hit: DocumentTraceHit; lensUsed: boolean; onAtomClick: (id: string) => void }) {
  const [detail, setDetail] = useState<AtomDetail | null>(null)
  useEffect(() => {
    let cancelled = false
    fetchAtomDetail(hit.atom_id).then((d) => {
      if (!cancelled) setDetail(d)
    })
    return () => {
      cancelled = true
    }
  }, [hit.atom_id])

  return (
    <li className="flex flex-col gap-1 font-mono text-[11px]">
      <div className="flex items-center justify-between gap-3">
        <button
          type="button"
          onClick={() => onAtomClick(hit.atom_id)}
          className="min-w-0 truncate text-left text-primary underline decoration-dotted underline-offset-2 hover:text-accent-soft"
        >
          {hit.name}
        </button>
        <span className="shrink-0 text-ink-faint tabular-nums">
          {hit.score.toFixed(2)}
          {hit.lexical_match && !lensUsed ? " · lexical" : ""}
        </span>
      </div>
      {detail?.agent_instruction && <p className="text-ink-dim italic">{truncate(detail.agent_instruction, TRUNCATE_AT)}</p>}
    </li>
  )
}

function ChunkHitPanel({ doc, chunk, onAtomClick }: { doc: DocumentTraceDoc; chunk: ChunkWithHits; onAtomClick: (id: string) => void }) {
  const fullQuote = collapseBlankLines(doc.full_text.slice(chunk.char_start, chunk.char_end))
  return (
    <div className="border border-rule bg-bg-well p-4">
      <p className="text-[13px] text-ink-dim italic whitespace-pre-wrap">"{truncate(fullQuote, 600)}"</p>
      {chunk.hits.length === 0 ? (
        <p className="mt-3 font-mono text-[11px] text-ink-faint">
          no pattern surfaced above threshold for this passage
          {!chunk.lens_used && " (lexical-only — the semantic lens was unavailable for this chunk)"}
        </p>
      ) : (
        <ul className="mt-3 flex flex-col gap-3">
          {chunk.hits.map((h) => (
            <HitRow key={h.atom_id} hit={h} lensUsed={chunk.lens_used} onAtomClick={onAtomClick} />
          ))}
        </ul>
      )}
    </div>
  )
}

// The same "why" question the Text view's click-through answers (which
// passage produced this atom, and what was its score) still applies when
// the atom was reached from the Network view instead, where there's no
// intermediate chunk click to anchor it -- computed generically from
// doc.hits so both entry points show it, rather than duplicating this
// per entry point.
function AtomDocumentContext({ doc, chunks, atomId }: { doc: DocumentTraceDoc; chunks: ChunkWithHits[]; atomId: string }) {
  const occurrences = doc.hits
    .filter((h) => h.atom_id === atomId)
    .map((h) => {
      const chunk = chunks.find((c) => c.index === h.chunk_index)
      return chunk ? { hit: h, quote: collapseBlankLines(doc.full_text.slice(chunk.char_start, chunk.char_end)) } : null
    })
    .filter((x): x is { hit: DocumentTraceHit; quote: string } => x !== null)

  if (occurrences.length === 0) return null

  return (
    <div className="mb-4 border-b border-rule pb-4">
      <div className="mb-2 font-mono text-[10px] tracking-wide text-ink-faint uppercase">
        in {doc.title} — {occurrences.length} passage{occurrences.length === 1 ? "" : "s"}
      </div>
      <ul className="flex flex-col gap-2.5">
        {occurrences.map((o, i) => (
          <li key={i} className="font-mono text-[11px]">
            <p className="text-ink-dim italic whitespace-pre-wrap">"{truncate(o.quote, 260)}"</p>
            <span className="text-ink-faint tabular-nums">
              {o.hit.score.toFixed(2)}
              {o.hit.lexical_match ? " · lexical" : ""}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

export function DocumentTrace() {
  const [docId, setDocId] = useState(data.documents[0]?.id ?? "")
  const doc = data.documents.find((d) => d.id === docId) ?? data.documents[0]
  const chunks = useMemo(() => (doc ? chunksWithHits(doc) : []), [doc])
  const segments = useMemo(() => (doc ? buildSegments(doc, chunks) : []), [doc, chunks])
  const [openChunkIndex, setOpenChunkIndex] = useState<number | null>(0)
  const [openAtomId, setOpenAtomId] = useState<string | null>(null)
  const [view, setView] = useState<ViewMode>("text")

  function pickDoc(id: string) {
    setDocId(id)
    setOpenChunkIndex(0)
    setOpenAtomId(null)
  }

  function openChunk(index: number) {
    setOpenChunkIndex(index)
  }

  if (!doc) {
    return <p className="font-mono text-xs text-ink-faint">no traced documents in this build</p>
  }

  const lensCoverage = chunks.length > 0 ? chunks.filter((c) => c.lens_used).length / chunks.length : 0
  const activeChunk = openChunkIndex !== null ? chunks[openChunkIndex] : undefined
  const selectedAtom = openAtomId ? nodesById.get(openAtomId) : undefined

  return (
    <div className="mx-auto flex max-w-[1400px] flex-col gap-8">
      <div className="max-w-[70ch]">
        <h1 className="font-display text-[clamp(22px,2.6vw,32px)] leading-[1.05] font-black tracking-tight text-balance">
          Reading a document as a sequence of patterns
        </h1>
        <p className="mt-3 text-[14px] text-ink-dim">
          Each document below is walked paragraph by paragraph, independently, against the full corpus — a single
          top-scoring pattern per paragraph, in the order the paragraph actually occurs. This is a precomputed
          artifact, not a live query: nothing here calls the matching engine from your browser. None of these
          documents are by an author already cited anywhere else in this corpus.
        </p>
      </div>

      <div className="flex flex-wrap gap-2">
        {data.documents.map((d) => (
          <button
            key={d.id}
            type="button"
            onClick={() => pickDoc(d.id)}
            className={
              "border px-3 py-1.5 text-left font-mono text-[11px] tracking-wide uppercase transition-colors " +
              (d.id === docId
                ? "border-primary bg-primary/15 text-foreground"
                : "border-rule text-ink-dim hover:border-primary/50")
            }
          >
            {d.title}
          </button>
        ))}
      </div>

      <div>
        <div className="flex flex-wrap items-baseline justify-between gap-2 border-b border-rule pb-2">
          <h2 className="font-mono text-[12px] tracking-[0.1em] text-foreground uppercase">
            {doc.title} — {doc.author}, {doc.year}
          </h2>
          <span className="font-mono text-[10px] text-ink-faint">
            {chunks.length} passages · {doc.hits.length} pattern hits ·{" "}
            {Math.round(lensCoverage * 100)}% semantic, rest lexical-only
          </span>
        </div>
        {doc.source_url && (
          <a
            href={doc.source_url}
            target="_blank"
            rel="noreferrer"
            className="mt-2 inline-block text-[11px] text-primary underline decoration-dotted underline-offset-2 hover:text-accent-soft"
          >
            read the full text →
          </a>
        )}
        {doc.chunking_note && (
          <p className="mt-2 text-[11px] text-ink-dim italic">
            <span className="not-italic text-ink-faint">Note: </span>
            {doc.chunking_note}
          </p>
        )}

        <div className="mt-4 flex gap-2">
          <button
            type="button"
            onClick={() => setView("text")}
            className={
              "border px-2.5 py-1 font-mono text-[10px] tracking-wide uppercase transition-colors " +
              (view === "text" ? "border-primary bg-primary/15 text-foreground" : "border-rule text-ink-dim hover:border-primary/50")
            }
          >
            Text
          </button>
          <button
            type="button"
            onClick={() => setView("network")}
            className={
              "border px-2.5 py-1 font-mono text-[10px] tracking-wide uppercase transition-colors " +
              (view === "network" ? "border-primary bg-primary/15 text-foreground" : "border-rule text-ink-dim hover:border-primary/50")
            }
          >
            Network
          </button>
        </div>

        {view === "text" ? (
          // A side panel next to the text, not a block below a scrolled
          // container: clicking a highlighted passage has to produce a
          // visible reaction without the reader needing to notice a
          // change happened somewhere off-screen and scroll to find it
          // (genius.com's annotation panel opens right beside the lyric
          // you clicked, for the same reason). Falls back to stacking
          // below the text on narrow viewports, where there's no room
          // for two columns anyway.
          <div className="mt-4 grid grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
            <div className="max-h-[70vh] overflow-y-auto border border-rule bg-bg-well p-5">
              <div className="max-w-[68ch] font-serif text-[15px] leading-relaxed whitespace-pre-wrap text-ink">
                {segments.map((seg) =>
                  seg.chunk ? (
                    <mark
                      key={seg.key}
                      role="button"
                      tabIndex={0}
                      onClick={() => openChunk(seg.chunk!.index)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault()
                          openChunk(seg.chunk!.index)
                        }
                      }}
                      title={`${seg.chunk.hits[0].name} (${seg.chunk.hits[0].score.toFixed(2)})`}
                      style={{
                        backgroundColor: tierTint(seg.chunk.hits[0].tier, openChunkIndex === seg.chunk.index),
                        boxDecorationBreak: "clone",
                        WebkitBoxDecorationBreak: "clone",
                        padding: "0.05em 0.15em",
                        borderRadius: "0.2em",
                      }}
                      className="cursor-pointer text-inherit transition-colors hover:brightness-125 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary"
                    >
                      {collapseBlankLines(seg.text)}
                    </mark>
                  ) : (
                    <span key={seg.key}>{collapseBlankLines(seg.text)}</span>
                  )
                )}
              </div>
            </div>

            <div className="lg:sticky lg:top-4">
              {activeChunk ? (
                <ChunkHitPanel doc={doc} chunk={activeChunk} onAtomClick={setOpenAtomId} />
              ) : (
                <p className="border border-dashed border-rule-light p-4 font-mono text-[11px] text-ink-faint">
                  click a highlighted passage to see which pattern it matched
                </p>
              )}
            </div>
          </div>
        ) : (
          <div className="mt-4">
            <Suspense
              fallback={
                <div className="flex h-40 items-center justify-center border border-rule bg-bg-well font-mono text-xs text-ink-faint">
                  loading network…
                </div>
              }
            >
              <TraceNetwork doc={doc} onAtomClick={setOpenAtomId} />
            </Suspense>
          </div>
        )}
      </div>

      <Dialog open={!!selectedAtom} onOpenChange={(open) => !open && setOpenAtomId(null)}>
        <DialogContent title={selectedAtom ? selectedAtom.name : "Atom detail"} description={selectedAtom?.id}>
          <DialogBody>
            {selectedAtom && (
              <>
                <AtomDocumentContext doc={doc} chunks={chunks} atomId={selectedAtom.id} />
                <AtomCard node={selectedAtom} onAtomClick={setOpenAtomId} />
              </>
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  )
}
