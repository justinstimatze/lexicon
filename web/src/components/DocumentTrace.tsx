import { lazy, Suspense, useState } from "react"
import { Link } from "react-router-dom"
import documentTraceData from "@/data/document-traces.json"
import type { ChunkWithHits, DocumentTraceData } from "@/lib/documentTrace"
import { chunksWithHits } from "@/lib/documentTrace"

// force-graph's canvas renderer pulls in its own physics engine — worth
// splitting out of the tab's initial bundle the same way Graph3D is split
// out of the app shell, even though this one is far lighter (Canvas2D,
// no three.js).
const TraceNetwork = lazy(() => import("@/components/TraceNetwork").then((m) => ({ default: m.TraceNetwork })))

const data = documentTraceData as unknown as DocumentTraceData

type ViewMode = "strip" | "network"

// One color band per rank within a chunk — the top hit reads strongest,
// a second hit (if any) reads as a lighter echo. Chunks with zero hits
// (nothing scored above -min-score, or a short merged fragment) render as
// a plain unlabeled gap rather than disappearing, so the strip's own
// length still reads as "this much of the document," gaps and all.
const RANK_CLASS = [
  "border-primary/60 bg-primary/15 text-foreground",
  "border-rule text-ink-dim",
]

function ChunkChip({ chunk, isOpen, onToggle }: { chunk: ChunkWithHits; isOpen: boolean; onToggle: () => void }) {
  const top = chunk.hits[0]
  return (
    <button
      type="button"
      onClick={onToggle}
      title={top ? `${top.name} (${top.score.toFixed(2)})` : "no pattern surfaced above threshold"}
      className={
        "min-w-[64px] shrink-0 border px-2 py-1.5 text-left font-mono text-[10px] tracking-wide uppercase transition-colors " +
        (isOpen
          ? "border-primary bg-primary/25 text-foreground"
          : top
            ? RANK_CLASS[0] + " hover:border-primary"
            : "border-dashed border-rule-light text-ink-faint hover:border-rule")
      }
    >
      {top ? top.name.split("-").slice(0, 3).join(" ") : "—"}
    </button>
  )
}

function ChunkDetail({ chunk }: { chunk: ChunkWithHits }) {
  return (
    <div className="border border-rule bg-bg-well p-4">
      <p className="text-[13px] text-ink-dim italic">"{chunk.excerpt}"</p>
      {chunk.hits.length === 0 ? (
        <p className="mt-3 font-mono text-[11px] text-ink-faint">
          no pattern surfaced above threshold for this passage
          {!chunk.lens_used && " (lexical-only — the semantic lens was unavailable for this chunk)"}
        </p>
      ) : (
        <ul className="mt-3 flex flex-col gap-2">
          {chunk.hits.map((h) => (
            <li key={h.atom_id} className="flex items-center justify-between gap-3 font-mono text-[11px]">
              <Link
                to={`/list/${h.atom_id}`}
                className="min-w-0 truncate text-primary underline decoration-dotted underline-offset-2 hover:text-accent-soft"
              >
                {h.name}
              </Link>
              <span className="shrink-0 text-ink-faint tabular-nums">
                {h.score.toFixed(2)}
                {h.lexical_match && !chunk.lens_used ? " · lexical" : ""}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function DocumentTrace() {
  const [docId, setDocId] = useState(data.documents[0]?.id ?? "")
  const doc = data.documents.find((d) => d.id === docId) ?? data.documents[0]
  const chunks = doc ? chunksWithHits(doc) : []
  const [openIndex, setOpenIndex] = useState<number | null>(chunks.length > 0 ? 0 : null)
  const [view, setView] = useState<ViewMode>("strip")

  function pickDoc(id: string) {
    setDocId(id)
    setOpenIndex(0)
  }

  if (!doc) {
    return <p className="font-mono text-xs text-ink-faint">no traced documents in this build</p>
  }

  const lensCoverage = chunks.length > 0 ? chunks.filter((c) => c.lens_used).length / chunks.length : 0
  const openChunk = openIndex !== null ? chunks[openIndex] : undefined

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
            onClick={() => setView("strip")}
            className={
              "border px-2.5 py-1 font-mono text-[10px] tracking-wide uppercase transition-colors " +
              (view === "strip" ? "border-primary bg-primary/15 text-foreground" : "border-rule text-ink-dim hover:border-primary/50")
            }
          >
            Strip
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

        {view === "strip" ? (
          <>
            <div className="mt-4 flex flex-wrap gap-1.5">
              {chunks.map((c, i) => (
                <ChunkChip key={c.index} chunk={c} isOpen={openIndex === i} onToggle={() => setOpenIndex(openIndex === i ? null : i)} />
              ))}
            </div>

            {openChunk && (
              <div className="mt-3">
                <ChunkDetail chunk={openChunk} />
              </div>
            )}
          </>
        ) : (
          <div className="mt-4">
            <Suspense
              fallback={
                <div className="flex h-40 items-center justify-center border border-rule bg-bg-well font-mono text-xs text-ink-faint">
                  loading network…
                </div>
              }
            >
              <TraceNetwork doc={doc} />
            </Suspense>
          </div>
        )}
      </div>
    </div>
  )
}
