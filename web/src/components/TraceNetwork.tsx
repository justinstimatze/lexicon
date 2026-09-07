import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"
import ForceGraph2D, { type ForceGraphMethods, type NodeObject, type LinkObject } from "react-force-graph-2d"
import type { DocumentTraceDoc, TraceGraphLink, TraceGraphNode } from "@/lib/documentTrace"
import { traceGraph } from "@/lib/documentTrace"

const GRAPH_HEIGHT = 560

type FGNode = NodeObject<TraceGraphNode>
type FGLink = LinkObject<TraceGraphNode, TraceGraphLink>

const TIER_COLOR: Record<string, string> = {
  atomic: "#8a7a5c",
  molecule: "#c98a4b",
  reaction: "#7fb0d6",
}

// A short "hybrid" read of the flow-diagram + co-occurrence-graph spec
// from the original feature idea: one force-directed graph rather than
// two separate diagrams, since the two signals share the same node set
// and no off-the-shelf single library draws both natively. Directed
// arrows carry the flow signal (which pattern's top hit tends to lead to
// which); undirected dashed edges carry co-occurrence (patterns sharing a
// chunk, possible only when top-k > 1 finds a second hit worth keeping).
export function TraceNetwork({ doc }: { doc: DocumentTraceDoc }) {
  const navigate = useNavigate()
  const fgRef = useRef<ForceGraphMethods<FGNode, FGLink> | undefined>(undefined)
  const containerRef = useRef<HTMLDivElement>(null)
  const graph = useMemo(() => traceGraph(doc), [doc])

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

  const handleClick = useCallback(
    (n: FGNode) => {
      navigate(`/list/${n.id}`)
    },
    [navigate]
  )

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
          onNodeClick={(n) => handleClick(n as FGNode)}
          cooldownTicks={80}
          // getGraphBbox only measures node circles, not the label text
          // drawn in nodeCanvasObject below them — padding has to absorb
          // that extra label height/width itself, hence the generous value.
          onEngineStop={() => fgRef.current?.zoomToFit(400, 60)}
        />
      </div>
    </div>
  )
}
