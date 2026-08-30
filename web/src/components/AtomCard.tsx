import { useEffect, useState, type ReactNode } from "react"
import graphData from "@/data/graph.json"
import type { LexGraph, LexNode } from "@/lib/graph"
import { recordVisit, useTrail } from "@/lib/trail"
import { fetchAtomDetail, type AtomDetail } from "@/lib/atomDetail"

const graph = graphData as unknown as LexGraph
const nodesById = new Map(graph.nodes.map((n) => [n.id, n]))

const LEX_ID = /lex-[a-z0-9]{5}/g

// A canonical-instances entry is either a concrete example (what most of
// the list is) or a distinctness note comparing this atom to a
// neighbor ("operationally distinct from lex-X ..."). The two read as
// identical paragraph blocks today, which is a big part of why a
// dense card is hard to skim — they're different kinds of content and
// belong in visually different places, not one undifferentiated list.
const DISTINCTNESS_NOTE = /^(operationally |conceptually )?(distinct from|adjacent to)\b/i

function splitInstances(instances: string[]) {
  const examples: string[] = []
  const distinctions: string[] = []
  for (const c of instances) {
    ;(DISTINCTNESS_NOTE.test(c) ? distinctions : examples).push(c)
  }
  return { examples, distinctions }
}

function withAtomLinks(text: string, onAtomClick?: (id: string) => void) {
  if (!onAtomClick) return text
  const parts = text.split(LEX_ID)
  const ids = text.match(LEX_ID) ?? []
  if (ids.length === 0) return text
  const out: ReactNode[] = []
  parts.forEach((part, i) => {
    out.push(part)
    const id = ids[i]
    if (id) {
      const target = nodesById.get(id)
      out.push(
        target ? (
          <button
            key={i}
            type="button"
            onClick={() => onAtomClick(id)}
            title={target.name}
            className="text-primary underline decoration-dotted underline-offset-2 hover:text-accent-soft"
          >
            {id}
          </button>
        ) : (
          id
        )
      )
    }
  })
  return out
}

const TRUNCATE_AT = 220

function ExpandableInstance({
  text,
  index,
  onAtomClick,
}: {
  text: string
  index: number
  onAtomClick?: (id: string) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const needsTruncation = text.length > TRUNCATE_AT
  const shown = expanded || !needsTruncation ? text : text.slice(0, TRUNCATE_AT).slice(0, text.lastIndexOf(" ", TRUNCATE_AT)) + "…"
  return (
    <li className="flex gap-2">
      <span className="shrink-0 text-ink-faint tabular-nums">{index + 1}.</span>
      <div>
        {withAtomLinks(shown, onAtomClick)}
        {needsTruncation && (
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            className="ml-1.5 text-primary hover:text-accent-soft"
          >
            {expanded ? "less" : "more"}
          </button>
        )}
      </div>
    </li>
  )
}

export function AtomCard({
  node,
  onClusterClick,
  onAtomClick,
}: {
  node: LexNode
  onClusterClick?: (clusterId: string) => void
  onAtomClick?: (id: string) => void
}) {
  const cluster = graph.clusters.find((c) => c.id === node.cluster)
  useEffect(() => {
    recordVisit(node.id, node.name)
  }, [node.id, node.name])
  const trail = useTrail()
  const rest = trail.filter((t) => t.id !== node.id)

  const [detail, setDetail] = useState<AtomDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(true)
  useEffect(() => {
    let cancelled = false
    setDetail(null)
    setDetailLoading(true)
    fetchAtomDetail(node.id).then((d) => {
      if (!cancelled) {
        setDetail(d)
        setDetailLoading(false)
      }
    })
    return () => {
      cancelled = true
    }
  }, [node.id])

  const { examples, distinctions } = detail?.canonical_instances
    ? splitInstances(detail.canonical_instances)
    : { examples: [], distinctions: [] }

  return (
    <div>
      <div className="text-ink-faint">{node.id}</div>
      <div className="mb-1.5 text-base leading-snug font-bold text-foreground">{node.name}</div>
      <div className="flex flex-wrap items-center gap-x-1.5 text-ink-dim">
        <span>
          {node.type_in} → {node.type_out}
        </span>
        <span className="text-ink-faint">·</span>
        <span>{node.tier}</span>
        <span className="text-ink-faint">·</span>
        <span>in-degree {node.in_degree}</span>
      </div>
      {cluster &&
        (onClusterClick ? (
          <button
            type="button"
            onClick={() => onClusterClick(cluster.id)}
            title="Filter to this cluster"
            className="mt-1 flex items-start gap-1.5 text-left text-ink-dim hover:text-primary"
          >
            <span className="mt-1 inline-block h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: cluster.color }} />
            <span className="underline decoration-dotted underline-offset-2">{cluster.label}</span>
          </button>
        ) : (
          <div className="mt-1 flex items-start gap-1.5 text-left text-ink-dim">
            <span className="mt-1 inline-block h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: cluster.color }} />
            <span>{cluster.label}</span>
          </div>
        ))}
      {detailLoading && (
        <div className="mt-2.5 text-ink-faint italic">loading…</div>
      )}
      {detail?.agent_instruction && (
        <div className="mt-2.5 border-l-2 border-primary/40 pl-2 text-ink italic">
          {detail.agent_instruction}
        </div>
      )}
      {examples.length > 0 && (
        <div className="mt-3 border-t border-rule pt-2">
          <div className="mb-1 text-ink-faint uppercase tracking-wide">instances</div>
          <ul className="flex flex-col gap-2.5 text-ink-dim">
            {examples.map((c, i) => (
              <ExpandableInstance key={i} text={c} index={i} onAtomClick={onAtomClick} />
            ))}
          </ul>
        </div>
      )}
      {distinctions.length > 0 && (
        <details className="mt-3 border-t border-rule pt-2">
          <summary className="cursor-pointer text-ink-faint uppercase tracking-wide select-none hover:text-primary">
            how this differs from neighbors ({distinctions.length})
          </summary>
          <ul className="mt-2 flex flex-col gap-2 text-ink-faint">
            {distinctions.map((d, i) => (
              <li key={i}>{withAtomLinks(d, onAtomClick)}</li>
            ))}
          </ul>
        </details>
      )}
      {detail?.critical_questions && detail.critical_questions.length > 0 && (
        <details className="mt-3 border-t border-rule pt-2">
          <summary className="cursor-pointer text-ink-faint uppercase tracking-wide select-none hover:text-primary">
            check yourself ({detail.critical_questions.length})
          </summary>
          <ul className="mt-2 flex flex-col gap-2.5 text-ink-dim">
            {detail.critical_questions.map((q, i) => (
              <li key={i} className="flex gap-2">
                <span className="shrink-0 text-ink-faint tabular-nums">{i + 1}.</span>
                <span>{withAtomLinks(q, onAtomClick)}</span>
              </li>
            ))}
          </ul>
        </details>
      )}
      {detail?.lineage && detail.lineage.length > 0 && (
        <details className="mt-3 border-t border-rule pt-2">
          {/* Closed by default — it's the receipts, not the headline; open
              on demand rather than pushing the drawer's scroll length up
              for everyone who just wants the instances. */}
          <summary className="cursor-pointer text-ink-faint uppercase tracking-wide select-none hover:text-primary">
            sources ({detail.lineage.length})
          </summary>
          <ul className="mt-2 flex flex-col gap-3">
            {detail.lineage.map((l, i) => (
              <li key={i} className="border-l-2 border-rule pl-2">
                <div className="text-ink-faint uppercase tracking-wide">
                  {l.source}
                  {l.tradition ? ` · ${l.tradition}` : ""}
                </div>
                {l.citation && <div className="mt-0.5 text-ink-dim">{l.citation}</div>}
                {l.quote && (
                  <blockquote className="mt-1 text-ink-dim italic">"{l.quote}"</blockquote>
                )}
              </li>
            ))}
          </ul>
        </details>
      )}
      {rest.length > 0 && onAtomClick && (
        <div className="mt-3 border-t border-rule pt-2">
          <div className="mb-1 text-ink-faint uppercase tracking-wide">recently viewed</div>
          <div className="flex flex-wrap gap-1.5">
            {rest.slice(0, 6).map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => onAtomClick(t.id)}
                title={t.name}
                className="rounded-sm border border-rule-light px-1.5 py-0.5 text-ink-faint hover:border-primary/50 hover:text-primary"
              >
                {t.id.replace(/^lex-/, "")}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
