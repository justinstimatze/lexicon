import { useEffect, useMemo, useRef, useState } from "react"
import ForceGraph2D, { type ForceGraphMethods, type NodeObject, type LinkObject } from "react-force-graph-2d"
import type { DocumentTraceDoc, TraceGraphLink, TraceGraphNode } from "@/lib/documentTrace"
import { TIER_COLOR, traceGraph } from "@/lib/documentTrace"

const GRAPH_HEIGHT = 640
// How many of a document's most-frequent atoms get an always-on canvas
// label. A fixed hitCount threshold doesn't work here — checked the real
// distribution and Gettysburg Address has zero atoms with hitCount >= 2 (it
// fires each of its 20 distinct atoms exactly once), so a threshold would
// leave that document's graph completely unlabeled while Common Sense (114
// distinct atoms) would still show 43 always-on labels. Top-K adapts to
// each document's own shape; the rest stay reachable via the hover tooltip.
const MAX_LABELED_NODES = 12

type FGNode = NodeObject<TraceGraphNode>
type FGLink = LinkObject<TraceGraphNode, TraceGraphLink>

const TIER_LEGEND: { tier: string; label: string }[] = [
  { tier: "atomic", label: "atomic" },
  { tier: "molecule", label: "molecule" },
  { tier: "reaction", label: "reaction" },
]

// A short "hybrid" read of the flow-diagram + co-occurrence-graph spec
// from the original feature idea: one force-directed graph rather than
// two separate diagrams, since the two signals share the same node set
// and no off-the-shelf single library draws both natively. Directed
// arrows carry the flow signal (which pattern's top hit tends to lead to
// which); undirected dashed edges carry co-occurrence (patterns sharing a
// chunk, possible only when top-k > 1 finds a second hit worth keeping).
export function TraceNetwork({ doc, onAtomClick }: { doc: DocumentTraceDoc; onAtomClick: (id: string) => void }) {
  const fgRef = useRef<ForceGraphMethods<FGNode, FGLink> | undefined>(undefined)
  const containerRef = useRef<HTMLDivElement>(null)
  const graph = useMemo(() => traceGraph(doc), [doc])

  const labeledNodeIds = useMemo(() => {
    const sorted = [...graph.nodes].sort((a, b) => b.hitCount - a.hitCount)
    return new Set(sorted.slice(0, MAX_LABELED_NODES).map((n) => n.id))
  }, [graph])

  // ForceGraph2D defaults width/height to window.innerWidth/innerHeight
  // when not given explicitly — it does not measure its own parent div.
  // Left unset, zoomToFit fits the graph to the whole viewport, and only
  // the top GRAPH_HEIGHT px of that are visible through this container's
  // overflow-hidden, clipping everything below. Measuring the container
  // directly and passing real dimensions is the fix.
  const [dims, setDims] = useState({ width: 800, height: GRAPH_HEIGHT })
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect
      setDims({ width, height })
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  // zoomToFit belongs only on the FIRST settle after a document loads, not
  // on every settle — dragging a node reheats the simulation, and without
  // this guard the view re-centers itself the moment it resettles,
  // silently discarding whatever pan/zoom the user just set by hand.
  const didInitialZoomRef = useRef(false)
  useEffect(() => {
    didInitialZoomRef.current = false
  }, [doc])

  // Loosen the default force layout for more breathing room between nodes
  // — the default spacing produces an illegible label-and-circle pileup
  // once a document has more than a handful of distinct atoms. Must
  // reheat after touching force params, or the change silently does
  // nothing once cooldownTicks has already exhausted the simulation.
  useEffect(() => {
    const fg = fgRef.current
    if (!fg) return
    fg.d3Force("charge")?.strength(-180)
    fg.d3Force("link")?.distance(70)
    fg.d3ReheatSimulation()
  }, [doc])

  if (graph.nodes.length === 0) {
    return <p className="font-mono text-[11px] text-ink-faint">no pattern hits to graph for this document</p>
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-4 font-mono text-[10px] text-ink-faint">
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-0.5 w-4 bg-ink-dim" style={{ clipPath: "polygon(0 40%, 70% 40%, 70% 20%, 100% 50%, 70% 80%, 70% 60%, 0 60%)" }} />
          transition — this pattern's chunk led to that one
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-0.5 w-4 border-t border-dashed border-ink-faint" />
          co-occurrence — both fired in the same chunk
        </span>
        <span>node size = how often it fired</span>
        <span className="flex items-center gap-3">
          {TIER_LEGEND.map((t) => (
            <span key={t.tier} className="flex items-center gap-1">
              <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: TIER_COLOR[t.tier] }} />
              {t.label}
            </span>
          ))}
        </span>
      </div>
      <div ref={containerRef} className="w-full overflow-hidden border border-rule bg-bg-well" style={{ height: GRAPH_HEIGHT }}>
        <ForceGraph2D<TraceGraphNode, TraceGraphLink>
          ref={fgRef}
          width={dims.width}
          height={dims.height}
          graphData={graph}
          nodeId="id"
          nodeLabel={(n) => `${(n as FGNode).name} · ${(n as FGNode).hitCount} hit${(n as FGNode).hitCount === 1 ? "" : "s"}`}
          nodeVal={(n) => 2 + Math.sqrt((n as FGNode).hitCount) * 2.5}
          nodeColor={(n) => TIER_COLOR[(n as FGNode).tier] ?? TIER_COLOR.atomic}
          nodeCanvasObjectMode={() => "after"}
          nodeCanvasObject={(n, ctx, globalScale) => {
            const node = n as FGNode
            if (!labeledNodeIds.has(node.id)) return
            const label = node.name.split("-").slice(0, 3).join(" ")
            const fontSize = Math.max(9, 11 / globalScale)
            ctx.font = `${fontSize}px "IBM Plex Mono", monospace`
            ctx.textAlign = "center"
            ctx.textBaseline = "top"
            ctx.fillStyle = "rgba(230, 225, 212, 0.85)"
            ctx.fillText(label, node.x ?? 0, (node.y ?? 0) + 8)
          }}
          linkColor={(l) => ((l as FGLink).kind === "transition" ? "rgba(230, 225, 212, 0.55)" : "rgba(230, 225, 212, 0.25)")}
          linkWidth={(l) => 0.6 + Math.sqrt((l as FGLink).weight)}
          linkLineDash={(l) => ((l as FGLink).kind === "co-occurrence" ? [2, 2] : null)}
          linkDirectionalArrowLength={(l) => ((l as FGLink).kind === "transition" ? 5 : 0)}
          linkDirectionalArrowRelPos={1}
          linkCurvature={0.15}
          backgroundColor="rgba(0,0,0,0)"
          onNodeClick={(n) => onAtomClick((n as FGNode).id)}
          cooldownTicks={80}
          onEngineStop={() => {
            if (didInitialZoomRef.current) return
            didInitialZoomRef.current = true
            // getGraphBbox only measures node circles, not the label text
            // drawn in nodeCanvasObject below them — padding has to absorb
            // that extra label height/width itself, hence the generous value.
            fgRef.current?.zoomToFit(400, 60)
          }}
        />
      </div>
    </div>
  )
}
