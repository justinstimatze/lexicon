import { useEffect, useMemo, useRef, useState } from "react"
import ForceGraph2D, { type ForceGraphMethods, type NodeObject, type LinkObject } from "react-force-graph-2d"
import { forceCollide } from "d3-force-3d"
import type { ChunkWithHits, DocumentTraceDoc, TraceGraphLink, TraceGraphNode } from "@/lib/documentTrace"
import { TIER_COLOR, localTraceGraph, wrapName } from "@/lib/documentTrace"

type FGNode = NodeObject<TraceGraphNode>
type FGLink = LinkObject<TraceGraphNode, TraceGraphLink>

// nodeRelSize 6, not force-graph's default 4: most occurrences are a
// single firing, so most nodes sit at the size floor, and at the default
// they were too small to click reliably once the view was zoomed to fit.
const NODE_REL_SIZE = 6
const LABEL_FONT_PX = 11
const LABEL_LINE_HEIGHT = 1.2
// Node centres are kept this far apart in graph units, and the fit is
// not allowed to zoom out below MIN_ZOOM or in above MAX_ZOOM: a
// neighbourhood with six long names may overflow the canvas and be
// panned rather than be shrunk until nothing is readable, and a single
// node must not be fitted into a circle that fills the box.
const COLLIDE_RADIUS = 70
const MIN_ZOOM = 0.8
const MAX_ZOOM = 1.6
// zoomToFit measures node circles only; the padding (screen px) absorbs
// a three-line label hanging below the lowest node.
const FIT_PADDING = 72
// The canvas stays invisible until the layout has settled and been fitted
// once, so the reader never sees nodes drifting or the view jumping to
// fit them. One fit per distinct graph data, the Graph tab's policy:
// onEngineStop refires for the same data (a drag reheats the simulation)
// and every refire used to reset the view.
const COOLDOWN_TICKS = 160

function nodeVal(n: FGNode): number {
  return 4 + Math.sqrt(n.recurrence) * 2.5
}

function nodeRadius(n: FGNode): number {
  return Math.sqrt(nodeVal(n)) * NODE_REL_SIZE
}

const INK = "230, 225, 212"

// The neighbourhood of the selected passage as a small force graph: its
// hits, plus the top hit of the passage on either side. Every node is
// one firing in one passage — click it and the text column goes there.
export function TraceNetwork({
  doc,
  chunks,
  activeIndex,
  openAtomId,
  highlightAtomId,
  onPickHit,
  onAtomHover,
  height = 440,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number
  openAtomId: string | null
  highlightAtomId: string | null
  onPickHit: (chunkIndex: number, atomId: string) => void
  onAtomHover: (id: string | null) => void
  height?: number
}) {
  const fgRef = useRef<ForceGraphMethods<FGNode, FGLink> | undefined>(undefined)
  const containerRef = useRef<HTMLDivElement>(null)

  const graph = useMemo(() => localTraceGraph(doc, chunks, activeIndex), [doc, chunks, activeIndex])
  // force-graph mutates node objects in place (x, y, vx…); handing it a
  // fresh array per graph identity keeps layouts from bleeding across.
  const graphData = useMemo(() => ({ nodes: graph.nodes.map((n) => ({ ...n })), links: graph.links.map((l) => ({ ...l })) }), [graph])

  // ForceGraph2D sizes itself to the window unless told otherwise; measure
  // the container so zoomToFit fits what's actually visible.
  const [width, setWidth] = useState(320)
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver((entries) => setWidth(entries[0].contentRect.width))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  // fittedFor is the graphData the view was last fitted to; the canvas is
  // shown only while it matches the current one.
  const [fittedFor, setFittedFor] = useState<typeof graphData | null>(null)
  const ready = fittedFor === graphData
  const fitOnce = () => {
    const fg = fgRef.current
    if (!fg || fittedFor === graphData) return
    fg.zoomToFit(0, FIT_PADDING)
    const k = fg.zoom()
    if (k < MIN_ZOOM) fg.zoom(MIN_ZOOM, 0)
    else if (k > MAX_ZOOM) fg.zoom(MAX_ZOOM, 0)
    setFittedFor(graphData)
  }

  // Real collision force so nodes (and the labels under them) can't sit
  // on top of each other; charge/link tuning alone only makes overlap
  // less likely. Set before the warmup runs for this graphData.
  useEffect(() => {
    const fg = fgRef.current
    if (!fg) return
    fg.d3Force("charge")?.strength(-340)
    fg.d3Force("link")?.distance(125)
    fg.d3Force("collide", forceCollide<FGNode>(() => COLLIDE_RADIUS).strength(1))
    fg.d3ReheatSimulation()
  }, [graphData])

  const linkColor = (l: FGLink) => {
    switch (l.kind) {
      case "transition":
        return `rgba(${INK}, 0.6)`
      case "recurrence":
        return `rgba(${INK}, 0.35)`
      default:
        return `rgba(${INK}, 0.25)`
    }
  }

  if (graph.nodes.length === 0) {
    return (
      <div ref={containerRef} className="w-full" style={{ height }}>
        <p className="p-4 font-mono text-[11px] text-ink-faint">No pattern fires in this passage or the ones beside it.</p>
      </div>
    )
  }

  return (
    <div ref={containerRef} className="w-full overflow-hidden" style={{ height, opacity: ready ? 1 : 0, transition: "opacity 180ms ease-out" }}>
      <ForceGraph2D<TraceGraphNode, TraceGraphLink>
        ref={fgRef}
        width={width}
        height={height}
        graphData={graphData}
        nodeId="id"
        nodeRelSize={NODE_REL_SIZE}
        nodeVal={(n) => nodeVal(n as FGNode)}
        nodeLabel={(n) => {
          const node = n as FGNode
          return `¶ ${node.chunkIndex + 1} · ${node.name.replace(/-/g, " ")} · ${node.confidence.toFixed(2)}`
        }}
        nodeColor={(n) => {
          const node = n as FGNode
          const c = TIER_COLOR[node.tier] ?? TIER_COLOR.atomic
          return node.chunkIndex === activeIndex ? c : `${c}99`
        }}
        nodeCanvasObjectMode={() => "after"}
        nodeCanvasObject={(n, ctx, globalScale) => {
          const node = n as FGNode
          const x = node.x ?? 0
          const y = node.y ?? 0
          const r = nodeRadius(node)
          const here = node.chunkIndex === activeIndex
          const pointed = node.atomId === openAtomId || node.atomId === highlightAtomId
          if (here || pointed) {
            ctx.beginPath()
            ctx.arc(x, y, r + 3, 0, 2 * Math.PI)
            ctx.strokeStyle = pointed ? "rgba(255, 255, 255, 0.95)" : `rgba(${INK}, 0.9)`
            ctx.lineWidth = (pointed ? 2.5 : 1.5) / globalScale
            ctx.stroke()
          }
          const fontSize = LABEL_FONT_PX / globalScale
          ctx.font = `${fontSize}px "IBM Plex Mono", monospace`
          ctx.textAlign = "center"
          ctx.textBaseline = "top"
          ctx.fillStyle = here ? `rgba(${INK}, 0.92)` : `rgba(${INK}, 0.62)`
          let ly = y + r + 4 / globalScale
          for (const line of wrapName(node.name, 20, 4)) {
            ctx.fillText(line, x, ly)
            ly += fontSize * LABEL_LINE_HEIGHT
          }
          // Which passage this firing belongs to, above the node — the
          // one fact that distinguishes two nodes of the same pattern.
          ctx.textBaseline = "bottom"
          ctx.fillStyle = `rgba(${INK}, 0.5)`
          ctx.font = `${(LABEL_FONT_PX - 1) / globalScale}px "IBM Plex Mono", monospace`
          ctx.fillText(`¶${node.chunkIndex + 1}`, x, y - r - 3 / globalScale)
        }}
        linkColor={(l) => linkColor(l as FGLink)}
        linkWidth={(l) => ((l as FGLink).kind === "transition" ? 1.6 : 1)}
        linkLineDash={(l) => {
          const k = (l as FGLink).kind
          return k === "co-occurrence" ? [2, 2] : k === "recurrence" ? [1, 3] : null
        }}
        linkDirectionalArrowLength={(l) => ((l as FGLink).kind === "transition" ? 10 : 0)}
        linkDirectionalArrowRelPos={1}
        linkDirectionalArrowColor={(l) => linkColor(l as FGLink)}
        linkCurvature={0.15}
        backgroundColor="rgba(0,0,0,0)"
        onNodeClick={(n) => onPickHit((n as FGNode).chunkIndex, (n as FGNode).atomId)}
        onNodeHover={(n) => onAtomHover(n ? (n as FGNode).atomId : null)}
        cooldownTicks={COOLDOWN_TICKS}
        onEngineStop={fitOnce}
      />
    </div>
  )
}
