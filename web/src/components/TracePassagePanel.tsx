import { useState } from "react"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { cn } from "@/lib/utils"
import type { ChunkWithHits, DocumentTraceDoc, DocumentTraceHit } from "@/lib/documentTrace"
import { TIER_COLOR, atomPassages, paragraphsOf } from "@/lib/documentTrace"

// How many unwrapped lines of the passage show before the "more" toggle.
// The whole passage is a few hundred pixels to the left already; the copy
// here exists so the panel can be read on its own (mobile bottom sheet,
// or when the reader's eye is on the hits), not to replace the column.
const PASSAGE_CLAMP_LINES = 10

function Confidence({ value, tier }: { value: number; tier: string }) {
  const color = TIER_COLOR[tier] ?? TIER_COLOR.atomic
  return (
    <span
      className="flex items-center gap-2"
      title="The lens's estimate that this pattern's mechanism is present in the passage, 0 to 1. Nothing below the run's floor is shown."
    >
      <span className="h-[3px] w-14 overflow-hidden rounded-full bg-ink/10">
        <span className="block h-full rounded-full" style={{ width: `${Math.round(value * 100)}%`, backgroundColor: color }} />
      </span>
      <span className="font-mono text-[10px] text-ink-faint tabular-nums">{value.toFixed(2)}</span>
    </span>
  )
}

// One pattern the lens found in the selected passage: which pattern, how
// sure, the sentence saying how this passage does it, the words it
// pointed at, and where else in the document it fires.
function HitCard({
  doc,
  hit,
  chunk,
  onSelect,
  onAtomClick,
  onAtomHover,
}: {
  doc: DocumentTraceDoc
  hit: DocumentTraceHit
  chunk: ChunkWithHits
  onSelect: (index: number) => void
  onAtomClick: (id: string) => void
  onAtomHover: (id: string | null) => void
}) {
  const elsewhere = atomPassages(doc, hit.atom_id).filter((i) => i !== chunk.index)
  const color = TIER_COLOR[hit.tier] ?? TIER_COLOR.atomic
  const anchored = hit.evidence_start !== undefined

  return (
    <li
      className="flex flex-col gap-2 border-t border-rule pt-3 first:border-t-0 first:pt-0"
      onMouseEnter={() => onAtomHover(hit.atom_id)}
      onMouseLeave={() => onAtomHover(null)}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-2 font-mono text-[10px] tracking-wide text-ink-faint uppercase">
          <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} aria-hidden />
          {hit.tier}
        </span>
        <Confidence value={hit.confidence} tier={hit.tier} />
      </div>
      <button
        type="button"
        onClick={() => onAtomClick(hit.atom_id)}
        onFocus={() => onAtomHover(hit.atom_id)}
        onBlur={() => onAtomHover(null)}
        className="text-left font-mono text-[12px] leading-snug break-words text-primary underline decoration-dotted underline-offset-[3px] hover:text-accent-soft focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none"
      >
        {hit.name}
      </button>
      {hit.why && <p className="font-sans text-[13px] leading-relaxed text-ink">{hit.why}</p>}
      {hit.evidence && (
        <blockquote
          className={cn("border-l-2 pl-2.5 font-serif text-[13px] leading-snug text-ink-dim italic", !anchored && "border-dashed")}
          style={{ borderColor: `${color}${anchored ? "" : "80"}` }}
          title={anchored ? "marked in the text" : "the lens quoted this, but the words could not be found in the passage as written"}
        >
          “{hit.evidence}”
        </blockquote>
      )}
      {elsewhere.length > 0 && (
        <p className="font-mono text-[10px] text-ink-faint">
          also in{" "}
          {elsewhere.map((i, k) => (
            <span key={i}>
              {k > 0 && ", "}
              <button
                type="button"
                onClick={() => onSelect(i)}
                className="text-ink-dim underline decoration-dotted underline-offset-2 hover:text-accent-soft focus-visible:ring-1 focus-visible:ring-primary focus-visible:outline-none"
              >
                ¶ {i + 1}
              </button>
            </span>
          ))}
        </p>
      )}
    </li>
  )
}

function PassageText({ doc, chunk }: { doc: DocumentTraceDoc; chunk: ChunkWithHits }) {
  const [expanded, setExpanded] = useState(false)
  const paragraphs = paragraphsOf(doc.full_text.slice(chunk.char_start, chunk.char_end))
  const words = paragraphs.reduce((n, p) => n + p.split(/\s+/).length, 0)
  // ~12 words per unwrapped line at this column width.
  const long = words > PASSAGE_CLAMP_LINES * 12
  return (
    <div className="border-b border-rule pb-3">
      <div
        className={cn("space-y-2 font-serif text-[13.5px] leading-[1.55] text-ink-dim", !expanded && long && "relative max-h-[15.5em] overflow-hidden")}
      >
        {paragraphs.map((p, i) => (
          <p key={i}>{p}</p>
        ))}
        {!expanded && long && <div className="pointer-events-none absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-bg-well to-transparent" />}
      </div>
      {long && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="mt-1.5 font-mono text-[10px] text-primary hover:text-accent-soft focus-visible:ring-1 focus-visible:ring-primary focus-visible:outline-none"
        >
          {expanded ? "less" : `whole passage · ${words} words`}
        </button>
      )}
    </div>
  )
}

export function TracePassagePanel({
  doc,
  chunks,
  activeIndex,
  onSelect,
  onAtomClick,
  onAtomHover,
  className,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number | null
  onSelect: (index: number) => void
  onAtomClick: (id: string) => void
  onAtomHover: (id: string | null) => void
  className?: string
}) {
  const chunk = activeIndex !== null ? chunks[activeIndex] : undefined
  const n = chunks.length
  const hasPrev = activeIndex !== null && activeIndex > 0
  const hasNext = activeIndex !== null && activeIndex < n - 1

  return (
    <section aria-label="selected passage" className={cn("border border-rule bg-bg-well", className)}>
      <header className="flex items-center justify-between gap-2 border-b border-rule px-4 py-2">
        <h3 className="font-mono text-[10px] tracking-[0.12em] text-ink-faint uppercase">
          {chunk ? (
            <>
              Passage <span className="text-foreground tabular-nums">{chunk.index + 1}</span> of <span className="tabular-nums">{n}</span>
            </>
          ) : (
            "No passage selected"
          )}
        </h3>
        <div className="flex items-center gap-1">
          <button
            type="button"
            aria-label="previous passage"
            disabled={!hasPrev}
            onClick={() => hasPrev && onSelect(activeIndex - 1)}
            className="flex h-7 w-7 items-center justify-center rounded-sm text-ink-dim hover:bg-ink/10 hover:text-foreground disabled:opacity-30 disabled:hover:bg-transparent focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none"
          >
            <ChevronLeft className="size-4" />
          </button>
          <button
            type="button"
            aria-label="next passage"
            disabled={!hasNext}
            onClick={() => hasNext && onSelect(activeIndex + 1)}
            className="flex h-7 w-7 items-center justify-center rounded-sm text-ink-dim hover:bg-ink/10 hover:text-foreground disabled:opacity-30 disabled:hover:bg-transparent focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none"
          >
            <ChevronRight className="size-4" />
          </button>
        </div>
      </header>
      <div className="flex flex-col gap-3 p-4">
        {!chunk ? (
          <p className="font-mono text-[11px] text-ink-faint">Select a passage to see which patterns it carries.</p>
        ) : (
          <>
            <PassageText doc={doc} chunk={chunk} />
            {!chunk.traced ? (
              <p className="font-sans text-[12px] leading-relaxed text-ink-faint">
                Not traced{chunk.trace_note ? `: ${chunk.trace_note}` : ""}. The pipeline could not judge this passage, so it carries no
                patterns rather than guessed ones.
              </p>
            ) : chunk.hits.length === 0 ? (
              <p className="font-sans text-[12px] leading-relaxed text-ink-faint">
                Nothing in the corpus fires here. Most exposition doesn't instantiate a named move, and the trace says so instead of
                filling the slot.
              </p>
            ) : (
              <ul className="flex flex-col gap-3">
                {chunk.hits.map((h) => (
                  <HitCard key={h.atom_id} doc={doc} hit={h} chunk={chunk} onSelect={onSelect} onAtomClick={onAtomClick} onAtomHover={onAtomHover} />
                ))}
              </ul>
            )}
          </>
        )}
      </div>
    </section>
  )
}
