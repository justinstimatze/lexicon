import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useSearchParams } from "react-router-dom"
import { X } from "lucide-react"
import { getGraph } from "@/lib/graphStore"
import type { LexNode } from "@/lib/graph"
import documentTraceData from "@/data/document-traces.json"
import type { ChunkWithHits, DocumentTraceData, DocumentTraceDoc } from "@/lib/documentTrace"
import { TIER_COLOR, atomPassages, chunksWithHits, paragraphsOf } from "@/lib/documentTrace"
import { AtomCard } from "@/components/AtomCard"
import { TraceText } from "@/components/TraceText"
import { scrollPassageIntoView } from "@/lib/traceScroll"
import { TracePassagePanel } from "@/components/TracePassagePanel"
import { TraceGraphPane } from "@/components/TraceGraphPane"
import { cn } from "@/lib/utils"

const graph = getGraph()
const nodesById = new Map(graph.nodes.map((n) => [n.id, n]))
const data = documentTraceData as unknown as DocumentTraceData

// Where a pattern fires in the open document, as jump links — the main
// point of opening a pattern from inside a text, so it sits at the top of
// the pattern panel, above the pattern's own description.
function AtomInDocument({
  doc,
  chunks,
  atomId,
  activeIndex,
  onSelect,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  atomId: string
  activeIndex: number | null
  onSelect: (index: number) => void
}) {
  const indices = atomPassages(doc, atomId)
  if (indices.length === 0) return null
  return (
    <div>
      <div className="mb-2 font-mono text-[10px] tracking-wide text-ink-faint uppercase">
        In {doc.title} — {indices.length} passage{indices.length === 1 ? "" : "s"}
      </div>
      <ul className="flex flex-col gap-1.5">
        {indices.map((i) => {
          const c = chunks[i]
          const hit = c?.hits.find((h) => h.atom_id === atomId)
          const lead = hit?.evidence ?? (c ? paragraphsOf(doc.full_text.slice(c.char_start, c.char_end)).join(" ") : "")
          const here = i === activeIndex
          return (
            <li key={i}>
              <button
                type="button"
                aria-current={here ? "true" : undefined}
                onClick={() => onSelect(i)}
                className={cn(
                  "flex w-full items-baseline gap-2 text-left font-mono text-[11px] hover:text-accent-soft focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none",
                  here && "text-foreground"
                )}
              >
                <span className={cn("shrink-0 underline decoration-dotted underline-offset-2 tabular-nums", here ? "text-foreground" : "text-primary")}>
                  ¶ {i + 1}
                </span>
                <span className={cn("min-w-0 truncate font-serif text-[12px] italic", here ? "text-ink" : "text-ink-dim")}>{lead}</span>
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

// The detail column's second face: a pattern instead of a passage. It
// sits in the same slot as the passage panel — not over the graph, not
// over the text — so the network stays clickable while a pattern is open.
// Order: the name, then where it fires (the reason a reader opened it),
// then the pattern's own description behind an expander.
function TraceAtomPanel({
  node,
  doc,
  chunks,
  activeIndex,
  onClose,
  onAtomClick,
  onSelectPassage,
  className,
}: {
  node: LexNode
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number | null
  onClose: () => void
  onAtomClick: (id: string) => void
  onSelectPassage: (index: number) => void
  className?: string
}) {
  const color = TIER_COLOR[node.tier] ?? TIER_COLOR.atomic
  return (
    <section aria-label="pattern detail" className={cn("border border-rule bg-bg-well", className)}>
      <header className="flex items-center justify-between gap-2 border-b border-rule px-4 py-2">
        <h3 className="flex items-center gap-2 font-mono text-[10px] tracking-[0.12em] text-ink-faint uppercase">
          <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} aria-hidden />
          Pattern · {node.tier}
        </h3>
        <button
          type="button"
          aria-label="back to passage"
          onClick={onClose}
          className="flex h-7 w-7 items-center justify-center rounded-sm text-ink-dim hover:bg-ink/10 hover:text-foreground focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none"
        >
          <X className="size-4" />
        </button>
      </header>
      <div className="flex flex-col gap-4 p-4 text-[12px] leading-relaxed">
        <div>
          <div className="font-mono text-[10px] text-ink-faint">{node.id}</div>
          <div className="font-mono text-[13px] leading-snug font-bold break-words text-foreground">{node.name}</div>
        </div>
        <AtomInDocument doc={doc} chunks={chunks} atomId={node.id} activeIndex={activeIndex} onSelect={onSelectPassage} />
        <details className="border-t border-rule pt-3">
          <summary className="cursor-pointer font-mono text-[10px] tracking-wide text-ink-faint uppercase select-none hover:text-primary">
            about this pattern
          </summary>
          <div className="mt-3">
            <AtomCard node={node} onAtomClick={onAtomClick} compact />
          </div>
        </details>
      </div>
    </section>
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
  // The pattern a reader arrived at the current passage through (a graph
  // node, an arc dot, a "¶ n" link in a pattern panel): its evidence is
  // marked strongly in the text and its card is marked in the panel,
  // until the next selection made some other way.
  const [pickedAtomId, setPickedAtomId] = useState<string | null>(null)

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

  // A click on the text itself: show that passage's hits, nothing pinned.
  const selectPassage = useCallback(
    (index: number, scroll = false) => {
      setOpenAtomId(null)
      setPickedAtomId(null)
      select(index, scroll)
    },
    [select]
  )

  // A click on an occurrence (graph node, arc dot): the same as clicking
  // the passage, with the pattern that was clicked kept in view.
  const pickHit = useCallback(
    (index: number, atomId: string) => {
      setOpenAtomId(null)
      setPickedAtomId(atomId)
      select(index, true)
    },
    [select]
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
    setPickedAtomId(null)
  }

  // ← / → (or j / k) step through passages; Escape closes an open pattern.
  useEffect(() => {
    if (activeIndex === null) return
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      const t = e.target as HTMLElement | null
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable)) return
      if (e.key === "Escape" && openAtomId) {
        e.preventDefault()
        setOpenAtomId(null)
      } else if ((e.key === "ArrowRight" || e.key === "j") && activeIndex < chunks.length - 1) {
        e.preventDefault()
        selectPassage(activeIndex + 1, true)
      } else if ((e.key === "ArrowLeft" || e.key === "k") && activeIndex > 0) {
        e.preventDefault()
        selectPassage(activeIndex - 1, true)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [activeIndex, chunks.length, openAtomId, selectPassage])

  if (!doc) {
    return <p className="font-mono text-xs text-ink-faint">no traced documents in this build</p>
  }

  const tracedCount = chunks.filter((c) => c.traced).length
  const emptyCount = chunks.filter((c) => c.traced && c.hits.length === 0).length
  const selectedAtom = openAtomId ? nodesById.get(openAtomId) : undefined
  const highlightAtomId = hoverAtomId ?? openAtomId ?? pickedAtomId

  const panelClass = cn(
    "xl:sticky xl:top-4 xl:max-h-[calc(100vh-2rem)] xl:overflow-y-auto",
    "max-lg:fixed max-lg:inset-x-0 max-lg:bottom-0 max-lg:z-40 max-lg:max-h-[44vh] max-lg:overflow-y-auto max-lg:border-x-0 max-lg:border-b-0 max-lg:shadow-[0_-8px_24px_rgba(0,0,0,0.35)]"
  )

  return (
    <div className="mx-auto flex max-w-[1520px] flex-col gap-6 max-lg:pb-[46vh]">
      <div className="max-w-[70ch]">
        <h1 className="font-display text-[clamp(22px,2.6vw,32px)] leading-[1.05] font-black tracking-tight text-balance">
          Reading a document as a sequence of patterns
        </h1>
        <p className="mt-3 text-[14px] text-ink-dim">
          Each document is walked passage by passage against the full corpus. A passage carries at most three patterns and often
          none, and every hit points at the words that carry it. Precomputed at build time, not a live query. None of these authors
          is cited anywhere else in the corpus.
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
            {chunks.length} passages · {doc.hits.length} hits · {emptyCount} with none
            {tracedCount < chunks.length && ` · ${chunks.length - tracedCount} not traced`}
          </span>
        </div>
        {doc.chunking_note && (
          <p className="mt-2 text-[11px] text-ink-dim italic">
            <span className="not-italic text-ink-faint">Note: </span>
            {doc.chunking_note}
          </p>
        )}

        {/*
          Three coordinated columns at xl (text · detail · graph), two at
          lg (the right pair stacked and sticky), one below that with the
          detail panel as a bottom sheet so a tap on a passage still
          produces a visible reaction. The detail column shows the selected
          passage or, when a pattern is open, that pattern — never a modal
          over the other two. Selecting in any column reflects in the
          others; Jigsaw and Voyant are the prior art.
        */}
        <div className="mt-5 grid grid-cols-1 items-start gap-5 lg:grid-cols-[minmax(0,1fr)_320px] xl:grid-cols-[minmax(0,1fr)_320px_360px]">
          <TraceText doc={doc} chunks={chunks} activeIndex={activeIndex} highlightAtomId={highlightAtomId} onSelect={(i) => selectPassage(i)} />

          <div className="flex flex-col gap-4 lg:sticky lg:top-4 lg:max-h-[calc(100vh-2rem)] lg:overflow-y-auto xl:contents">
            {selectedAtom ? (
              <TraceAtomPanel
                node={selectedAtom}
                doc={doc}
                chunks={chunks}
                activeIndex={activeIndex}
                onClose={() => setOpenAtomId(null)}
                onAtomClick={setOpenAtomId}
                onSelectPassage={(i) => {
                  // From inside a pattern, "¶ n" is a tour of that pattern's
                  // firings: go there and keep the panel open for the next one.
                  setPickedAtomId(selectedAtom.id)
                  select(i, true)
                }}
                className={panelClass}
              />
            ) : (
              <TracePassagePanel
                doc={doc}
                chunks={chunks}
                activeIndex={activeIndex}
                highlightAtomId={highlightAtomId}
                onSelect={(i) => selectPassage(i, true)}
                onAtomClick={setOpenAtomId}
                onAtomHover={setHoverAtomId}
                className={panelClass}
              />
            )}
            <TraceGraphPane
              doc={doc}
              chunks={chunks}
              activeIndex={activeIndex}
              openAtomId={openAtomId}
              highlightAtomId={highlightAtomId}
              onPickHit={pickHit}
              onAtomHover={setHoverAtomId}
              className="xl:sticky xl:top-4"
            />
          </div>
        </div>
      </div>
    </div>
  )
}
