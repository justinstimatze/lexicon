import { useEffect, useMemo, useRef, useState } from "react"
import { useNavigate, useParams } from "react-router-dom"
import graphData from "@/data/graph.json"
import type { LexGraph, LexNode } from "@/lib/graph"
import { AtomCard } from "@/components/AtomCard"
import { Dialog, DialogContent, DialogBody } from "@/components/ui/dialog"

const graph = graphData as unknown as LexGraph

const TIERS = ["atomic", "composition", "molecule", "reaction"] as const
type Tier = (typeof TIERS)[number]

const colorByCluster = new Map(graph.clusters.map((c) => [c.id, c.color]))
const labelByCluster = new Map(graph.clusters.map((c) => [c.id, c.label]))
const nodesById = new Map(graph.nodes.map((n) => [n.id, n]))

type SortKey = "id" | "name" | "type_in" | "type_out" | "tier" | "cluster" | "in_degree" | "status"

const COLUMNS: { key: SortKey; label: string; className?: string }[] = [
  { key: "id", label: "id", className: "w-24" },
  { key: "name", label: "name" },
  { key: "type_in", label: "type-in", className: "w-28" },
  { key: "type_out", label: "type-out", className: "w-28" },
  { key: "tier", label: "tier", className: "w-28" },
  { key: "cluster", label: "cluster", className: "w-40" },
  { key: "in_degree", label: "in-degree", className: "w-32" },
  { key: "status", label: "status", className: "w-28" },
]

function compareBy(key: SortKey) {
  return (a: LexNode, b: LexNode) => {
    if (key === "in_degree") return a.in_degree - b.in_degree
    if (key === "cluster") {
      const la = labelByCluster.get(a.cluster) ?? ""
      const lb = labelByCluster.get(b.cluster) ?? ""
      return la.localeCompare(lb)
    }
    return String(a[key]).localeCompare(String(b[key]))
  }
}

export function ListView() {
  const { id } = useParams<{ id?: string }>()
  const navigate = useNavigate()
  const [tiers, setTiers] = useState<Record<Tier, boolean>>({
    atomic: true,
    composition: true,
    molecule: true,
    reaction: true,
  })
  const [statusFilters, setStatusFilters] = useState({ active: true, underReview: false })
  const [query, setQuery] = useState("")
  const [clusterFilter, setClusterFilter] = useState<string | null>(null)
  // Derived from the /list/:id route param, not its own state — the URL
  // is the single source of truth for which atom's dialog is open, so a
  // deep link, a browser-back, and a row click all go through the same
  // path instead of state and URL silently drifting apart.
  const selected = id ? (nodesById.get(id) ?? null) : null
  const [sortKey, setSortKey] = useState<SortKey>("in_degree")
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc")
  const clusterFilterLabel = clusterFilter ? labelByCluster.get(clusterFilter) : null

  function passesFilters(n: LexNode) {
    const okStatus = (n.status === "active" && statusFilters.active) || (n.status === "under-review" && statusFilters.underReview)
    if (!okStatus) return false
    if (!tiers[n.tier as Tier]) return false
    if (clusterFilter && n.cluster !== clusterFilter) return false
    const q = query.trim().toLowerCase()
    if (q && !(n.id.toLowerCase().includes(q) || n.name.toLowerCase().includes(q))) return false
    return true
  }

  function filterToCluster(clusterId: string) {
    setClusterFilter(clusterId)
    navigate("/list")
  }

  function jumpToAtom(atomId: string) {
    navigate(`/list/${atomId}`)
  }

  const rows = useMemo(() => {
    const visible = graph.nodes.filter(passesFilters)
    const sorted = [...visible].sort(compareBy(sortKey))
    if (sortDir === "desc") sorted.reverse()
    return sorted
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tiers, statusFilters, clusterFilter, query, sortKey, sortDir])

  const maxInDegree = useMemo(() => Math.max(1, ...rows.map((n) => n.in_degree)), [rows])

  // Every row used to render into a real <table> at once -- with the full
  // catalog visible and no filter narrowing it, that's 2000+ <tr>s always
  // in the DOM, which is real weight on a low-end phone (slow first paint,
  // janky scroll) and inflates every per-row axe finding by the same
  // factor. Pagination would fight the About tab's own description of this
  // view ("read straight through"), so this grows the rendered slice as
  // the user nears the bottom instead of chunking it into pages -- same
  // continuous-scroll feel, bounded DOM size.
  const BATCH = 200
  const [visibleCount, setVisibleCount] = useState(BATCH)
  useEffect(() => {
    setVisibleCount(BATCH)
  }, [rows])
  const visibleRows = rows.slice(0, visibleCount)
  const hasMore = visibleCount < rows.length
  const sentinelRef = useRef<HTMLTableRowElement | null>(null)
  useEffect(() => {
    const el = sentinelRef.current
    if (!el) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          setVisibleCount((c) => Math.min(rows.length, c + BATCH))
        }
      },
      { rootMargin: "800px" }
    )
    io.observe(el)
    return () => io.disconnect()
  }, [rows])
  // A deep link or a cross-reference jump (jumpToAtom, from inside another
  // atom's card) can land on a row past the current batch -- the dialog
  // itself still opens fine either way (selected is looked up straight off
  // the full graph, not the paginated slice), but without this the row
  // grid behind it would never actually reveal or highlight that row.
  useEffect(() => {
    if (!selected) return
    const idx = rows.findIndex((n) => n.id === selected.id)
    if (idx >= 0 && idx >= visibleCount) {
      setVisibleCount(Math.min(rows.length, idx + BATCH))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, rows])

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"))
    } else {
      setSortKey(key)
      setSortDir(key === "in_degree" ? "desc" : "asc")
    }
  }

  return (
    <div className="flex w-full flex-col text-foreground">
      {/* Visually hidden: the tab label already names this view for sighted
          users, but List/Pivot/Graph otherwise render zero headings at all,
          leaving screen-reader heading-navigation nowhere to land outside
          the About tab. */}
      <h1 className="sr-only">Atom list</h1>
      <div className="sticky top-0 z-30 flex flex-wrap items-center gap-3 border-b border-rule bg-muted px-3 py-2 font-mono text-[11px] sm:gap-4">
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
          <span className="ml-auto text-ink-faint">{rows.length} atoms visible</span>
      </div>

      <div
        className="overflow-x-auto focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-inset"
        tabIndex={0}
        role="region"
        aria-label="Atom list table, horizontally scrollable"
      >
          <table className="w-full border-collapse text-[11px]">
            <thead>
              <tr>
                {COLUMNS.map((c) => (
                  <th
                    key={c.key}
                    scope="col"
                    tabIndex={0}
                    aria-sort={sortKey === c.key ? (sortDir === "asc" ? "ascending" : "descending") : "none"}
                    onClick={() => toggleSort(c.key)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault()
                        toggleSort(c.key)
                      }
                    }}
                    className={
                      "cursor-pointer border border-rule bg-muted p-2 text-left font-semibold text-foreground select-none hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-inset " +
                      (c.className ?? "")
                    }
                  >
                    {c.label}
                    {sortKey === c.key ? (sortDir === "asc" ? " ↑" : " ↓") : ""}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {visibleRows.map((n) => (
                <tr
                  key={n.id}
                  tabIndex={0}
                  role="button"
                  aria-label={`Open ${n.name}`}
                  onClick={() => navigate(`/list/${n.id}`)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault()
                      navigate(`/list/${n.id}`)
                    }
                  }}
                  className={
                    "cursor-pointer hover:bg-primary/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-inset " +
                    (selected?.id === n.id ? "bg-primary/15" : "")
                  }
                >
                  <td className="border border-rule p-1.5 font-mono text-ink-faint">{n.id.replace(/^lex-/, "")}</td>
                  <td className="border border-rule p-1.5 text-foreground">{n.name}</td>
                  <td className="border border-rule p-1.5 text-ink-dim">{n.type_in}</td>
                  <td className="border border-rule p-1.5 text-ink-dim">{n.type_out}</td>
                  <td className="border border-rule p-1.5 text-ink-dim">{n.tier}</td>
                  <td className="max-w-0 border border-rule p-1.5 text-ink-dim">
                    <span className="flex items-center gap-1.5" title={labelByCluster.get(n.cluster) ?? n.cluster}>
                      <span
                        className="inline-block h-2 w-2 shrink-0 rounded-full"
                        style={{ backgroundColor: colorByCluster.get(n.cluster) ?? "#5a4f3a" }}
                      />
                      <span className="truncate">{labelByCluster.get(n.cluster) ?? n.cluster}</span>
                    </span>
                  </td>
                  <td className="border border-rule p-1.5">
                    <span className="flex items-center gap-1.5">
                      <span className="h-1.5 w-12 shrink-0 overflow-hidden rounded-full bg-bg-well">
                        <span
                          className="block h-full bg-primary"
                          style={{ width: `${(n.in_degree / maxInDegree) * 100}%` }}
                        />
                      </span>
                      <span className="font-mono text-ink-dim">{n.in_degree}</span>
                    </span>
                  </td>
                  <td className="border border-rule p-1.5 text-ink-dim">{n.status}</td>
                </tr>
              ))}
              {hasMore && (
                <tr ref={sentinelRef} aria-hidden="true">
                  <td colSpan={COLUMNS.length} className="border border-rule p-2 text-center text-ink-faint">
                    loading more…
                  </td>
                </tr>
              )}
            </tbody>
          </table>
      </div>

      <Dialog open={!!selected} onOpenChange={(open) => !open && navigate("/list")}>
        <DialogContent title={selected ? selected.name : "Atom detail"} description={selected?.id}>
          <DialogBody>
            {selected && <AtomCard node={selected} onClusterClick={filterToCluster} onAtomClick={jumpToAtom} />}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  )
}
