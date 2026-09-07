// No shipped types for this package. Declaring only the one export
// TraceNetwork.tsx actually uses, matching d3-force's own forceCollide
// shape (d3-force-3d is a drop-in fork, API-compatible for 2D use).
declare module "d3-force-3d" {
  export interface ForceCollide<NodeDatum> {
    (alpha: number): void
    initialize(nodes: NodeDatum[], random?: () => number): void
    radius(radius: number | ((node: NodeDatum, i: number, nodes: NodeDatum[]) => number)): ForceCollide<NodeDatum>
    radius(): (node: NodeDatum, i: number, nodes: NodeDatum[]) => number
    strength(strength: number): ForceCollide<NodeDatum>
    strength(): number
    iterations(iterations: number): ForceCollide<NodeDatum>
    iterations(): number
  }

  export function forceCollide<NodeDatum = object>(
    radius?: number | ((node: NodeDatum, i: number, nodes: NodeDatum[]) => number)
  ): ForceCollide<NodeDatum>
}
