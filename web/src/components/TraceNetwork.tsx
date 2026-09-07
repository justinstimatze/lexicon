import { useEffect, useMemo, useRef, useState } from "react"
import ForceGraph2D, { type ForceGraphMethods, type NodeObject, type LinkObject } from "react-force-graph-2d"
import { forceCollide } from "d3-force-3d"
import { cn } from "@/lib/utils"
import type { ChunkWithHits, DocumentTraceDoc, TraceGraphLink, TraceGraphNode } from "@/lib/documentTrace"
import { TIER_COLOR, localTraceGraph, traceGraph, wrapName } from "@/lib/documentTrace"

type FGNode = NodeObject<TraceGraphNode>
type FGLink = LinkObject<TraceGraphNode, TraceGraphLink>
type Mode = "local" | "document"

// nodeRelSize 6, not force-graph's default 4: most atoms fire once, so
// most nodes sit at the size floor, and at the default they were too small
// to click reliably once the view was zoomed out to fit.
const NODE_REL_SIZE = 6
// Whole-document mode can't label everything (Common Sense: 114 nodes);
// the top-K by hit count get a label, the rest have the hover tooltip.
const MAX_LABELED_NODES = 3
const LABEL_FONT_PX = 11
const LABEL_LINE_HEIGHT = 1.2
// Local mode: label font in graph units (scales with the view, so the
// collision force can account for it) and the collision radius that
// encloses a node plus its three-line label.
const LOCAL_LABEL_FONT = 12
const LOCAL_LABEL_RADIUS = 68

function nodeVal(n: FGNode): number {
  return 4 + Math.sqrt(n.hitCount) * 2.5
}

function nodeRadius(n: FGNode): number {
  return Math.sqrt(nodeVal(n)) * NODE_REL_SIZE
}

const TIER_LEGEND = ["atomic", "molecule", "reaction"]

// Two views of the same hits. "local" is the default and the one that's
// actually legible: the selected passage's atoms plus its neighbours',
// few enough nodes to draw every name in full (Obsidian's local-graph
// pane is the model). "document" is the whole-text force graph — kept as
// the secondary view because the overall shape is sometimes interesting,
// not because individual nodes can be read off it at 114 nodes.
export function TraceNetwork({
  doc,
  chunks,
  activeIndex,
  onAtomClick,
  onAtomHover,
  className,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number | null
  onAtomClick: (id: string) => void
  onAtomHover: (id: string | null) => void
  className?: string
}) {
  const [mode, setMode] = useState<Mode>("local")
  const fgRef = useRef<ForceGraphMethods<FGNode, FGLink> | undefined>(undefined)
  const containerRef = useRef<HTMLDivElement>(null)

  const local = mode === "local" && activeIndex !== null
  const graph = useMemo(
    () => (local ? localTraceGraph(doc, chunks, activeIndex) : { ...traceGraph(doc), focus: new Set<string>() }),
    [doc, chunks, activeIndex, local]
  )
  // force-graph mutates node objects in place (x, y, vx…); handing it a
  // fresh array per graph identity keeps layouts from bleeding across.
  const graphData = useMemo(() => ({ nodes: graph.nodes.map((n) => ({ ...n })), links: graph.links.map((l) => ({ ...l })) }), [graph])

  // Which atoms fired in the selected passage — ringed in both modes, so
  // the whole-text view still answers "where does this passage sit".
  const focus = useMemo(
    () => new Set(chunks.find((c) => c.index === activeIndex)?.hits.map((h) => h.atom_id) ?? []),
    [chunks, activeIndex]
  )

  const labeled = useMemo(() => {
    if (local) return new Set(graph.nodes.map((n) => n.id))
    // Whole-text mode lives in a ~360px column: at 20+ nodes even a dozen
    // screen-pixel labels collide. Label the selected passage's atoms and
    // the few most frequent; the rest keep the hover tooltip.
    const sorted = [...graph.nodes].sort((a, b) => b.hitCount - a.hitCount)
    return new Set([...focus, ...sorted.slice(0, MAX_LABELED_NODES).map((n) => n.id)])
  }, [graph, local, focus])

  const height = local ? 400 : 560

  // ForceGraph2D sizes itself to the window unless told otherwise; measure
  // the container so zoomToFit fits what's actually visible.
  const [dims, setDims] = useState({ width: 320, height })
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver((entries) => {
      const { width } = entries[0].contentRect
      setDims({ width, height })
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [height])

  // zoomToFit only on the first settle after the graph changes identity —
  // never on the settle that follows a drag, which would throw away the
  // reader's own pan/zoom.
  const didInitialZoomRef = useRef(false)
  useEffect(() => {
    didInitialZoomRef.current = false
  }, [graphData])

  // Real collision force so nodes (and the labels under them) can't
  // overlap whatever the random initial layout was; charge/link tuning
  // alone only makes overlap less likely. Reheat after touching forces or
  // the change no-ops once cooldownTicks has run out.
  useEffect(() => {
    const fg = fgRef.current
    if (!fg) return
    fg.d3Force("charge")?.strength(local ? -380 : -220)
    fg.d3Force("link")?.distance(local ? 115 : 80)
    // Local mode draws labels in graph units (see nodeCanvasObject), so a
    // collision radius in the same units can genuinely cover the label
    // box: three lines of ~22 mono chars is ~140 wide, half of that is 70.
    // Document mode labels are screen-pixel sized, so the radius there is
    // only a node-spacing hint.
    fg.d3Force("collide", forceCollide<FGNode>((n) => (local ? LOCAL_LABEL_RADIUS : nodeRadius(n) + 22)).strength(1))
    fg.d3ReheatSimulation()
  }, [graphData, local])

  const empty = graph.nodes.length === 0

  return (
    <section aria-label="pattern network" className={cn("border border-rule bg-bg-well", className)}>
      <header className="flex flex-wrap items-center justify-between gap-2 border-b border-rule px-4 py-2">
        <h3 className="font-mono text-[10px] tracking-[0.12em] text-ink-faint uppercase">
          {local ? "Around this passage" : "Whole document"}
        </h3>
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

      <div ref={containerRef} className="w-full overflow-hidden" style={{ height }}>
        {empty ? (
          <p className="p-4 font-mono text-[11px] text-ink-faint">
            {activeIndex === null && mode === "local" ? "Select a passage to see its neighbourhood." : "No pattern hits to graph."}
          </p>
        ) : (
          <ForceGraph2D<TraceGraphNode, TraceGraphLink>
            ref={fgRef}
            width={dims.width}
            height={dims.height}
            graphData={graphData}
            nodeId="id"
            nodeRelSize={NODE_REL_SIZE}
            nodeVal={(n) => nodeVal(n as FGNode)}
            nodeLabel={(n) => `${(n as FGNode).name} · fires ${(n as FGNode).hitCount}×`}
            nodeColor={(n) => {
              const node = n as FGNode
              const c = TIER_COLOR[node.tier] ?? TIER_COLOR.atomic
              return local && !focus.has(node.id) ? `${c}99` : c
            }}
            nodeCanvasObjectMode={() => "after"}
            nodeCanvasObject={(n, ctx, globalScale) => {
              const node = n as FGNode
              const x = node.x ?? 0
              const y = node.y ?? 0
              const r = nodeRadius(node)
              if (focus.has(node.id)) {
                ctx.beginPath()
                ctx.arc(x, y, r + 3, 0, 2 * Math.PI)
                ctx.strokeStyle = "rgba(230, 225, 212, 0.9)"
                ctx.lineWidth = local ? 1.5 : 1.5 / globalScale
                ctx.stroke()
              }
              if (!labeled.has(node.id)) return
              const lines = local ? wrapName(node.name, 19, 3) : wrapName(node.name, 18, 2)
              // Local: graph units, so labels shrink and grow with the view
              // and the collision radius above stays honest. Document: a
              // constant screen size, since those labels are a sparse top-K
              // and legibility at any zoom matters more than overlap there.
              const fontSize = local ? LOCAL_LABEL_FONT : LABEL_FONT_PX / globalScale
              ctx.font = `${fontSize}px "IBM Plex Mono", monospace`
              ctx.textAlign = "center"
              ctx.textBaseline = "top"
              ctx.fillStyle = local && !focus.has(node.id) ? "rgba(230, 225, 212, 0.6)" : "rgba(230, 225, 212, 0.9)"
              let ly = y + r + (local ? 4 : 4 / globalScale)
              for (const line of lines) {
                ctx.fillText(line, x, ly)
                ly += fontSize * LABEL_LINE_HEIGHT
              }
            }}
            linkColor={(l) => ((l as FGLink).kind === "transition" ? "rgba(230, 225, 212, 0.55)" : "rgba(230, 225, 212, 0.25)")}
            linkWidth={(l) => 0.6 + Math.sqrt((l as FGLink).weight)}
            linkLineDash={(l) => ((l as FGLink).kind === "co-occurrence" ? [2, 2] : null)}
            linkDirectionalArrowLength={(l) => ((l as FGLink).kind === "transition" ? 5 : 0)}
            linkDirectionalArrowRelPos={1}
            linkCurvature={0.15}
            backgroundColor="rgba(0,0,0,0)"
            onNodeClick={(n) => onAtomClick((n as FGNode).id)}
            onNodeHover={(n) => onAtomHover(n ? (n as FGNode).id : null)}
            cooldownTicks={local ? 120 : 250}
            onEngineStop={() => {
              if (didInitialZoomRef.current) return
              didInitialZoomRef.current = true
              // getGraphBbox measures node circles only; the padding has to
              // absorb the labels drawn below them.
              fgRef.current?.zoomToFit(300, local ? 56 : 60)
            }}
          />
        )}
      </div>

      <footer className="flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-rule px-4 py-2 font-mono text-[10px] text-ink-faint">
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
          <span className="inline-block h-2 w-2 rounded-full border border-ink" aria-hidden />
          this passage
        </span>
        <span className="flex items-center gap-3">
          {TIER_LEGEND.map((t) => (
            <span key={t} className="flex items-center gap-1">
              <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: TIER_COLOR[t] }} aria-hidden />
              {t}
            </span>
          ))}
        </span>
        <span>size = how often it fires</span>
      </footer>
    </section>
  )
}
