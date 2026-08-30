import { useSyncExternalStore } from "react"

// A browser-local breadcrumb of visited atoms — no backend, no account.
// The catalog's real use pattern is wandering (atom links to atom via
// canonical-instances cross-refs); this lets someone retrace the chain
// that got them somewhere interesting. Recorded from AtomCard itself so
// every surface that shows one (Pivot/List drawer, the About preview)
// contributes without extra wiring.

const KEY = "lexicon.trail.v1"
const MAX = 12

export interface TrailEntry {
  id: string
  name: string
  at: number
}

const listeners = new Set<() => void>()

function subscribe(callback: () => void) {
  listeners.add(callback)
  return () => listeners.delete(callback)
}

// useSyncExternalStore requires getSnapshot to return a referentially
// stable result when nothing changed — returning a freshly-JSON.parsed
// array on every call (a different reference each time even with
// identical content) makes it conclude the store changed on every
// render, which recurses straight into "Maximum update depth exceeded."
// Cache against the raw string and only reparse when it actually moves.
let cachedRaw: string | null = null
let cachedTrail: TrailEntry[] = []

export function readTrail(): TrailEntry[] {
  let raw: string | null
  try {
    raw = localStorage.getItem(KEY)
  } catch {
    raw = null
  }
  if (raw === cachedRaw) return cachedTrail
  cachedRaw = raw
  try {
    cachedTrail = raw ? (JSON.parse(raw) as TrailEntry[]) : []
  } catch {
    // Corrupt JSON — the trail is a nicety, not load-bearing, so fail
    // quiet rather than throw.
    cachedTrail = []
  }
  return cachedTrail
}

export function recordVisit(id: string, name: string) {
  try {
    const existing = readTrail().filter((e) => e.id !== id)
    const next = [{ id, name, at: Date.now() }, ...existing].slice(0, MAX)
    const raw = JSON.stringify(next)
    localStorage.setItem(KEY, raw)
    // Prime the cache directly rather than re-reading — keeps this the
    // one place a new array reference gets minted.
    cachedRaw = raw
    cachedTrail = next
    listeners.forEach((cb) => cb())
  } catch {
    // Storage disabled / quota exceeded — same as above, nothing to
    // recover, nothing to surface.
  }
}

// Not useState+useEffect: the trail lives in localStorage — an external
// store React doesn't own — and this is the API React ships for
// subscribing to exactly that. Every AtomCard mounted at once (e.g.
// Pivot's drawer and the About preview) stays in sync automatically.
export function useTrail(): TrailEntry[] {
  return useSyncExternalStore(subscribe, readTrail)
}
