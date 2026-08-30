import { useMemo, useState } from "react"
import graphData from "@/data/graph.json"
import type { LexGraph, LexNode } from "@/lib/graph"
import { GAP_SUGGESTIONS } from "@/lib/gapSuggestions"
import { AtomCard } from "@/components/AtomCard"
import { Dialog, DialogContent, DialogBody } from "@/components/ui/dialog"

const graph = graphData as unknown as LexGraph

// Mirrors render/internal/viz/pivot.go's PivotRowOrder/PivotColOrder —
// the row/col vocabulary the gap-triage pass (and GAP_SUGGESTIONS'
// keys) was written against. Changing this order changes nothing
// visually load-bearing, but keep it in sync with the Go source since
// the two aren't linted against each other.
const ROW_ORDER = [
  "state", "situation", "process", "question", "frame",
  "claim", "posture", "composition", "structure",
]
const COL_ORDER = [
  "state", "process", "posture", "frame", "claim",
  "question", "composition", "structure", "typology", "warning",
]

const TIERS = ["atomic", "composition", "molecule", "reaction"] as const
type Tier = (typeof TIERS)[number]

const colorByCluster = new Map(graph.clusters.map((c) => [c.id, c.color]))
const nodesById = new Map(graph.nodes.map((n) => [n.id, n]))

interface Cell {
  row: string
  col: string
  all: LexNode[]
  visible: LexNode[]
  isAbsoluteGap: boolean
}

export function PivotTable() {
  const [tiers, setTiers] = useState<Record<Tier, boolean>>({
    atomic: true,
    composition: true,
    molecule: true,
    reaction: true,
  })
  const [statusFilters, setStatusFilters] = useState({ active: true, underReview: false })
  const [query, setQuery] = useState("")
  const [clusterFilter, setClusterFilter] = useState<string | null>(null)
  const [selected, setSelected] = useState<LexNode | null>(null)
  const [gapCell, setGapCell] = useState<{ row: string; col: string } | null>(null)
  const clusterFilterLabel = clusterFilter ? graph.clusters.find((c) => c.id === clusterFilter)?.label : null

  const nodesByCell = useMemo(() => {
    const map = new Map<string, LexNode[]>()
    for (const n of graph.nodes) {
      const key = `${n.type_in}__${n.type_out}`
      const arr = map.get(key)
      if (arr) arr.push(n)
      else map.set(key, [n])
    }
    for (const arr of map.values()) {
      arr.sort((a, b) => {
        if (a.is_molecule !== b.is_molecule) return a.is_molecule ? -1 : 1
        if (a.in_degree !== b.in_degree) return b.in_degree - a.in_degree
        return a.id < b.id ? -1 : 1
      })
    }
    return map
  }, [])

  function passesFilters(n: LexNode) {
    const okStatus = (n.status === "active" && statusFilters.active) || (n.status === "under-review" && statusFilters.underReview)
    if (!okStatus) return false
    if (!tiers[n.tier as Tier]) return false
    if (clusterFilter && n.cluster !== clusterFilter) return false
    const q = query.trim().toLowerCase()
    if (q && !(n.id.toLowerCase().includes(q) || n.name.toLowerCase().includes(q))) return false
    return true
  }

  function openAtom(n: LexNode) {
    setGapCell(null)
    setSelected(n)
  }

  function filterToCluster(clusterId: string) {
    setClusterFilter(clusterId)
    setSelected(null)
    setGapCell(null)
  }

  function jumpToAtom(id: string) {
    const n = nodesById.get(id)
    if (n) {
      setGapCell(null)
      setSelected(n)
    }
  }

  const { rows, totalVisible, totalGaps } = useMemo(() => {
    let visibleCount = 0
    let gapCount = 0
    const built = ROW_ORDER.map((row) => ({
      row,
      cells: COL_ORDER.map((col): Cell => {
        const all = nodesByCell.get(`${row}__${col}`) ?? []
        const visible = all.filter(passesFilters)
        visibleCount += visible.length
        const isAbsoluteGap = all.length === 0
        if (isAbsoluteGap) gapCount += 1
        return { row, col, all, visible, isAbsoluteGap }
      }),
    }))
    return { rows: built, totalVisible: visibleCount, totalGaps: gapCount }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodesByCell, tiers, statusFilters, clusterFilter, query])

  return (
    <div className="flex w-full flex-col text-foreground">
      <h1 className="sr-only">Pivot table — atoms grouped by type-in and type-out</h1>
      <div className="sticky top-0 z-30 flex flex-wrap items-center gap-3 border-b border-rule bg-muted px-3 py-2 font-mono text-[11px] sm:gap-4">
        <span
          className="text-ink-faint uppercase tracking-wide"
          title="Rows and columns are the atom's own type-in/type-out shape — e.g. a 'state → claim' atom sits in the state row, claim column."
        >
          type-in ↓ / type-out →
        </span>
        <div className="flex flex-wrap items-center gap-2.5">
          <span className="text-ink-faint">tier:</span>
          {TIERS.map((t) => (
            <label key={t} className="flex cursor-pointer items-center gap-1.5 px-1 py-1 text-ink-dim">
              <input
                type="checkbox"
                checked={tiers[t]}
                onChange={(e) => setTiers((prev) => ({ ...prev, [t]: e.target.checked }))}
                className="accent-primary"
              />
              {t}
            </label>
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-2.5">
          <span className="text-ink-faint">status:</span>
          <label className="flex cursor-pointer items-center gap-1.5 px-1 py-1 text-ink-dim">
            <input
              type="checkbox"
              checked={statusFilters.active}
              onChange={(e) => setStatusFilters((p) => ({ ...p, active: e.target.checked }))}
              className="accent-primary"
            />
            active
          </label>
          <label className="flex cursor-pointer items-center gap-1.5 px-1 py-1 text-ink-dim">
            <input
              type="checkbox"
              checked={statusFilters.underReview}
              onChange={(e) => setStatusFilters((p) => ({ ...p, underReview: e.target.checked }))}
              className="accent-primary"
            />
            under-review
          </label>
        </div>
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="search by name or id…"
          aria-label="Search atoms by name or id"
          className="rounded-sm border border-rule-light bg-bg-well px-2 py-2 text-foreground placeholder:text-ink-faint focus:outline-none focus:ring-1 focus:ring-primary/60"
        />
        {clusterFilter && (
          <button
            type="button"
            onClick={() => setClusterFilter(null)}
            className="flex items-center gap-1.5 rounded-sm border border-primary/50 bg-primary/10 px-2 py-1 text-primary hover:bg-primary/20"
          >
            {clusterFilterLabel ?? clusterFilter}
            <span aria-hidden="true">×</span>
          </button>
        )}
        <span
          className="ml-auto text-ink-faint"
          title="Unfilled cells: type shapes with zero atoms in the whole catalog, filters aside — a real gap, not a filtered-out count."
        >
          {totalVisible} atoms visible · {totalGaps} unfilled cells
        </span>
      </div>
      <div className="border-b border-rule bg-bg-well px-3 py-1.5 font-mono text-[10px] text-ink-faint">
        each cell: <span className="text-ink-dim">visible</span> or{" "}
        <span className="text-ink-dim">visible / total</span> when a filter is hiding some. chip color =
        cluster (community) — hover a chip for its name, click for detail and the cluster's full label.
        within a cell, order is molecules first, then descending in-degree — the first chips are the
        cell's hubs, not a random or alphabetical order.
      </div>

      <div
        className="overflow-x-auto focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-inset"
        tabIndex={0}
        role="region"
        aria-label="Pivot table, horizontally scrollable"
      >
          <table className="w-full border-collapse text-[11px]">
            <thead>
              <tr>
                <th scope="col" className="sticky left-0 z-20 h-9 border border-rule bg-muted p-2 text-center text-ink-faint italic">
                  <span className="sr-only">type-in (row) / type-out (column)</span>
                  &nbsp;
                </th>
                {COL_ORDER.map((co) => (
                  <th
                    key={co}
                    scope="col"
                    className="h-9 min-w-[130px] border border-rule bg-muted p-2 text-center font-semibold text-foreground"
                  >
                    {co}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map(({ row, cells }) => (
                <tr key={row}>
                  {/* sticky left-0 only now — the table lives in normal page
                      flow (no inner vertical scroll box), so only the
                      horizontal stick (which row am I in, scrolled right)
                      still applies; a vertical stick here would need to
                      track the filter bar's height, which changes as it
                      wraps on narrow viewports. */}
                  <th scope="row" className="sticky left-0 z-10 min-w-[90px] border border-rule bg-muted p-2 text-right font-semibold text-foreground">
                    {row}
                  </th>
                  {cells.map((cell) => (
                    <td
                      key={cell.col}
                      className={
                        "min-w-[130px] max-w-[220px] border border-rule p-1.5 align-top " +
                        (cell.isAbsoluteGap
                          ? "cursor-pointer border-dashed border-accent-soft/60 bg-bg-well hover:border-primary/70 hover:bg-primary/10"
                          : "")
                      }
                      onClick={
                        cell.isAbsoluteGap
                          ? () => {
                              setSelected(null)
                              setGapCell({ row: cell.row, col: cell.col })
                            }
                          : undefined
                      }
                    >
                      {cell.visible.length > 0 ? (
                        <>
                          <span className="mb-1 block font-mono text-sm font-semibold text-primary">
                            {cell.visible.length}
                            {cell.all.length !== cell.visible.length ? ` / ${cell.all.length}` : ""}
                          </span>
                          {/* Capped + internally scrollable: an uncapped cell
                              (claim→claim runs 166 chips) stretched its whole
                              <tr> to 1000+px, which is what broke the sticky
                              row label below — nothing could keep a label
                              pinned to a row taller than the viewport. Capping
                              here fixes both the label and the "scroll past a
                              wall of chips to reach the next row" problem. */}
                          <div className="flex max-h-[240px] flex-wrap gap-1 overflow-y-auto pr-0.5">
                            {cell.visible.map((n) => (
                              <button
                                key={n.id}
                                title={`${n.id} · ${n.name}`}
                                aria-label={`${n.id} · ${n.name}`}
                                onClick={(e) => {
                                  e.stopPropagation()
                                  openAtom(n)
                                }}
                                style={{ backgroundColor: colorByCluster.get(n.cluster) ?? "#5a4f3a" }}
                                // One size everywhere, not a breakpoint-conditional
                                // one — the previous max-[599px] rule gave phones
                                // 44x24 chips but left the 600-767px tablet range
                                // at 30x24, a real regression right where touch
                                // input is still the norm. h-7 w-11 is bigger
                                // than either of the old sizes, at every width.
                                className={
                                  "flex h-7 w-11 items-center justify-center border font-mono text-[9px] leading-none text-accent-ink transition hover:brightness-125 hover:ring-2 hover:ring-inset hover:ring-white/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/80 " +
                                  (n.is_molecule ? "border-2 border-primary" : "border-bg-well/60") +
                                  (selected?.id === n.id ? " outline outline-2 outline-offset-1 outline-red-500" : "")
                                }
                              >
                                {n.id.replace(/^lex-/, "")}
                              </button>
                            ))}
                          </div>
                        </>
                      ) : cell.all.length > 0 ? (
                        <span className="font-mono text-ink-faint">0 visible / {cell.all.length} filtered</span>
                      ) : (
                        <span className="block pt-2 text-center text-[10px] text-ink-faint italic">— gap —</span>
                      )}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
      </div>

      <Dialog
        open={!!selected || !!gapCell}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null)
            setGapCell(null)
          }
        }}
      >
        <DialogContent
          title={selected ? selected.name : gapCell ? `${gapCell.row} → ${gapCell.col} — unfilled cell` : "Detail"}
          description={selected?.id}
        >
          <DialogBody>
            {selected && <AtomCard node={selected} onClusterClick={filterToCluster} onAtomClick={jumpToAtom} />}
            {gapCell && <GapCard row={gapCell.row} col={gapCell.col} />}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function GapCard({ row, col }: { row: string; col: string }) {
  const suggestions = GAP_SUGGESTIONS[`${row}__${col}`] ?? []
  return (
    <div>
      <div className="text-primary">gap</div>
      <div className="mb-2 text-sm font-semibold text-foreground">
        {row} → {col}
      </div>
      <div className="mb-3 text-ink-dim">
        No atom fills this cell yet.
      </div>
      <div className="mb-1 text-ink-faint uppercase tracking-wide">candidates that might fit</div>
      {suggestions.length === 0 ? (
        <div className="text-ink-faint italic">No obvious candidate — open question.</div>
      ) : (
        <ul className="flex flex-col gap-2">
          {suggestions.map((s) => (
            <li key={s.name}>
              <div className="font-semibold text-foreground">{s.name}</div>
              <div className="text-ink-dim">{s.why}</div>
            </li>
          ))}
        </ul>
      )}
      <div className="mt-3 text-[10px] text-ink-faint">
        Hypotheses only; not minted atoms. They mark the conceptual shape a real atom could take.
      </div>
    </div>
  )
}
