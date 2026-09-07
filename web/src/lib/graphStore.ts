import { useSyncExternalStore } from "react"
import type { LexGraph } from "@/lib/graph"
// `?url` makes Vite emit graph.json as a hashed static asset and hand back
// its URL, instead of inlining ~3.7MB of nodes and edges into a JS chunk.
// A JSON file is fetched with progress the page can show, parsed with
// JSON.parse (several times faster than evaluating an object literal of
// the same size), and served gzipped by any static host.
import graphUrl from "@/data/graph.json?url"

export interface GraphProgress {
  received: number
  // Known only when the response is uncompressed and sized; a gzipped
  // response reports its compressed length, which would make a percent
  // lie, so total stays null and the UI shows bytes so far instead.
  total: number | null
  done: boolean
  error: string | null
}

let graph: LexGraph | null = null
let inflight: Promise<LexGraph> | null = null
let progress: GraphProgress = { received: 0, total: null, done: false, error: null }
const listeners = new Set<() => void>()

function set(next: GraphProgress) {
  progress = next
  for (const l of listeners) l()
}

// The catalog, for a module that is only ever evaluated after loadGraph()
// resolved — every route that needs it is wrapped that way in App.tsx.
// Throwing here rather than returning null keeps every consumer's
// `const graph = getGraph()` as simple as the static import it replaced.
export function getGraph(): LexGraph {
  if (!graph) throw new Error("graph.json is not loaded yet — this module must be imported after loadGraph() resolves")
  return graph
}

export function isGraphLoaded(): boolean {
  return graph !== null
}

export function loadGraph(): Promise<LexGraph> {
  if (graph) return Promise.resolve(graph)
  if (inflight) return inflight
  inflight = (async () => {
    const res = await fetch(graphUrl)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const encoded = res.headers.get("content-encoding")
    const len = res.headers.get("content-length")
    const total = !encoded && len ? Number(len) : null
    let text: string
    if (res.body) {
      const reader = res.body.getReader()
      const chunks: Uint8Array[] = []
      let received = 0
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        chunks.push(value)
        received += value.length
        set({ received, total, done: false, error: null })
      }
      const all = new Uint8Array(received)
      let off = 0
      for (const c of chunks) {
        all.set(c, off)
        off += c.length
      }
      text = new TextDecoder().decode(all)
    } else {
      text = await res.text()
    }
    graph = JSON.parse(text) as LexGraph
    set({ ...progress, total: progress.total ?? progress.received, done: true })
    return graph
  })().catch((e: unknown) => {
    set({ ...progress, error: e instanceof Error ? e.message : String(e) })
    inflight = null
    throw e
  })
  return inflight
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

export function useGraphProgress(): GraphProgress {
  return useSyncExternalStore(subscribe, () => progress, () => progress)
}
