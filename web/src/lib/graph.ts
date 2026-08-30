// Mirrors render/internal/viz/exporter.go's Graph/Node/Edge and
// cluster.go's ClusterMeta — the shared contract every renderer
// (Go HTML templates, this frontend) consumes. Keep field names and
// optionality in sync with the Go structs; `lexicon export-graph`
// is the source of truth.

export interface LexLineage {
  source: string
  tradition?: string
  citation?: string
  quote?: string
}

export interface LexNode {
  id: string
  name: string
  type_in: string
  type_out: string
  tier: string
  status: string
  cluster: string
  evokes?: string[]
  canonical_instances?: string[]
  agent_instruction?: string
  critical_questions?: string[]
  lineage?: LexLineage[]
  in_degree: number
  is_molecule: boolean
}

export type EdgeType = "related" | "decomposes-into"

export interface LexEdge {
  source: string
  target: string
  type: EdgeType
}

export interface ClusterMeta {
  id: string
  name: string
  label: string
  color: string
}

export type Position = [number, number, number]

export type LayoutName =
  | "cosmic_web"
  | "cluster_puffs"
  | "degree_shells"
  | "type_grid"
  | "flurry"

export interface LexGraph {
  nodes: LexNode[]
  edges: LexEdge[]
  clusters: ClusterMeta[]
  layouts?: Partial<Record<LayoutName, Record<string, Position>>>
}
