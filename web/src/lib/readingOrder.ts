// Mirrors render/cmd/lexicon/cmd_reading_order.go's roNode/roEdge/
// readingOrderOutput. `lexicon reading-order` is the source of truth —
// nodes and edges are computed from the corpus (lineage-prefix matching
// + tree-restricted adjacency reach), never hand-listed here.

export type ReadingOrderKind = "core" | "spark"
export type ReadingOrderEdgeKind = "solid" | "dashed"

export interface ReadingOrderNode {
  key: string
  title: string
  author: string
  edition: string
  kind: ReadingOrderKind
  url?: string
  // A one-line content note for a source whose author or text carries
  // documented historical/ethical baggage worth knowing before reading —
  // see the cringe-check pass, 2026-08-30. Empty for nearly every source.
  note?: string
  atom_count: number
  atom_ids: string[]
  reach: number
  total_in_degree: number
  max_in_degree: number
  tier: number
}

export type ReadingOrderDirectionSource = "scaffolds" | "related-tiebreak"

export interface ReadingOrderEdge {
  from: string
  to: string
  weight: number
  kind: ReadingOrderEdgeKind
  // Only set on "solid" edges. "scaffolds" means a real scaffolds-from
  // prerequisite exists between these two sources; "related-tiebreak"
  // means no scaffolds-from edge was found for this pair, so the
  // direction is still a proxy (see the comment on TechTree below).
  direction_source?: ReadingOrderDirectionSource
}

// A spark candidate that didn't make the tree's top-N cut by tree-restricted
// reach, but still has real standing in the corpus at large — surfaced
// separately rather than forced into the (already dense) diagram.
export interface ReadingOrderFurther {
  key: string
  title: string
  author: string
  edition: string
  url?: string
  note?: string
  atom_count: number
  reach: number
  total_in_degree: number
  max_in_degree: number
  // The tree node (a ReadingOrderNode.key) this source associates with
  // most strongly, in either citation direction — deliberately not a tree
  // edge; the frontend renders it as a loose satellite, not a solid
  // parent-child line, since the underlying signal is corpus-wide weight
  // that lost the tree-reach cut, not a confirmed prerequisite. Empty
  // when nothing in the tree cites this source at all.
  parent?: string
}

export interface ReadingOrderData {
  nodes: ReadingOrderNode[]
  edges: ReadingOrderEdge[]
  further: ReadingOrderFurther[]
}
