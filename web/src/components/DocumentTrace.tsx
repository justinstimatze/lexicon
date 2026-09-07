import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useSearchParams } from "react-router-dom"
import graphData from "@/data/graph.json"
import type { LexGraph } from "@/lib/graph"
import documentTraceData from "@/data/document-traces.json"
import type { ChunkWithHits, DocumentTraceData, DocumentTraceDoc } from "@/lib/documentTrace"
import { atomPassages, chunksWithHits, paragraphsOf } from "@/lib/documentTrace"
import { AtomCard } from "@/components/AtomCard"
import { Dialog, DialogContent, DialogBody } from "@/components/ui/dialog"
import { TraceText } from "@/components/TraceText"
import { scrollPassageIntoView } from "@/lib/traceScroll"
import { TracePassagePanel } from "@/components/TracePassagePanel"
import { cn } from "@/lib/utils"

// force-graph's canvas renderer pulls in its own physics engine — kept out
// of the tab's initial bundle the same way Graph3D is split out of the
// app shell.
const TraceNetwork = lazy(() => import("@/components/TraceNetwork").then((m) => ({ default: m.TraceNetwork })))

const graph = graphData as unknown as LexGraph
const nodesById = new Map(graph.nodes.map((n) => [n.id, n]))
const data = documentTraceData as unknown as DocumentTraceData

// Where an atom fires in the open document, as jump links — shown in the
// atom drawer under the card so a reader who arrived from the network
// (no passage selected on the way in) can still get back to the text.
function AtomInDocument({
  doc,
  chunks,
  atomId,
  onSelect,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  atomId: string
  onSelect: (index: number) => void
}) {
  const indices = atomPassages(doc, atomId)
  if (indices.length === 0) return null
  return (
    <div className="mt-4 border-t border-rule pt-4">
      <div className="mb-2 font-mono text-[10px] tracking-wide text-ink-faint uppercase">
        In {doc.title} — {indices.length} passage{indices.length === 1 ? "" : "s"}
      </div>
      <ul className="flex flex-col gap-1.5">
        {indices.map((i) => {
          const c = chunks[i]
          const lead = c ? paragraphsOf(doc.full_text.slice(c.char_start, c.char_end)).join(" ") : ""
          return (
            <li key={i}>
              <button
                type="button"
                onClick={() => onSelect(i)}
                className="flex w-full items-baseline gap-2 text-left font-mono text-[11px] hover:text-accent-soft focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none"
              >
                <span className="shrink-0 text-primary underline decoration-dotted underline-offset-2 tabular-nums">¶ {i + 1}</span>
                <span className="min-w-0 truncate font-serif text-[12px] text-ink-dim">{lead}</span>
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

export function DocumentTrace() {
  // Selection lives in the URL (#/trace?doc=…&p=3): reloading, sharing,
  // and back/forward all keep the reader's place, the way Sefaria's
  // segment links do.
  const [params, setParams] = useSearchParams()
  const doc = data.documents.find((d) => d.id === params.get("doc")) ?? data.documents[0]
  const chunks = useMemo(() => (doc ? chunksWithHits(doc) : []), [doc])
  const activeIndex = useMemo(() => {
    if (chunks.length === 0) return null
    const p = Number(params.get("p"))
    const i = Number.isFinite(p) && p >= 1 ? Math.floor(p) - 1 : 0
    return Math.min(chunks.length - 1, Math.max(0, i))
  }, [params, chunks.length])

  const [openAtomId, setOpenAtomId] = useState<string | null>(null)
  const [hoverAtomId, setHoverAtomId] = useState<string | null>(null)

  const select = useCallback(
    (index: number, scroll = false) => {
      if (!doc) return
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          next.set("doc", doc.id)
          next.set("p", String(index + 1))
          return next
        },
        { replace: true }
      )
      if (scroll) scrollPassageIntoView(index)
    },
    [doc, setParams]
  )

  // A deep link (?p=9) or a document switch should land the reader on the
  // passage it names; a click on a passage already in view should not
  // move the page. Runs once per document, after the rows exist. On the
  // very first mount the page is already at the top, so ¶1 needs no
  // scroll; on a later switch the page may be scrolled deep into the
  // previous document, so even ¶1 does.
  const initialDocRef = useRef<string | null>(null)
  useEffect(() => {
    if (!doc || activeIndex === null) return
    if (initialDocRef.current === doc.id) return
    const firstMount = initialDocRef.current === null
    initialDocRef.current = doc.id
    if (activeIndex > 0 || !firstMount) scrollPassageIntoView(activeIndex)
  }, [doc, activeIndex])

  const pickDoc = (id: string) => {
    setParams({ doc: id, p: "1" })
    setOpenAtomId(null)
    setHoverAtomId(null)
  }

  // ← / → (or j / k) step through passages when nothing else owns the keys.
  useEffect(() => {
    if (activeIndex === null) return
    const onKey = (e: KeyboardEvent) => {
      if (openAtomId || e.metaKey || e.ctrlKey || e.altKey) return
      const t = e.target as HTMLElement | null
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable)) return
      if ((e.key === "ArrowRight" || e.key === "j") && activeIndex < chunks.length - 1) {
        e.preventDefault()
        select(activeIndex + 1, true)
      } else if ((e.key === "ArrowLeft" || e.key === "k") && activeIndex > 0) {
        e.preventDefault()
        select(activeIndex - 1, true)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [activeIndex, chunks.length, openAtomId, select])

  if (!doc) {
    return <p className="font-mono text-xs text-ink-faint">no traced documents in this build</p>
  }

  const lensCount = chunks.filter((c) => c.lens_used).length
  const selectedAtom = openAtomId ? nodesById.get(openAtomId) : undefined
  const highlightAtomId = hoverAtomId ?? openAtomId

  return (
    <div className="mx-auto flex max-w-[1520px] flex-col gap-6 max-lg:pb-[46vh]">
      <div className="max-w-[70ch]">
        <h1 className="font-display text-[clamp(22px,2.6vw,32px)] leading-[1.05] font-black tracking-tight text-balance">
          Reading a document as a sequence of patterns
        </h1>
        <p className="mt-3 text-[14px] text-ink-dim">
          Each document is walked passage by passage against the full corpus — the two strongest patterns per passage, in
          reading order. Precomputed at build time, not a live query. None of these authors is cited anywhere else in the
          corpus.
        </p>
      </div>

      <nav aria-label="documents" className="flex flex-wrap gap-2">
        {data.documents.map((d) => (
          <button
            key={d.id}
            type="button"
            aria-current={d.id === doc.id ? "page" : undefined}
            onClick={() => pickDoc(d.id)}
            className={cn(
              "border px-3 py-1.5 text-left font-mono text-[11px] tracking-wide uppercase transition-colors focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none",
              d.id === doc.id ? "border-primary bg-primary/15 text-foreground" : "border-rule text-ink-dim hover:border-primary/50"
            )}
          >
            {d.title}
          </button>
        ))}
      </nav>

      <div>
        <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 border-b border-rule pb-2">
          <h2 className="font-mono text-[12px] tracking-[0.1em] text-foreground uppercase">
            {doc.title} — {doc.author}, {doc.year}
            {doc.source_url && (
              <>
                {" "}
                <a
                  href={doc.source_url}
                  target="_blank"
                  rel="noreferrer"
                  className="ml-2 font-normal tracking-normal text-primary normal-case underline decoration-dotted underline-offset-2 hover:text-accent-soft"
                >
                  source ↗
                </a>
              </>
            )}
          </h2>
          <span className="font-mono text-[10px] text-ink-faint">
            {chunks.length} passages · {doc.hits.length} hits · semantic lens on{" "}
            {lensCount === chunks.length ? "every passage" : `${lensCount} of ${chunks.length}`}
          </span>
        </div>
        {doc.chunking_note && (
          <p className="mt-2 text-[11px] text-ink-dim italic">
            <span className="not-italic text-ink-faint">Note: </span>
            {doc.chunking_note}
          </p>
        )}

        {/*
          Three coordinated columns at xl (text · selected passage · its
          neighbourhood), two at lg (the right pair stacked and sticky), one
          below that with the passage panel as a bottom sheet so a tap on
          a passage still produces a visible reaction. Selecting in any
          column reflects in the others; Jigsaw and Voyant are the prior
          art for that.
        */}
        <div className="mt-5 grid grid-cols-1 items-start gap-5 lg:grid-cols-[minmax(0,1fr)_320px] xl:grid-cols-[minmax(0,1fr)_320px_360px]">
          <TraceText doc={doc} chunks={chunks} activeIndex={activeIndex} highlightAtomId={highlightAtomId} onSelect={(i) => select(i)} />

          <div className="flex flex-col gap-4 lg:sticky lg:top-4 lg:max-h-[calc(100vh-2rem)] lg:overflow-y-auto xl:contents">
            <TracePassagePanel
              doc={doc}
              chunks={chunks}
              activeIndex={activeIndex}
              onSelect={(i) => select(i, true)}
              onAtomClick={setOpenAtomId}
              onAtomHover={setHoverAtomId}
              className={cn(
                "xl:sticky xl:top-4 xl:max-h-[calc(100vh-2rem)] xl:overflow-y-auto",
                "max-lg:fixed max-lg:inset-x-0 max-lg:bottom-0 max-lg:z-40 max-lg:max-h-[44vh] max-lg:overflow-y-auto max-lg:border-x-0 max-lg:border-b-0 max-lg:shadow-[0_-8px_24px_rgba(0,0,0,0.35)]"
              )}
            />
            <Suspense
              fallback={
                <div className="flex h-40 items-center justify-center border border-rule bg-bg-well font-mono text-xs text-ink-faint">
                  loading network…
                </div>
              }
            >
              <TraceNetwork
                doc={doc}
                chunks={chunks}
                activeIndex={activeIndex}
                onAtomClick={setOpenAtomId}
                onAtomHover={setHoverAtomId}
                className="xl:sticky xl:top-4"
              />
            </Suspense>
          </div>
        </div>
      </div>

      <Dialog open={!!selectedAtom} onOpenChange={(open) => !open && setOpenAtomId(null)}>
        <DialogContent title={selectedAtom ? selectedAtom.name : "Atom detail"} description={selectedAtom?.id}>
          <DialogBody>
            {selectedAtom && (
              <>
                <AtomCard node={selectedAtom} onAtomClick={setOpenAtomId} />
                <AtomInDocument
                  doc={doc}
                  chunks={chunks}
                  atomId={selectedAtom.id}
                  onSelect={(i) => {
                    setOpenAtomId(null)
                    select(i, true)
                  }}
                />
              </>
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  )
}
