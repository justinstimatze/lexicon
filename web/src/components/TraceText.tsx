import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import type { ChunkWithHits, DocumentTraceDoc } from "@/lib/documentTrace"
import { TIER_COLOR, paragraphsOf } from "@/lib/documentTrace"
import { scrollPassageIntoView } from "@/lib/traceScroll"

// The reading column of the Trace tab. Modelled on how Sefaria presents a
// text with connections: a passage number in the left gutter, a small dot
// in the right margin where a passage has something attached, no
// background highlight anywhere except the one passage currently selected
// (a flat block wash, not per-line pills). Every passage here has hits, so
// highlighting them all would say nothing — the dots carry "this is
// clickable", the wash carries "this is what the panel is showing".

interface Passage {
  key: string
  // null for inert text between chunks (rare — the paragraph splitter
  // covers everything except separators, but a stray gap shouldn't vanish)
  index: number | null
  paragraphs: string[]
  chunk: ChunkWithHits | null
}

function buildPassages(doc: DocumentTraceDoc, chunks: ChunkWithHits[]): Passage[] {
  const out: Passage[] = []
  let cursor = 0
  const pushGap = (key: string, text: string) => {
    const paragraphs = paragraphsOf(text)
    if (paragraphs.length > 0) out.push({ key, index: null, paragraphs, chunk: null })
  }
  for (const c of chunks) {
    if (c.char_start > cursor) pushGap(`gap-${c.index}`, doc.full_text.slice(cursor, c.char_start))
    out.push({
      key: `p-${c.index}`,
      index: c.index,
      paragraphs: paragraphsOf(doc.full_text.slice(c.char_start, c.char_end)),
      chunk: c,
    })
    cursor = c.char_end
  }
  if (cursor < doc.full_text.length) pushGap("gap-end", doc.full_text.slice(cursor))
  return out
}

function MarginDot({ tier, filled, emphasised }: { tier: string; filled: boolean; emphasised: boolean }) {
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
  tier: string
}

// A bucket bar in the Hypothes.is sense: the whole document compressed
// into a thin strip, one tick per passage, with the current viewport drawn
// over it. Common Sense is 169 passages; without this there's no way to
// see where you are or where the rest is.
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
        next.push({ index: p.index, top: el.offsetTop / total, height: el.offsetHeight / total, tier: p.chunk.hits[0]?.tier ?? "atomic" })
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
          const color = TIER_COLOR[t.tier] ?? TIER_COLOR.atomic
          const on = t.index === activeIndex || highlightSet.has(t.index)
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
                backgroundColor: on ? color : `${color}66`,
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
  // or open in the drawer): every passage it fires in gets its dot lit, so
  // the three columns read as one linked view.
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
            <div key={p.key} className="grid grid-cols-[1.75rem_minmax(0,1fr)_1.25rem] sm:grid-cols-[2.5rem_minmax(0,1fr)_1.5rem] gap-x-2 py-2">
              <span />
              <div className="max-w-[62ch] space-y-4 text-ink-dim">
                {p.paragraphs.map((t, i) => (
                  <p key={i}>{t}</p>
                ))}
              </div>
            </div>
          ) : (
            <PassageRow
              key={p.key}
              passage={p}
              active={p.index === activeIndex}
              emphasised={highlightSet.has(p.index)}
              onSelect={onSelect}
            />
          )
        )}
      </article>
    </div>
  )
}

function PassageRow({
  passage,
  active,
  emphasised,
  onSelect,
}: {
  passage: Passage
  active: boolean
  emphasised: boolean
  onSelect: (index: number) => void
}) {
  const index = passage.index!
  const top = passage.chunk!.hits[0]
  return (
    <div
      role="button"
      tabIndex={0}
      aria-pressed={active}
      aria-label={`passage ${index + 1}${top ? `, ${top.name.replace(/-/g, " ")}` : ""}`}
      data-passage={index}
      onClick={() => onSelect(index)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault()
          onSelect(index)
        }
      }}
      className={cn(
        "-mx-2 grid cursor-pointer grid-cols-[1.75rem_minmax(0,1fr)_1.25rem] sm:grid-cols-[2.5rem_minmax(0,1fr)_1.5rem] gap-x-2 border-l-2 px-2 py-3 transition-colors duration-150",
        "focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-inset focus-visible:outline-none",
        active ? "border-primary bg-primary/[0.07]" : "border-transparent hover:bg-ink/[0.035]"
      )}
    >
      <span className="pt-[0.3em] pr-1 text-right font-mono text-[10px] text-ink-faint tabular-nums select-none [font-variant-numeric:tabular-nums]">
        {index + 1}
      </span>
      <div className="max-w-[62ch] space-y-3">
        {passage.paragraphs.map((t, i) => (
          <p key={i}>{t}</p>
        ))}
      </div>
      <span className="flex justify-center pt-[0.5em]">
        {top && <MarginDot tier={top.tier} filled={active || emphasised} emphasised={emphasised} />}
      </span>
    </div>
  )
}
