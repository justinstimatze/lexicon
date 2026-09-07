import { useEffect, useState } from "react"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { cn } from "@/lib/utils"
import type { ChunkWithHits, DocumentTraceDoc, DocumentTraceHit, MatchKind } from "@/lib/documentTrace"
import { MATCH_KIND_LABEL, SCORE_CEILING, TIER_COLOR, atomPassages, matchKind } from "@/lib/documentTrace"
import { fetchAtomDetail, type AtomDetail } from "@/lib/atomDetail"

const INSTRUCTION_TRUNCATE_AT = 280

function truncate(text: string, max: number) {
  if (text.length <= max) return text
  return text.slice(0, text.lastIndexOf(" ", max)) + "…"
}

// A keyword-only hit saturates at the 1.60 ceiling — the weakest evidence
// drawing the longest bar. Those bars render hatched and dimmed so length
// reads as "how the pipeline scored it", not "how sure you should be".
function ScoreBar({ score, tier, kind }: { score: number; tier: string; kind: MatchKind }) {
  const color = TIER_COLOR[tier] ?? TIER_COLOR.atomic
  const pct = Math.min(100, Math.round((score / SCORE_CEILING) * 100))
  const weak = kind === "keyword"
  return (
    <div
      className="flex items-center gap-2"
      title={`score ${score.toFixed(2)} of ${SCORE_CEILING.toFixed(1)}${weak ? " — keyword overlap only, no semantic signal" : ""}`}
    >
      <div className="h-[3px] flex-1 overflow-hidden rounded-full bg-ink/10">
        <div
          className="h-full rounded-full"
          style={{
            width: `${pct}%`,
            backgroundColor: weak ? "transparent" : color,
            backgroundImage: weak ? `repeating-linear-gradient(90deg, ${color}99 0 3px, transparent 3px 6px)` : undefined,
          }}
        />
      </div>
      <span className="w-8 shrink-0 text-right font-mono text-[10px] text-ink-faint tabular-nums">{score.toFixed(2)}</span>
    </div>
  )
}

// One pattern that fired on the selected passage. The passage itself is
// not re-quoted here — it's a few hundred pixels to the left and already
// marked as selected; the panel's job is the part the text can't show:
// which pattern, on what kind of evidence, what it claims, and where else
// in this document it fires.
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
  // Keyed by atom id in the parent list, so a different atom is a fresh
  // instance — no reset-in-effect needed.
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

  const kind = matchKind(hit, chunk)
  const elsewhere = atomPassages(doc, hit.atom_id).filter((i) => i !== chunk.index)
  const color = TIER_COLOR[hit.tier] ?? TIER_COLOR.atomic

  return (
    <li
      className="flex flex-col gap-2 border-t border-rule pt-3 first:border-t-0 first:pt-0"
      onMouseEnter={() => onAtomHover(hit.atom_id)}
      onMouseLeave={() => onAtomHover(null)}
    >
      <div className="flex items-center gap-2 font-mono text-[10px] tracking-wide text-ink-faint uppercase">
        <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} aria-hidden />
        <span>{hit.tier}</span>
        <span aria-hidden>·</span>
        <span className={cn(kind === "keyword" && "text-ink-faint/80 normal-case")}>{MATCH_KIND_LABEL[kind]}</span>
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
      <ScoreBar score={hit.score} tier={hit.tier} kind={kind} />
      {detail?.agent_instruction && (
        <p className="font-sans text-[13px] leading-relaxed text-ink-dim">{truncate(detail.agent_instruction, INSTRUCTION_TRUNCATE_AT)}</p>
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
      <div className="p-4">
        {!chunk ? (
          <p className="font-mono text-[11px] text-ink-faint">Select a passage to see which patterns it matched.</p>
        ) : chunk.hits.length === 0 ? (
          <p className="font-mono text-[11px] text-ink-faint">No pattern surfaced above threshold for this passage.</p>
        ) : (
          <>
            <ul className="flex flex-col gap-3">
              {chunk.hits.map((h) => (
                <HitCard key={h.atom_id} doc={doc} hit={h} chunk={chunk} onSelect={onSelect} onAtomClick={onAtomClick} onAtomHover={onAtomHover} />
              ))}
            </ul>
            {!chunk.lens_used && (
              <p className="mt-4 border-t border-rule pt-3 font-sans text-[11px] leading-relaxed text-ink-faint">
                The semantic lens didn't run on this passage, so these are surface-keyword matches across the whole catalog — weaker
                evidence than the semantic hits elsewhere in this document.
              </p>
            )}
          </>
        )}
      </div>
    </section>
  )
}
