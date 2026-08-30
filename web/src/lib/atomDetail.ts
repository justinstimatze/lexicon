import type { LexLineage } from "@/lib/graph"

// The text-heavy per-atom fields the bundled graph.json trims out —
// see render/cmd/lexicon/cmd_export_graph.go's -details-dir. Fetched
// on demand when a card opens rather than shipped for all 1000+ atoms
// upfront; that trimmed fraction was 97% of a -full export's size.
export interface AtomDetail {
  canonical_instances?: string[]
  agent_instruction?: string
  critical_questions?: string[]
  lineage?: LexLineage[]
}

const cache = new Map<string, Promise<AtomDetail | null>>()

export function fetchAtomDetail(id: string): Promise<AtomDetail | null> {
  const cached = cache.get(id)
  if (cached) return cached
  const promise = fetch(`${import.meta.env.BASE_URL}atoms/${id}.json`)
    .then((r) => (r.ok ? (r.json() as Promise<AtomDetail>) : null))
    .catch(() => null)
  cache.set(id, promise)
  return promise
}
