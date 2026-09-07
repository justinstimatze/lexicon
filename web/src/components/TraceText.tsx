import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import type { ChunkWithHits, DocumentTraceDoc, DocumentTraceHit, EvidenceMark } from "@/lib/documentTrace"
import { TIER_COLOR, displayParagraphs, evidenceMarks, segmentParagraph } from "@/lib/documentTrace"
import { scrollPassageIntoView } from "@/lib/traceScroll"

// The reading column of the Trace tab. Modelled on how Sefaria presents a
// text with connections: a passage number in the left gutter, a small dot
// in the right margin where a passage has something attached, no
// background wash anywhere except the one passage currently selected.
// Inside the selected passage (and any passage a pointed-at atom fires
// in) the words the lens quoted as evidence are marked — the highlight
// is the span that carries the match, not the whole paragraph.

interface Passage {
  key: string
  // null for inert text between chunks (rare — the paragraph splitter
  // covers everything except separators, but a stray gap shouldn't vanish)
  index: number | null
  raw: string
  chunk: ChunkWithHits | null
}

function buildPassages(doc: DocumentTraceDoc, chunks: ChunkWithHits[]): Passage[] {
  const out: Passage[] = []
  let cursor = 0
  const pushGap = (key: string, raw: string) => {
    if (raw.trim().length > 0) out.push({ key, index: null, raw, chunk: null })
  }
  for (const c of chunks) {
    if (c.char_start > cursor) pushGap(`gap-${c.index}`, doc.full_text.slice(cursor, c.char_start))
    out.push({ key: `p-${c.index}`, index: c.index, raw: doc.full_text.slice(c.char_start, c.char_end), chunk: c })
    cursor = c.char_end
  }
  if (cursor < doc.full_text.length) pushGap("gap-end", doc.full_text.slice(cursor))
  return out
}

type DotState = "hit" | "none" | "untraced"

function MarginDot({ tier, state, filled, emphasised }: { tier: string; state: DotState; filled: boolean; emphasised: boolean }) {
  if (state === "none") return null
  if (state === "untraced") {
    return <span aria-hidden title="not traced" className="block h-2 w-2 rounded-full border border-dashed border-ink-faint/70" />
  }
  const color = TIER_COLOR[tier] ?? TIER_COLOR.atomic
  return (
    <span
      aria-hidden
      className="block h-2 w-2 rounded-full transition-[background-color,box-shadow] duration-150"
      style={{
        backgroundColor: filled ? color : "transparent",
        boxShadow: emphasised ? `0 0 0 1.5px ${color}, 0 0 0 4px ${color}33` : `inset 0 0 0 1.5px ${color}`,
      }}
    />
  )
}

interface MinimapTick {
  index: number
  top: number
  height: number
  tier: string | null
}

// A bucket bar in the Hypothes.is sense: the whole document compressed
// into a thin strip, one tick per passage with a hit, with the current
// viewport drawn over it. Common Sense is 169 passages; without this
// there's no way to see where you are or where the rest is.
function TraceMinimap({
  articleRef,
  passages,
  activeIndex,
  highlightSet,
  onSelect,
}: {
  articleRef: React.RefObject<HTMLElement | null>
  passages: Passage[]
  activeIndex: number | null
  highlightSet: Set<number>
  onSelect: (index: number) => void
}) {
  const [ticks, setTicks] = useState<MinimapTick[]>([])
  const [viewport, setViewport] = useState({ top: 0, height: 1 })

  useLayoutEffect(() => {
    const article = articleRef.current
    if (!article) return
    const measure = () => {
      const total = article.offsetHeight || 1
      const next: MinimapTick[] = []
      for (const p of passages) {
        if (p.index === null || !p.chunk) continue
        const el = article.querySelector<HTMLElement>(`[data-passage="${p.index}"]`)
        if (!el) continue
        next.push({ index: p.index, top: el.offsetTop / total, height: el.offsetHeight / total, tier: p.chunk.hits[0]?.tier ?? null })
      }
      setTicks(next)
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(article)
    return () => ro.disconnect()
  }, [articleRef, passages])

  useEffect(() => {
    const article = articleRef.current
    if (!article) return
    let raf = 0
    const update = () => {
      raf = 0
      const rect = article.getBoundingClientRect()
      const total = rect.height || 1
      const top = Math.min(1, Math.max(0, -rect.top / total))
      const height = Math.min(1 - top, window.innerHeight / total)
      setViewport({ top, height })
    }
    const onScroll = () => {
      if (!raf) raf = requestAnimationFrame(update)
    }
    update()
    window.addEventListener("scroll", onScroll, { passive: true })
    window.addEventListener("resize", onScroll)
    return () => {
      window.removeEventListener("scroll", onScroll)
      window.removeEventListener("resize", onScroll)
      if (raf) cancelAnimationFrame(raf)
    }
  }, [articleRef, passages])

  return (
    <nav aria-label="passage overview" className="sticky top-4 hidden h-[calc(100vh-2rem)] w-3 shrink-0 sm:block">
      <div className="relative h-full w-full border-l border-rule-light">
        {ticks.map((t) => {
          const on = t.index === activeIndex || highlightSet.has(t.index)
          // A passage with nothing in it is still a place to land, drawn
          // as the faintest tick so the strip still reads as the whole text.
          const color = t.tier ? (TIER_COLOR[t.tier] ?? TIER_COLOR.atomic) : null
          return (
            <button
              key={t.index}
              type="button"
              aria-label={`go to passage ${t.index + 1}`}
              onClick={() => {
                onSelect(t.index)
                scrollPassageIntoView(t.index)
              }}
              className="absolute left-0 w-full cursor-pointer focus-visible:ring-1 focus-visible:ring-primary focus-visible:outline-none"
              style={{
                top: `${t.top * 100}%`,
                height: `max(3px, ${t.height * 100}%)`,
                backgroundColor: color ? (on ? color : `${color}66`) : on ? "var(--color-ink-dim)" : "var(--color-rule)",
                clipPath: "inset(0.5px 0 0.5px 3px)",
              }}
            />
          )
        })}
        <div
          aria-hidden
          className="pointer-events-none absolute -left-px w-[calc(100%+2px)] border border-ink/40 bg-ink/[0.06]"
          style={{ top: `${viewport.top * 100}%`, height: `${viewport.height * 100}%` }}
        />
      </div>
    </nav>
  )
}

export function TraceText({
  doc,
  chunks,
  activeIndex,
  highlightAtomId,
  onSelect,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number | null
  // An atom being pointed at elsewhere (hovered in the neighbourhood graph,
  // or open in the detail column): every passage it fires in gets its dot
  // lit and its evidence marked, so the three columns read as one view.
  highlightAtomId: string | null
  onSelect: (index: number) => void
}) {
  const articleRef = useRef<HTMLElement>(null)
  const passages = useMemo(() => buildPassages(doc, chunks), [doc, chunks])
  const highlightSet = useMemo(() => {
    if (!highlightAtomId) return new Set<number>()
    return new Set(doc.hits.filter((h) => h.atom_id === highlightAtomId).map((h) => h.chunk_index))
  }, [doc, highlightAtomId])

  return (
    <div className="flex items-start gap-3">
      <TraceMinimap articleRef={articleRef} passages={passages} activeIndex={activeIndex} highlightSet={highlightSet} onSelect={onSelect} />
      <article
        ref={articleRef}
        lang="en"
        className="min-w-0 flex-1 font-serif text-[16px] leading-[1.65] text-ink [font-variant-numeric:oldstyle-nums]"
        style={{ fontOpticalSizing: "auto" }}
      >
        {passages.map((p) =>
          p.index === null || !p.chunk ? (
            <div key={p.key} className="grid grid-cols-[1.75rem_minmax(0,1fr)_1.25rem] gap-x-2 py-2 sm:grid-cols-[2.5rem_minmax(0,1fr)_1.5rem]">
              <span />
              <div className="max-w-[62ch] space-y-4 text-ink-dim">
                {displayParagraphs(p.raw).map((para, i) => (
                  <p key={i}>{para.text}</p>
                ))}
              </div>
            </div>
          ) : (
            <PassageRow
              key={p.key}
              passage={p}
              active={p.index === activeIndex}
              emphasised={highlightSet.has(p.index)}
              highlightAtomId={highlightAtomId}
              onSelect={onSelect}
            />
          )
        )}
      </article>
    </div>
  )
}

function Evidence({ hit, children, strong }: { hit: DocumentTraceHit; children: React.ReactNode; strong: boolean }) {
  const color = TIER_COLOR[hit.tier] ?? TIER_COLOR.atomic
  return (
    <mark
      className="rounded-[2px] text-inherit [box-decoration-break:clone]"
      style={{
        backgroundColor: strong ? `${color}40` : `${color}22`,
        boxShadow: `0 1.5px 0 ${color}${strong ? "" : "99"}`,
        padding: "0.05em 0",
      }}
      title={`${hit.name.replace(/-/g, " ")} · confidence ${hit.confidence.toFixed(2)}`}
    >
      {children}
    </mark>
  )
}

function PassageRow({
  passage,
  active,
  emphasised,
  highlightAtomId,
  onSelect,
}: {
  passage: Passage
  active: boolean
  emphasised: boolean
  highlightAtomId: string | null
  onSelect: (index: number) => void
}) {
  const index = passage.index!
  const chunk = passage.chunk!
  const top = chunk.hits[0]
  const state: DotState = !chunk.traced ? "untraced" : top ? "hit" : "none"

  // Evidence is marked only where a reader is looking: the selected passage
  // shows all of its hits' spans; a passage a pointed-at atom fires in
  // shows that atom's span. Everything else renders plain — 169 passages
  // of Common Sense with every span marked would be the wall of noise the
  // per-paragraph tint used to be.
  const marks = useMemo<EvidenceMark[]>(() => {
    if (active) return evidenceMarks(chunk, chunk.hits)
    if (emphasised && highlightAtomId) return evidenceMarks(chunk, chunk.hits.filter((h) => h.atom_id === highlightAtomId))
    return []
  }, [active, emphasised, highlightAtomId, chunk])
  const paragraphs = useMemo(() => displayParagraphs(passage.raw), [passage.raw])

  return (
    <div
      role="button"
      tabIndex={0}
      aria-pressed={active}
      aria-label={`passage ${index + 1}${top ? `, ${top.name.replace(/-/g, " ")}` : chunk.traced ? ", no pattern" : ", not traced"}`}
      data-passage={index}
      onClick={() => onSelect(index)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault()
          onSelect(index)
        }
      }}
      className={cn(
        "-mx-2 grid cursor-pointer grid-cols-[1.75rem_minmax(0,1fr)_1.25rem] gap-x-2 border-l-2 px-2 py-3 transition-colors duration-150 sm:grid-cols-[2.5rem_minmax(0,1fr)_1.5rem]",
        "focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-inset focus-visible:outline-none",
        active ? "border-primary bg-primary/[0.05]" : "border-transparent hover:bg-ink/[0.035]"
      )}
    >
      <span
        className={cn(
          "pt-[0.3em] pr-1 text-right font-mono text-[10px] tabular-nums select-none [font-variant-numeric:tabular-nums]",
          state === "hit" ? "text-ink-faint" : "text-ink-faint/60"
        )}
      >
        {index + 1}
      </span>
      <div className={cn("max-w-[62ch] space-y-3", state !== "hit" && !active && "text-ink/85")}>
        {paragraphs.map((para, i) => (
          <p key={i}>
            {marks.length === 0
              ? para.text
              : segmentParagraph(para, marks).map((s, k) =>
                  s.hit ? (
                    <Evidence key={k} hit={s.hit} strong={emphasised ? s.hit.atom_id === highlightAtomId : s.hit === top}>
                      {s.text}
                    </Evidence>
                  ) : (
                    <span key={k}>{s.text}</span>
                  )
                )}
          </p>
        ))}
      </div>
      <span className="flex justify-center pt-[0.5em]">
        <MarginDot tier={top?.tier ?? "atomic"} state={state} filled={active || emphasised} emphasised={emphasised} />
      </span>
    </div>
  )
}
