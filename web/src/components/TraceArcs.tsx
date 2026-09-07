import { useEffect, useMemo, useRef, useState } from "react"
import type { ChunkWithHits, DocumentTraceDoc } from "@/lib/documentTrace"
import { TIER_COLOR, atomOccurrences } from "@/lib/documentTrace"

// The whole text as an arc diagram (Wattenberg's shape, turned vertical):
// passages run top to bottom on an axis, every firing is a dot beside its
// passage, and an arc joins each firing of a pattern to its next one. No
// force layout, so nothing moves, nothing zooms, and a pattern's
// recurrence is a shape the eye can follow. A click on a dot goes to that
// passage; hovering a dot lights every other firing of the same pattern
// here, in the text column, and in the passage panel.

const AXIS_X = 34
const DOT_GAP = 9
const DOT_R = 3
const TOP = 14
const BOTTOM = 14
const MAX_BULGE = 230

function yFor(index: number, count: number, height: number): number {
  if (count <= 1) return height / 2
  return TOP + (index / (count - 1)) * (height - TOP - BOTTOM)
}

export function TraceArcs({
  doc,
  chunks,
  activeIndex,
  openAtomId,
  highlightAtomId,
  onPickHit,
  onAtomHover,
  height = 600,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number | null
  openAtomId: string | null
  highlightAtomId: string | null
  onPickHit: (chunkIndex: number, atomId: string) => void
  onAtomHover: (id: string | null) => void
  height?: number
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(320)
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver((entries) => setWidth(entries[0].contentRect.width))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const n = chunks.length
  const occurrences = useMemo(() => atomOccurrences(doc), [doc])
  const recurring = occurrences.filter((o) => o.chunks.length > 1)
  const pointed = highlightAtomId ?? openAtomId
  const pointedName = pointed ? occurrences.find((o) => o.atomId === pointed)?.name : null

  // Dot positions: rank within the passage (strongest first) sets x.
  const dotX = (rank: number) => AXIS_X + 12 + rank * DOT_GAP
  const rankOf = useMemo(() => {
    const m = new Map<string, number>()
    for (const c of chunks) c.hits.forEach((h, k) => m.set(`${c.index}:${h.atom_id}`, k))
    return m
  }, [chunks])

  const arcs = useMemo(() => {
    const out: { atomId: string; tier: string; d: string; span: number }[] = []
    for (const o of recurring) {
      for (let k = 0; k + 1 < o.chunks.length; k++) {
        const a = o.chunks[k]
        const b = o.chunks[k + 1]
        const x1 = dotX(rankOf.get(`${a}:${o.atomId}`) ?? 0)
        const x2 = dotX(rankOf.get(`${b}:${o.atomId}`) ?? 0)
        const y1 = yFor(a, n, height)
        const y2 = yFor(b, n, height)
        const bulge = Math.min(MAX_BULGE, Math.max(18, (y2 - y1) * 0.9), Math.max(24, width - AXIS_X - 60))
        const cx = Math.max(x1, x2) + bulge
        out.push({ atomId: o.atomId, tier: o.tier, d: `M ${x1} ${y1} C ${cx} ${y1}, ${cx} ${y2}, ${x2} ${y2}`, span: b - a })
      }
    }
    // Long arcs first so the short ones draw on top and stay legible.
    out.sort((p, q) => q.span - p.span)
    return out
  }, [recurring, rankOf, n, height, width])

  const tickEvery = n > 60 ? 20 : n > 24 ? 10 : n > 12 ? 5 : 1

  return (
    <div ref={containerRef} className="relative w-full">
      <svg width={width} height={height} role="img" aria-label={`${n} passages, ${doc.hits.length} pattern firings`} className="block select-none">
        <line x1={AXIS_X} x2={AXIS_X} y1={TOP} y2={height - BOTTOM} stroke="var(--color-rule)" strokeWidth={1} />
        {activeIndex !== null && (
          <line
            x1={AXIS_X - 6}
            x2={width - 6}
            y1={yFor(activeIndex, n, height)}
            y2={yFor(activeIndex, n, height)}
            stroke="var(--color-primary)"
            strokeOpacity={0.35}
            strokeWidth={1}
          />
        )}
        {chunks.map((c) =>
          c.index % tickEvery === 0 || c.index === n - 1 ? (
            <text
              key={`t${c.index}`}
              x={AXIS_X - 6}
              y={yFor(c.index, n, height)}
              textAnchor="end"
              dominantBaseline="middle"
              className="fill-ink-faint font-mono text-[9px]"
              style={{ fontVariantNumeric: "tabular-nums" }}
            >
              {c.index + 1}
            </text>
          ) : null
        )}
        <g>
          {arcs.map((a, k) => {
            const color = TIER_COLOR[a.tier] ?? TIER_COLOR.atomic
            const on = pointed === a.atomId
            return (
              <path
                key={k}
                d={a.d}
                fill="none"
                stroke={color}
                strokeOpacity={on ? 0.95 : pointed ? 0.07 : 0.28}
                strokeWidth={on ? 1.6 : 1}
              />
            )
          })}
        </g>
        <g>
          {chunks.map((c) =>
            c.hits.map((h, k) => {
              const color = TIER_COLOR[h.tier] ?? TIER_COLOR.atomic
              const here = c.index === activeIndex
              const on = pointed === h.atom_id
              const dim = pointed !== null && !on
              return (
                <circle
                  key={`${c.index}:${h.atom_id}`}
                  cx={dotX(k)}
                  cy={yFor(c.index, n, height)}
                  r={on || here ? DOT_R + 1 : DOT_R}
                  fill={color}
                  fillOpacity={dim ? 0.25 : 0.95}
                  stroke={on ? "#fff" : here ? `rgb(230 225 212)` : "none"}
                  strokeWidth={on ? 1.5 : 1}
                  tabIndex={0}
                  role="button"
                  aria-label={`passage ${c.index + 1}, ${h.name.replace(/-/g, " ")}`}
                  className="cursor-pointer outline-none focus-visible:stroke-primary"
                  onClick={() => onPickHit(c.index, h.atom_id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault()
                      onPickHit(c.index, h.atom_id)
                    }
                  }}
                  onMouseEnter={() => onAtomHover(h.atom_id)}
                  onMouseLeave={() => onAtomHover(null)}
                  onFocus={() => onAtomHover(h.atom_id)}
                  onBlur={() => onAtomHover(null)}
                >
                  <title>{`¶ ${c.index + 1} · ${h.name.replace(/-/g, " ")} · ${h.confidence.toFixed(2)}`}</title>
                </circle>
              )
            })
          )}
        </g>
      </svg>
      <div className="pointer-events-none absolute top-2 right-3 max-w-[60%] text-right font-mono text-[10px] leading-snug text-ink-dim">
        {pointedName ? (
          <>
            <span className="text-foreground">{pointedName.replace(/-/g, " ")}</span>
            <br />
            fires in {occurrences.find((o) => o.atomId === pointed)?.chunks.length ?? 0} passage
            {(occurrences.find((o) => o.atomId === pointed)?.chunks.length ?? 0) === 1 ? "" : "s"}
          </>
        ) : (
          <>
            {recurring.length} of {occurrences.length} patterns recur
            <br />
            hover a dot to follow one
          </>
        )}
      </div>
    </div>
  )
}
