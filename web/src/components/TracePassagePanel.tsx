import { useEffect, useState } from "react"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { cn } from "@/lib/utils"
import type { ChunkWithHits, DocumentTraceDoc, DocumentTraceHit } from "@/lib/documentTrace"
import { TIER_COLOR, atomPassages } from "@/lib/documentTrace"
import { fetchAtomDetail, type AtomDetail } from "@/lib/atomDetail"

const INSTRUCTION_TRUNCATE_AT = 320

function truncate(text: string, max: number) {
  if (text.length <= max) return text
  return text.slice(0, text.lastIndexOf(" ", max)) + "…"
}

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

// Loaded only when the card's detail is opened.
function HitDetail({ hit, anchored, onAtomClick }: { hit: DocumentTraceHit; anchored: boolean; onAtomClick: (id: string) => void }) {
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
  const color = TIER_COLOR[hit.tier] ?? TIER_COLOR.atomic
  return (
    <div className="mt-2 flex flex-col gap-2">
      {detail?.agent_instruction ? (
        <p className="font-sans text-[12px] leading-relaxed text-ink-dim">{truncate(detail.agent_instruction, INSTRUCTION_TRUNCATE_AT)}</p>
      ) : (
        <p className="font-mono text-[10px] text-ink-faint">loading…</p>
      )}
      {hit.evidence && !anchored && (
        <blockquote
          className="border-l-2 border-dashed pl-2.5 font-serif text-[13px] leading-snug text-ink-dim italic"
          style={{ borderColor: `${color}80` }}
          title="the lens quoted this, but the words could not be found in the passage as written"
        >
          “{hit.evidence}”
        </blockquote>
      )}
      <button
        type="button"
        onClick={() => onAtomClick(hit.atom_id)}
        className="self-start font-mono text-[10px] tracking-wide text-primary uppercase hover:text-accent-soft focus-visible:ring-1 focus-visible:ring-primary focus-visible:outline-none"
      >
        open pattern →
      </button>
    </div>
  )
}

// One pattern the lens found in the selected passage. The words it fired
// on are already marked in the text column, so they aren't repeated here;
// the card carries what the text can't show — which pattern, how sure,
// one sentence on why, and where else in the document it fires. The
// pattern's own description waits behind "detail".
function HitCard({
  doc,
  hit,
  chunk,
  emphasised,
  onSelect,
  onAtomClick,
  onAtomHover,
}: {
  doc: DocumentTraceDoc
  hit: DocumentTraceHit
  chunk: ChunkWithHits
  emphasised: boolean
  onSelect: (index: number) => void
  onAtomClick: (id: string) => void
  onAtomHover: (id: string | null) => void
}) {
  const elsewhere = atomPassages(doc, hit.atom_id).filter((i) => i !== chunk.index)
  const color = TIER_COLOR[hit.tier] ?? TIER_COLOR.atomic
  const anchored = hit.evidence_start !== undefined
  const [open, setOpen] = useState(false)

  return (
    <li
      className={cn(
        "-ml-4 flex flex-col gap-1.5 border-t border-l-2 border-rule py-3 pl-[14px] first:border-t-0 first:pt-0",
        emphasised ? "" : "border-l-transparent"
      )}
      style={emphasised ? { borderLeftColor: color } : undefined}
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
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 font-mono text-[10px] text-ink-faint">
        {elsewhere.length > 0 && (
          <span>
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
          </span>
        )}
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className="text-ink-faint underline decoration-dotted underline-offset-2 hover:text-accent-soft focus-visible:ring-1 focus-visible:ring-primary focus-visible:outline-none"
        >
          {open ? "less" : "detail"}
        </button>
      </div>
      {open && <HitDetail hit={hit} anchored={anchored} onAtomClick={onAtomClick} />}
    </li>
  )
}

export function TracePassagePanel({
  doc,
  chunks,
  activeIndex,
  highlightAtomId,
  onSelect,
  onAtomClick,
  onAtomHover,
  className,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number | null
  highlightAtomId: string | null
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
              {chunk.hits.length > 0 && (
                <span className="ml-2 text-ink-faint/80 normal-case tracking-normal">
                  · {chunk.hits.length} pattern{chunk.hits.length === 1 ? "" : "s"}
                </span>
              )}
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
      <div className="p-4">
        {!chunk ? (
          <p className="font-mono text-[11px] text-ink-faint">Select a passage to see which patterns it carries.</p>
        ) : !chunk.traced ? (
          <p className="font-sans text-[12px] leading-relaxed text-ink-faint">
            Not traced{chunk.trace_note ? `: ${chunk.trace_note}` : ""}. The pipeline could not judge this passage, so it carries no
            patterns rather than guessed ones.
          </p>
        ) : chunk.hits.length === 0 ? (
          <p className="font-sans text-[12px] leading-relaxed text-ink-faint">
            Nothing in the corpus fires here. Most exposition doesn't instantiate a named move, and the trace says so instead of filling
            the slot.
          </p>
        ) : (
          <ul className="flex flex-col">
            {chunk.hits.map((h) => (
              <HitCard
                key={h.atom_id}
                doc={doc}
                hit={h}
                chunk={chunk}
                emphasised={h.atom_id === highlightAtomId}
                onSelect={onSelect}
                onAtomClick={onAtomClick}
                onAtomHover={onAtomHover}
              />
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}
