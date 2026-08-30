import { useEffect, useState } from "react"
import graphData from "@/data/graph.json"
import type { LexGraph } from "@/lib/graph"
import { AtomCard } from "@/components/AtomCard"
import { fetchAtomDetail, type AtomDetail } from "@/lib/atomDetail"

const graph = graphData as unknown as LexGraph
const nodesById = new Map(graph.nodes.map((n) => [n.id, n]))

// A stable pick, not a random one per load — the highest in-degree active
// atom is the one most others already point at, so it doubles as a
// reasonable "start here" flagship rather than an arbitrary example.
const flagship = [...graph.nodes]
  .filter((n) => n.status === "active")
  .sort((a, b) => b.in_degree - a.in_degree)[0]

// Hand-picked, not another in-degree sort — chosen to span registers
// (self-reflection, group power, historical lock-in) rather than cluster
// in the flagship's own neighborhood. Ids only; every field rendered
// below is read live off the node, so this can't drift from the YAML.
const alsoWorthKnowing = ["lex-5d8hm", "lex-mwgep"]
  .map((id) => nodesById.get(id))
  .filter((n) => n !== undefined)

export function AboutPreview() {
  const [details, setDetails] = useState<Record<string, AtomDetail | null>>({})
  useEffect(() => {
    let cancelled = false
    Promise.all(alsoWorthKnowing.map((n) => fetchAtomDetail(n.id))).then((results) => {
      if (cancelled) return
      const byId: Record<string, AtomDetail | null> = {}
      alsoWorthKnowing.forEach((n, i) => {
        byId[n.id] = results[i]
      })
      setDetails(byId)
    })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-md border border-rule bg-card p-4 font-mono text-[11px] text-ink">
        <div className="mb-3 text-ink-faint uppercase tracking-wide">one entry, in full</div>
        <AtomCard node={flagship} />
      </div>
      <div className="rounded-md border border-rule bg-card p-4 font-mono text-[11px] text-ink">
        <div className="mb-3 text-ink-faint uppercase tracking-wide">also worth knowing</div>
        <ul className="flex flex-col gap-3">
          {alsoWorthKnowing.map((node) => (
            <li key={node.id} className="border-t border-rule pt-3 first:border-t-0 first:pt-0">
              <div className="text-ink-faint">{node.id}</div>
              <div className="mb-1 text-sm font-semibold text-foreground">{node.name}</div>
              <div className="text-ink-dim">
                {node.type_in} → {node.type_out}
              </div>
              {details[node.id]?.agent_instruction && (
                <div className="mt-2 border-l-2 border-primary/40 pl-2 text-ink italic">
                  {details[node.id]?.agent_instruction}
                </div>
              )}
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
