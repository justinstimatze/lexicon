import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import readingOrderData from "@/data/reading-order.json"
import type { ReadingOrderData, ReadingOrderEdge, ReadingOrderFurther, ReadingOrderNode } from "@/lib/readingOrder"
import { Dialog, DialogContent, DialogBody } from "@/components/ui/dialog"

const data = readingOrderData as unknown as ReadingOrderData
const nodesByKey = new Map(data.nodes.map((n) => [n.key, n]))
const furtherByKey = new Map(data.further.map((f) => [f.key, f]))

// Further sources that lost the top-15 tree-reach cut but still cite (or
// are cited by) a tree node somewhere -- grouped by that single strongest
// parent so they render as satellites under it. Sources with no parent at
// all (nothing in the tree connects to them) are handled separately, in
// ReadingOrder.tsx's leftover "further keystones" list.
const furtherByParent = new Map<string, ReadingOrderFurther[]>()
for (const f of data.further) {
  if (!f.parent) continue
  const arr = furtherByParent.get(f.parent) ?? []
  arr.push(f)
  furtherByParent.set(f.parent, arr)
}

const NODE_WIDTH = 208
const NODE_HEIGHT = 52
// Matches DialogContent's `max-w-md` in ui/dialog.tsx (28rem). The drawer
// docks fixed over the right DRAWER_WIDTH px of the viewport without
// shifting page layout, so nothing else here learns to leave room for it
// automatically -- this has to be done by hand.
const DRAWER_WIDTH = 448

const TIER_LABELS = ["Era I", "Era II", "Era III", "Era IV", "Era V", "Era VI", "Era VII", "Era VIII"]

function edgeRank(e: ReadingOrderEdge): number {
  if (e.kind === "solid" && e.direction_source === "scaffolds") return 0
  if (e.kind === "solid") return 1
  return 2
}

function edgeLabel(e: ReadingOrderEdge): string {
  if (e.kind === "dashed") return "secondary cross-reference"
  return e.direction_source === "scaffolds" ? "confirmed prerequisite" : "strongest link, unconfirmed"
}

function bySolidFirst(a: ReadingOrderEdge, b: ReadingOrderEdge) {
  const r = edgeRank(a) - edgeRank(b)
  return r !== 0 ? r : b.weight - a.weight
}

// Eras stack vertically, each one a wrapping row of nodes -- this used to
// run left-to-right instead, which meant every era past whatever fit on
// screen needed a SEPARATE horizontal scroll on top of the page's own
// vertical one. Node positions are no longer computed from a formula:
// they're MEASURED off the actual rendered DOM (measure() below), because
// a wrapping flex row's layout depends on the viewport width in a way a
// column/row formula can't predict. The SVG overlay just connects
// whatever those measured centers turn out to be.
//
// Solid = each node's single strongest incoming edge (its parent above);
// dashed = its next-strongest cross-references, always a density signal.
// A solid edge's DIRECTION comes from one of two places, and the line
// style says which: a scaffolds-from prerequisite between the two
// sources (confirmed — a real "grasping A primes B" judgment), or, where
// none exists yet, the old proxy (the corpus enforces `related` as a
// reciprocal, undirected field, so which end is "upstream" is a tiebreak
// on atom_count, not a real direction — see
// render/cmd/lexicon/cmd_reading_order.go). Scaffolds-from covers 877 of
// 3664 atoms as of 2026-08-30, so most solid edges are still the proxy;
// each renders honestly rather than looking equally confirmed.
type Selectable = ReadingOrderNode | ReadingOrderFurther

function isTreeNode(n: Selectable): n is ReadingOrderNode {
  return "tier" in n
}

// Lets a click outside the diagram (the era lists and further-keystones
// grid below it in ReadingOrder.tsx) open the same drawer a graph click
// does, and -- for anything with a rendered node -- scroll/highlight it
// in place, via the same selectNode path a graph click already uses.
// State stays local to TechTree rather than lifting to ReadingOrder: the
// position-measurement, drawer-width narrowing, and scroll-into-view
// effects below are all keyed off this component's own DOM refs, and
// none of that has a reason to live in the parent.
export interface TechTreeHandle {
  select: (key: string) => void
}

export const TechTree = forwardRef<TechTreeHandle>(function TechTree(_props, ref) {
  const [selected, setSelected] = useState<Selectable | null>(null)
  const [showDashed, setShowDashed] = useState(false)
  // Further-satellites show by default (empty set); a node's own key goes
  // in here only once someone collapses that specific cluster.
  const [collapsedFurther, setCollapsedFurther] = useState<Set<string>>(new Set())
  const toggleFurther = useCallback((key: string) => {
    setCollapsedFurther((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])
  const [positions, setPositions] = useState<Map<string, { x: number; y: number }>>(new Map())
  const [canvasSize, setCanvasSize] = useState({ width: 0, height: 0 })
  const [availableWidth, setAvailableWidth] = useState<number | undefined>(undefined)
  const containerRef = useRef<HTMLDivElement>(null)
  const nodeRefs = useRef(new Map<string, HTMLButtonElement>())

  const tiers = useMemo(() => {
    const byTier = new Map<number, ReadingOrderNode[]>()
    for (const n of data.nodes) {
      const arr = byTier.get(n.tier) ?? []
      arr.push(n)
      byTier.set(n.tier, arr)
    }
    return [...byTier.entries()].sort((a, b) => a[0] - b[0])
  }, [])

  const measure = useCallback(() => {
    const container = containerRef.current
    if (!container) return
    const containerRect = container.getBoundingClientRect()
    const next = new Map<string, { x: number; y: number }>()
    for (const [key, el] of nodeRefs.current) {
      const r = el.getBoundingClientRect()
      next.set(key, { x: r.left - containerRect.left + r.width / 2, y: r.top - containerRect.top + r.height / 2 })
    }
    setPositions(next)
    setCanvasSize({ width: container.clientWidth, height: container.scrollHeight })
  }, [])

  useLayoutEffect(() => {
    measure()
  }, [measure, tiers, availableWidth])

  useEffect(() => {
    const container = containerRef.current
    if (!container || typeof ResizeObserver === "undefined") return
    const ro = new ResizeObserver(() => measure())
    ro.observe(container)
    return () => ro.disconnect()
  }, [measure])

  // Narrows the diagram to whatever's actually visible beside the open
  // drawer, so the clicked node can't land hidden behind it. Below
  // roughly two node-widths of visible room, the constraint is skipped
  // instead of squeezing the grid down to nothing -- narrow viewports
  // already give the drawer most of the screen anyway.
  function computeAvailableWidth(): number | undefined {
    const container = containerRef.current
    if (!container) return undefined
    const visible = window.innerWidth - DRAWER_WIDTH - container.getBoundingClientRect().left
    return visible >= NODE_WIDTH * 2 ? visible : undefined
  }

  function selectNode(n: Selectable) {
    setSelected(n)
    setAvailableWidth(computeAvailableWidth())
  }

  function deselect() {
    setSelected(null)
    setAvailableWidth(undefined)
  }

  useImperativeHandle(ref, () => ({
    select(key: string) {
      const target = nodesByKey.get(key) ?? furtherByKey.get(key)
      if (target) selectNode(target)
    },
  }))

  useEffect(() => {
    if (!selected) return
    function onResize() {
      setAvailableWidth(computeAvailableWidth())
    }
    window.addEventListener("resize", onResize)
    return () => window.removeEventListener("resize", onResize)
  }, [selected])

  // Runs after the width narrows and the grid has reflowed around it --
  // selectNode sets both states in the same batch, so by the time this
  // fires (after paint) the DOM already reflects the drawer-aware layout,
  // not the one from before it narrowed.
  useEffect(() => {
    if (!selected) return
    nodeRefs.current.get(selected.key)?.scrollIntoView({ behavior: "smooth", block: "center" })
  }, [selected])

  const connected = useMemo(() => {
    if (!selected) return null
    const nodeKeys = new Set<string>([selected.key])
    const edgeKeys = new Set<string>()
    for (const e of data.edges) {
      if (e.from === selected.key || e.to === selected.key) {
        edgeKeys.add(`${e.from}->${e.to}`)
        nodeKeys.add(e.from)
        nodeKeys.add(e.to)
      }
    }
    return { nodeKeys, edgeKeys }
  }, [selected])

  const visibleEdges = showDashed ? data.edges : data.edges.filter((e) => e.kind === "solid")

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-4 font-mono text-[11px] text-ink-faint">
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-0 w-5 border-t-2 border-primary" /> confirmed prerequisite
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-0 w-5 border-t-2 border-dashed border-primary/70" /> strongest link,
          direction unconfirmed
        </span>
        <label className="flex cursor-pointer items-center gap-1.5 border border-rule-light px-1.5 py-0.5 hover:border-primary/60">
          <input type="checkbox" checked={showDashed} onChange={(e) => setShowDashed(e.target.checked)} className="accent-primary" />
          <span className="inline-block h-0 w-5 border-t border-dashed border-rule-light" /> show secondary cross-reference
        </label>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-2.5 w-2.5 border-2 border-primary" /> core read
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-2.5 w-2.5 border border-dashed border-ink-dim" /> keystone
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-2 w-4 rounded-full border border-rule-light bg-bg-well" /> further keystone
          (satellite, no line — see note below)
        </span>
      </div>
      <p className="font-mono text-[10px] text-ink-faint">
        Top-to-bottom position is a computed era — solid lines are each source's
        strongest link into the next. A full line means a real scaffolds-from prerequisite was found; a dashed-but-
        heavy line means it's still the strongest link but no prerequisite has been confirmed yet, so treat that
        direction as a guess. Click a source to see what it's primed by and what it primes next. Read any era in
        any order. A source badged "N further" also has satellites attached — real sources that lost the top-15
        cut on tree-restricted reach, tucked under whichever tree node cites them most. No line is drawn to them
        deliberately: that association is corpus-wide, not tree-confirmed, so it gets adjacency, not an edge.
        Click the badge to collapse a crowded cluster.
      </p>

      <div
        ref={containerRef}
        className="relative border border-rule bg-bg-well transition-[max-width] duration-300"
        style={{ maxWidth: availableWidth }}
      >
        <svg
          className="pointer-events-none absolute inset-0"
          width={canvasSize.width}
          height={canvasSize.height}
        >
          {visibleEdges.map((e, i) => {
            const from = positions.get(e.from)
            const to = positions.get(e.to)
            if (!from || !to) return null
            const midY = (from.y + to.y) / 2
            const confirmed = e.kind === "solid" && e.direction_source === "scaffolds"
            const baseOpacity = e.kind === "solid" ? (confirmed ? 0.75 : 0.5) : 0.4
            const isConnected = connected?.edgeKeys.has(`${e.from}->${e.to}`) ?? false
            const opacity = isConnected ? 1 : connected ? baseOpacity * 0.35 : baseOpacity
            const strokeWidth = (e.kind === "solid" ? 1.5 : 1) + (isConnected ? 1 : 0)
            return (
              <path
                key={i}
                d={`M ${from.x} ${from.y} C ${from.x} ${midY}, ${to.x} ${midY}, ${to.x} ${to.y}`}
                fill="none"
                stroke={e.kind === "solid" ? "var(--accent)" : "var(--rule-light)"}
                strokeWidth={strokeWidth}
                strokeDasharray={e.kind === "dashed" ? "3,3" : e.kind === "solid" && !confirmed ? "7,3" : undefined}
                opacity={opacity}
                style={{ transition: "opacity 150ms, stroke-width 150ms" }}
              />
            )
          })}
        </svg>

        <div className="relative flex flex-col gap-6 p-4">
          {tiers.map(([tier, nodes]) => (
            <div key={tier} className="flex flex-col gap-2">
              <div className="font-mono text-[10px] tracking-[0.1em] text-ink-faint uppercase">
                {TIER_LABELS[tier] ?? `Era ${tier + 1}`}
              </div>
              <div className="flex flex-wrap gap-x-2 gap-y-5">
                {nodes.map((n) => {
                  const isSelected = selected?.key === n.key
                  const isNeighbor = !isSelected && (connected?.nodeKeys.has(n.key) ?? false)
                  const isDimmed = !!connected && !isSelected && !isNeighbor
                  const kids = furtherByParent.get(n.key) ?? []
                  const kidsCollapsed = collapsedFurther.has(n.key)
                  return (
                    // relative + fixed width so the badge (absolutely
                    // positioned against THIS wrapper, not the button) can
                    // sit outside the button's own box without the
                    // button's overflow-hidden clipping it, and so
                    // satellite pills wrap to the same width as the node
                    // above them.
                    <div key={n.key} className="relative flex flex-col items-start" style={{ width: NODE_WIDTH }}>
                      <button
                        ref={(el) => {
                          if (el) nodeRefs.current.set(n.key, el)
                          else nodeRefs.current.delete(n.key)
                        }}
                        type="button"
                        onClick={() => selectNode(n)}
                        title={`${n.title} — ${n.author}`}
                        style={{ height: NODE_HEIGHT }}
                        className={
                          "flex w-full flex-col justify-center overflow-hidden rounded-sm border bg-bg-raised px-2 py-1 text-left transition hover:border-primary/70 hover:bg-bg-raised/80 " +
                          (n.kind === "core" ? "border-2 border-primary/60" : "border border-dashed border-ink-dim") +
                          (isSelected ? " ring-2 ring-primary ring-offset-1 ring-offset-bg-well" : "") +
                          (isNeighbor ? " border-primary" : "") +
                          (isDimmed ? " opacity-45" : "")
                        }
                      >
                        <span className="truncate text-[11px] font-semibold text-foreground">{n.title}</span>
                        <span className="truncate text-[10px] text-ink-faint">{n.author}</span>
                      </button>
                      {kids.length > 0 && (
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            toggleFurther(n.key)
                          }}
                          title={kidsCollapsed ? "show further keystones" : "collapse further keystones"}
                          className={
                            "absolute -top-2 -right-2 z-10 rounded-full border px-1.5 py-0.5 font-mono text-[9px] tracking-wide whitespace-nowrap shadow-[0_1px_3px_rgba(0,0,0,0.5)] transition " +
                            (kidsCollapsed
                              ? "border-rule-light bg-bg-well text-ink-faint hover:border-primary/60 hover:text-primary"
                              : "border-primary bg-primary text-accent-ink hover:bg-accent-soft")
                          }
                        >
                          {kids.length} further
                        </button>
                      )}
                      {kids.length > 0 && !kidsCollapsed && (
                        <div className="mt-1.5 flex flex-wrap gap-1">
                          {kids.map((f) => (
                            <FurtherPill key={f.key} node={f} isSelected={selected?.key === f.key} onSelect={selectNode} />
                          ))}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      </div>

      <Dialog open={!!selected} onOpenChange={(open) => !open && deselect()}>
        <DialogContent title={selected ? selected.title : "Source detail"} description={selected?.author}>
          <DialogBody>{selected && <NodeDetail node={selected} onSelect={selectNode} />}</DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  )
})

// Selecting a satellite (click opens the same detail drawer a tree node
// opens, via onSelect) also promotes it to full node size in place -- same
// dimensions and layout as a real node, but a dotted border rather than
// core's solid or keystone's dashed, so the moment of focus doesn't read
// as "this just became a tree member." The read-it link, if any, lives in
// the drawer now (matching a tree node's own behavior) rather than on the
// pill itself.
function FurtherPill({
  node,
  isSelected,
  onSelect,
}: {
  node: ReadingOrderFurther
  isSelected: boolean
  onSelect: (n: Selectable) => void
}) {
  const label = `${node.title} — ${node.author}`
  if (isSelected) {
    return (
      <button
        type="button"
        onClick={() => onSelect(node)}
        title={label}
        style={{ width: NODE_WIDTH, height: NODE_HEIGHT }}
        className="flex flex-col justify-center overflow-hidden rounded-sm border border-dotted border-primary bg-bg-raised px-2 py-1 text-left ring-2 ring-primary ring-offset-1 ring-offset-bg-well transition"
      >
        <span className="truncate text-[11px] font-semibold text-foreground">{node.title}</span>
        <span className="truncate text-[10px] text-ink-faint">{node.author}</span>
      </button>
    )
  }
  return (
    <button
      type="button"
      onClick={() => onSelect(node)}
      title={label}
      className="max-w-[190px] truncate rounded-full border border-rule-light bg-bg-well px-2 py-0.5 font-mono text-[9.5px] text-ink-dim transition hover:border-primary/60 hover:text-primary"
    >
      {node.title}
    </button>
  )
}

function EdgeList({
  title,
  edges,
  side,
  empty,
  onSelect,
}: {
  title: string
  edges: ReadingOrderEdge[]
  side: "from" | "to"
  empty: string
  onSelect: (n: Selectable) => void
}) {
  return (
    <div>
      <div className="mb-1 font-mono text-[10px] tracking-[0.08em] text-ink-faint uppercase">{title}</div>
      {edges.length === 0 ? (
        <div className="text-[11px] text-ink-faint italic">{empty}</div>
      ) : (
        <ul className="flex flex-col gap-2">
          {edges.map((e, i) => {
            const other = nodesByKey.get(side === "from" ? e.from : e.to)
            if (!other) return null
            return (
              <li key={i}>
                <button
                  type="button"
                  onClick={() => onSelect(other)}
                  className="flex w-full flex-col items-start gap-0.5 text-left text-[12px] leading-snug text-foreground hover:text-primary hover:underline"
                >
                  <span>{other.title}</span>
                  <span
                    className={
                      "font-mono text-[9px] uppercase tracking-wide " +
                      (e.kind === "solid" && e.direction_source === "scaffolds" ? "text-primary" : "text-ink-faint")
                    }
                  >
                    {edgeLabel(e)}
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function NodeDetail({ node, onSelect }: { node: Selectable; onSelect: (n: Selectable) => void }) {
  const tree = isTreeNode(node)
  const primedBy = tree ? data.edges.filter((e) => e.to === node.key).sort(bySolidFirst) : []
  const primesNext = tree ? data.edges.filter((e) => e.from === node.key).sort(bySolidFirst) : []
  const parent = !tree && node.parent ? nodesByKey.get(node.parent) : undefined
  return (
    <div>
      <div className="text-primary">{tree ? (node.kind === "core" ? "core read" : "keystone") : "further keystone"}</div>
      <div className="mb-1 text-sm font-semibold text-foreground">{node.title}</div>
      <div className="mb-3 text-ink-dim">
        {node.author} — {node.edition}
      </div>
      {node.note && <div className="mb-3 text-[12px] text-ink-dim italic">{node.note}</div>}
      <dl className="mb-4 grid grid-cols-2 gap-x-4 gap-y-1 font-mono text-[11px] text-ink-faint">
        <dt>atoms drawn</dt>
        <dd className="text-ink-dim">{node.atom_count}</dd>
        {tree && (
          <>
            <dt>era</dt>
            <dd className="text-ink-dim">{TIER_LABELS[node.tier] ?? node.tier}</dd>
            <dt>cited back by</dt>
            <dd className="text-ink-dim">
              {node.reach} other {node.reach === 1 ? "source" : "sources"} in this list
            </dd>
          </>
        )}
        {!tree && (
          <>
            <dt>leaned on by</dt>
            <dd className="text-ink-dim">{node.total_in_degree} atoms corpus-wide</dd>
          </>
        )}
      </dl>
      {tree ? (
        <div className="mb-4 flex flex-col gap-4">
          <EdgeList
            title="Primed by"
            edges={primedBy}
            side="from"
            empty="Nothing else in this list primes this — a root."
            onSelect={onSelect}
          />
          <EdgeList
            title="Primes next"
            edges={primesNext}
            side="to"
            empty="Nothing forward from here yet — an endpoint in this tree."
            onSelect={onSelect}
          />
        </div>
      ) : (
        parent && (
          <div className="mb-4">
            <div className="mb-1 font-mono text-[10px] tracking-[0.08em] text-ink-faint uppercase">Attached to</div>
            <button
              type="button"
              onClick={() => onSelect(parent)}
              className="text-left text-[12px] leading-snug text-foreground hover:text-primary hover:underline"
            >
              {parent.title}
            </button>
            <div className="mt-1 text-[11px] text-ink-faint italic">
              A real connection, not a confirmed prerequisite — not enough of the tree above cites it back to earn
              its own place in the sequence.
            </div>
          </div>
        )
      )}
      {node.url ? (
        <a
          href={node.url}
          target="_blank"
          rel="noreferrer"
          className="mt-3 inline-block rounded-sm border border-rule-light px-2 py-1 text-[11px] text-primary hover:border-primary/50"
        >
          read it →
        </a>
      ) : (
        <div className="mt-3 text-[11px] text-ink-faint italic">no verified free or purchasable link found yet</div>
      )}
    </div>
  )
}
