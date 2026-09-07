import { lazy, Suspense, useState } from "react"
import { cn } from "@/lib/utils"
import type { ChunkWithHits, DocumentTraceDoc } from "@/lib/documentTrace"
import { TIER_COLOR } from "@/lib/documentTrace"
import { TraceArcs } from "@/components/TraceArcs"

// force-graph's canvas renderer pulls in its own physics engine — kept out
// of the tab's initial bundle the same way Graph3D is split out of the
// app shell. The arc view is plain SVG and loads with the tab.
const TraceNetwork = lazy(() => import("@/components/TraceNetwork").then((m) => ({ default: m.TraceNetwork })))

type Mode = "local" | "document"
const TIER_LEGEND = ["atomic", "molecule", "reaction"]

// The right column: one control (this passage / whole text), one view.
// "This passage" is the neighbourhood force graph; "Whole text" is the
// arc diagram along the passage axis. Both are made of occurrences, so a
// click anywhere in either means "take me to that passage".
export function TraceGraphPane({
  doc,
  chunks,
  activeIndex,
  openAtomId,
  highlightAtomId,
  onPickHit,
  onAtomHover,
  className,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number | null
  openAtomId: string | null
  highlightAtomId: string | null
  onPickHit: (chunkIndex: number, atomId: string) => void
  onAtomHover: (id: string | null) => void
  className?: string
}) {
  const [mode, setMode] = useState<Mode>("local")
  const local = mode === "local"

  return (
    <section aria-label="pattern network" className={cn("border border-rule bg-bg-well", className)}>
      <header className="flex items-center justify-between gap-2 border-b border-rule px-4 py-2">
        <h3 className="font-mono text-[10px] tracking-[0.12em] text-ink-faint uppercase">{local ? "Around this passage" : "Whole text"}</h3>
        <div role="group" aria-label="network scope" className="flex gap-1">
          {(["local", "document"] as Mode[]).map((m) => (
            <button
              key={m}
              type="button"
              aria-pressed={mode === m}
              onClick={() => setMode(m)}
              className={cn(
                "border px-2 py-0.5 font-mono text-[10px] tracking-wide uppercase transition-colors focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none",
                mode === m ? "border-primary bg-primary/15 text-foreground" : "border-rule text-ink-dim hover:border-primary/50"
              )}
            >
              {m === "local" ? "This passage" : "Whole text"}
            </button>
          ))}
        </div>
      </header>

      {local ? (
        activeIndex === null ? (
          <p className="p-4 font-mono text-[11px] text-ink-faint">Select a passage to see its neighbourhood.</p>
        ) : (
          <Suspense
            fallback={
              <div className="flex h-[440px] items-center justify-center font-mono text-xs text-ink-faint">loading network…</div>
            }
          >
            <TraceNetwork
              doc={doc}
              chunks={chunks}
              activeIndex={activeIndex}
              openAtomId={openAtomId}
              highlightAtomId={highlightAtomId}
              onPickHit={onPickHit}
              onAtomHover={onAtomHover}
            />
          </Suspense>
        )
      ) : (
        <TraceArcs
          doc={doc}
          chunks={chunks}
          activeIndex={activeIndex}
          openAtomId={openAtomId}
          highlightAtomId={highlightAtomId}
          onPickHit={onPickHit}
          onAtomHover={onAtomHover}
        />
      )}

      <footer className="flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-rule px-4 py-2 font-mono text-[10px] text-ink-faint">
        {local ? (
          <>
            <span className="flex items-center gap-1.5">
              <span
                className="inline-block h-0.5 w-4 bg-ink-dim"
                style={{ clipPath: "polygon(0 40%, 70% 40%, 70% 20%, 100% 50%, 70% 80%, 70% 60%, 0 60%)" }}
                aria-hidden
              />
              led to
            </span>
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-0.5 w-4 border-t border-dashed border-ink-faint" aria-hidden />
              fired together
            </span>
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-0.5 w-4 border-t border-dotted border-ink-faint" aria-hidden />
              same pattern again
            </span>
            <span>size = how often it fires in the text · click a node to go to its passage</span>
          </>
        ) : (
          <span>one dot per firing, strongest nearest the axis · an arc joins a pattern to its next firing · click a dot to go there</span>
        )}
        <span className="flex items-center gap-3">
          {TIER_LEGEND.map((t) => (
            <span key={t} className="flex items-center gap-1">
              <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: TIER_COLOR[t] }} aria-hidden />
              {t}
            </span>
          ))}
        </span>
      </footer>
    </section>
  )
}
