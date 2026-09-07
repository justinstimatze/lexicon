import { useEffect, useMemo, useRef, useState } from "react"
import ForceGraph2D, { type ForceGraphMethods, type NodeObject, type LinkObject } from "react-force-graph-2d"
import { forceCollide } from "d3-force-3d"
import { cn } from "@/lib/utils"
import type { ChunkWithHits, DocumentTraceDoc, TraceGraphData, TraceGraphLink, TraceGraphNode } from "@/lib/documentTrace"
import { TIER_COLOR, localTraceGraph, traceGraph, wrapName } from "@/lib/documentTrace"

type FGNode = NodeObject<TraceGraphNode>
type FGLink = LinkObject<TraceGraphNode, TraceGraphLink>
type Mode = "local" | "document"

// nodeRelSize 6, not force-graph's default 4: most atoms fire once, so
// most nodes sit at the size floor, and at the default they were too small
// to click reliably once the view was zoomed out to fit.
const NODE_REL_SIZE = 6
// Whole-document mode always labels the selected passage's atoms and the
// few most frequent; the rest of the recurring atoms get labels once the
// reader has zoomed in past LABEL_ZOOM (semantic zoom — at the fitted
// scale sixty labels in a 360px column is a smear, at 1.4× they read).
// The rule is stated in the footer; a reader shouldn't have to guess it.
const ALWAYS_LABELED_TOP = 3
const LABEL_ZOOM = 1.35
const LABEL_FONT_PX = 11
const LABEL_LINE_HEIGHT = 1.2
// Labels are a constant screen size in both modes. Local mode keeps
// node centres apart in graph units (LOCAL_COLLIDE) and, after the fit,
// refuses to zoom out below LOCAL_MIN_ZOOM: a neighbourhood with eight
// long names is allowed to overflow the canvas and be panned rather than
// be shrunk until nothing is readable.
const LOCAL_COLLIDE = 70
const LOCAL_MIN_ZOOM = 0.8
// And the other way: a neighbourhood of one node (a passage with a single
// hit and silent neighbours) must not be fitted into a circle that fills
// the canvas.
const LOCAL_MAX_ZOOM = 1.6
// zoomToFit measures node circles only; the padding (screen px) has to
// absorb a three-line label hanging below the lowest node.
const LOCAL_FIT_PADDING = 72
const DOCUMENT_FIT_PADDING = 48
// Above this many nodes, whole-document mode opens on the recurring
// patterns only; Common Sense has over a hundred distinct atoms and the
// full set is a hairball at 360px wide.
const RECURRING_DEFAULT_ABOVE = 40
// The engine tick at which the first fit-to-view happens. Early enough
// that the reader barely sees the unfitted layout, late enough that the
// layout has spread; after this tick a zoom by the reader is theirs.
const FIT_AT_TICK = 40

function nodeVal(n: FGNode): number {
  return 4 + Math.sqrt(n.hitCount) * 2.5
}

function nodeRadius(n: FGNode): number {
  return Math.sqrt(nodeVal(n)) * NODE_REL_SIZE
}

const TIER_LEGEND = ["atomic", "molecule", "reaction"]

function restrictGraph(g: TraceGraphData, keep: Set<string>): TraceGraphData {
  return {
    nodes: g.nodes.filter((n) => keep.has(n.id)),
    links: g.links.filter((l) => keep.has(l.source) && keep.has(l.target)),
  }
}

// Two views of the same hits. "local" is the default and the one that's
// actually legible: the selected passage's atoms plus its neighbours',
// few enough nodes to draw every name in full (Obsidian's local-graph
// pane is the model). "document" is the whole-text force graph — kept as
// the secondary view because the overall shape is sometimes interesting,
// not because individual nodes can be read off it at a hundred nodes.
export function TraceNetwork({
  doc,
  chunks,
  activeIndex,
  openAtomId,
  onAtomClick,
  onAtomHover,
  className,
}: {
  doc: DocumentTraceDoc
  chunks: ChunkWithHits[]
  activeIndex: number | null
  openAtomId: string | null
  onAtomClick: (id: string) => void
  onAtomHover: (id: string | null) => void
  className?: string
}) {
  const [mode, setMode] = useState<Mode>("local")
  const [recurringOnly, setRecurringOnly] = useState<boolean | null>(null)
  const fgRef = useRef<ForceGraphMethods<FGNode, FGLink> | undefined>(undefined)
  const containerRef = useRef<HTMLDivElement>(null)

  const local = mode === "local" && activeIndex !== null

  // Which atoms fired in the selected passage — ringed in both modes, so
  // the whole-text view still answers "where does this passage sit".
  const focus = useMemo(
    () => new Set(chunks.find((c) => c.index === activeIndex)?.hits.map((h) => h.atom_id) ?? []),
    [chunks, activeIndex]
  )

  // The whole-document graph depends on the document only. Deriving it
  // from the selection too meant every passage click rebuilt the graph,
  // re-ran the layout, and re-fit the view — the zoom reset the reader
  // kept hitting.
  const docGraph = useMemo(() => traceGraph(doc), [doc])
  const recurringCount = useMemo(() => docGraph.nodes.filter((n) => n.hitCount > 1).length, [docGraph])
  const showRecurringOnly = recurringOnly ?? docGraph.nodes.length > RECURRING_DEFAULT_ABOVE
  // Deliberately independent of the selection: including the selected
  // passage's one-off atoms would change the node set on every passage
  // click, which rebuilds the layout and resets the reader's zoom.
  const documentGraph = useMemo(() => {
    if (!showRecurringOnly) return docGraph
    return restrictGraph(docGraph, new Set(docGraph.nodes.filter((n) => n.hitCount > 1).map((n) => n.id)))
  }, [docGraph, showRecurringOnly])
  const localGraph = useMemo(
    () => (activeIndex === null ? null : localTraceGraph(doc, chunks, activeIndex)),
    [doc, chunks, activeIndex]
  )
  const graph = local ? localGraph! : documentGraph

  // force-graph mutates node objects in place (x, y, vx…); handing it a
  // fresh array per graph identity keeps layouts from bleeding across.
  const graphData = useMemo(() => ({ nodes: graph.nodes.map((n) => ({ ...n })), links: graph.links.map((l) => ({ ...l })) }), [graph])

  // Always-on labels. Local: every node. Document: the selected passage's
  // atoms plus the most frequent few; everything else waits for zoom.
  const labeled = useMemo(() => {
    if (local) return new Set(graph.nodes.map((n) => n.id))
    const top = [...graph.nodes]
      .sort((a, b) => b.hitCount - a.hitCount)
      .slice(0, ALWAYS_LABELED_TOP)
      .map((n) => n.id)
    return new Set([...focus, ...top])
  }, [graph, local, focus])

  const height = local ? 440 : 600

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

  // Fit-to-view policy. The view is fitted once early in the layout (tick
  // FIT_AT_TICK) and once more when the engine settles — unless the reader
  // has zoomed or panned since the graph changed, in which case their
  // view is theirs and nothing touches it. onZoom fires for programmatic
  // fits too, so fits are bracketed by fittingRef and ignored.
  const tickRef = useRef(0)
  const userZoomedRef = useRef(false)
  const fittingRef = useRef(false)
  const fitTimerRef = useRef<number | undefined>(undefined)
  useEffect(() => {
    tickRef.current = 0
    userZoomedRef.current = false
  }, [graphData])
  useEffect(() => () => window.clearTimeout(fitTimerRef.current), [])
  const fit = () => {
    const fg = fgRef.current
    if (userZoomedRef.current || !fg) return
    fittingRef.current = true
    fg.zoomToFit(300, local ? LOCAL_FIT_PADDING : DOCUMENT_FIT_PADDING)
    window.clearTimeout(fitTimerRef.current)
    fitTimerRef.current = window.setTimeout(() => {
      if (local && !userZoomedRef.current) {
        const k = fg.zoom()
        if (k < LOCAL_MIN_ZOOM) fg.zoom(LOCAL_MIN_ZOOM, 200)
        else if (k > LOCAL_MAX_ZOOM) fg.zoom(LOCAL_MAX_ZOOM, 200)
      }
      fitTimerRef.current = window.setTimeout(() => {
        fittingRef.current = false
      }, 260)
    }, 340)
  }

  // Real collision force so nodes (and the labels under them) can't
  // overlap whatever the random initial layout was; charge/link tuning
  // alone only makes overlap less likely. Reheat after touching forces or
  // the change no-ops once cooldownTicks has run out.
  useEffect(() => {
    const fg = fgRef.current
    if (!fg) return
    fg.d3Force("charge")?.strength(local ? -340 : -260)
    fg.d3Force("link")?.distance(local ? 125 : 90)
    // Labels are screen-sized, so a graph-unit collision radius can only
    // keep centres apart, not guarantee label boxes never touch; the
    // local minimum zoom (see fit) is what makes the spacing hold on
    // screen.
    fg.d3Force("collide", forceCollide<FGNode>((n) => (local ? LOCAL_COLLIDE : nodeRadius(n) + 18)).strength(1))
    fg.d3ReheatSimulation()
  }, [graphData, local])

  const empty = graph.nodes.length === 0
  const linkColor = (l: FGLink) => ((l as FGLink).kind === "transition" ? "rgba(230, 225, 212, 0.6)" : "rgba(230, 225, 212, 0.25)")

  return (
    <section aria-label="pattern network" className={cn("border border-rule bg-bg-well", className)}>
      <header className="flex flex-wrap items-center justify-between gap-2 border-b border-rule px-4 py-2">
        <h3 className="font-mono text-[10px] tracking-[0.12em] text-ink-faint uppercase">
          {local ? "Around this passage" : "Whole document"}
        </h3>
        <div className="flex items-center gap-2">
          {!local && recurringCount > 0 && recurringCount < docGraph.nodes.length && (
            <div role="group" aria-label="which patterns" className="flex gap-1">
              {(
                [
                  [true, `recurring · ${recurringCount}`],
                  [false, `all · ${docGraph.nodes.length}`],
                ] as [boolean, string][]
              ).map(([v, label]) => (
                <button
                  key={label}
                  type="button"
                  aria-pressed={showRecurringOnly === v}
                  onClick={() => setRecurringOnly(v)}
                  className={cn(
                    "border px-2 py-0.5 font-mono text-[10px] tracking-wide transition-colors focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:outline-none",
                    showRecurringOnly === v ? "border-ink/40 bg-ink/10 text-foreground" : "border-rule text-ink-dim hover:border-ink/40"
                  )}
                >
                  {label}
                </button>
              ))}
            </div>
          )}
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
        </div>
      </header>

      <div ref={containerRef} className="w-full overflow-hidden" style={{ height }}>
        {empty ? (
          <p className="p-4 font-mono text-[11px] text-ink-faint">
            {activeIndex === null && mode === "local"
              ? "Select a passage to see its neighbourhood."
              : local
                ? "No pattern fires in this passage or the ones beside it."
                : "No pattern hits to graph."}
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
            nodeLabel={(n) => `${(n as FGNode).name.replace(/-/g, " ")} · fires ${(n as FGNode).hitCount}×`}
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
              const isOpen = node.id === openAtomId
              if (focus.has(node.id) || isOpen) {
                ctx.beginPath()
                ctx.arc(x, y, r + 3, 0, 2 * Math.PI)
                ctx.strokeStyle = isOpen ? "rgba(255, 255, 255, 0.95)" : "rgba(230, 225, 212, 0.9)"
                ctx.lineWidth = (isOpen ? 2.5 : 1.5) / globalScale
                ctx.stroke()
              }
              const zoomedIn = !local && globalScale >= LABEL_ZOOM && node.hitCount > 1
              if (!labeled.has(node.id) && !isOpen && !zoomedIn) return
              const lines = wrapName(node.name, 20, local ? 4 : 3)
              // Constant screen size in both modes: a name is either
              // readable or not drawn.
              const fontSize = LABEL_FONT_PX / globalScale
              ctx.font = `${fontSize}px "IBM Plex Mono", monospace`
              ctx.textAlign = "center"
              ctx.textBaseline = "top"
              ctx.fillStyle = local && !focus.has(node.id) ? "rgba(230, 225, 212, 0.62)" : "rgba(230, 225, 212, 0.9)"
              let ly = y + r + 4 / globalScale
              for (const line of lines) {
                ctx.fillText(line, x, ly)
                ly += fontSize * LABEL_LINE_HEIGHT
              }
            }}
            linkColor={(l) => linkColor(l as FGLink)}
            linkWidth={(l) => 0.6 + Math.sqrt((l as FGLink).weight)}
            linkLineDash={(l) => ((l as FGLink).kind === "co-occurrence" ? [2, 2] : null)}
            linkDirectionalArrowLength={(l) => ((l as FGLink).kind === "transition" ? 10 : 0)}
            linkDirectionalArrowRelPos={1}
            linkDirectionalArrowColor={(l) => linkColor(l as FGLink)}
            linkCurvature={0.15}
            backgroundColor="rgba(0,0,0,0)"
            onNodeClick={(n) => onAtomClick((n as FGNode).id)}
            onNodeHover={(n) => onAtomHover(n ? (n as FGNode).id : null)}
            onZoom={() => {
              if (!fittingRef.current && tickRef.current >= FIT_AT_TICK) userZoomedRef.current = true
            }}
            cooldownTicks={local ? 120 : 250}
            onEngineTick={() => {
              tickRef.current += 1
              if (tickRef.current === FIT_AT_TICK) fit()
            }}
            onEngineStop={() => {
              if (tickRef.current < FIT_AT_TICK) tickRef.current = FIT_AT_TICK
              fit()
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
        {!local && <span>labels: this passage + the {ALWAYS_LABELED_TOP} most frequent · zoom in for the rest</span>}
      </footer>
    </section>
  )
}
