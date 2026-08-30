import { useSyncExternalStore } from "react"

// Per-viewer "read" checkmarks for the Reading Order tab — browser-local,
// no backend, no account. Mirrors trail.ts's useSyncExternalStore pattern:
// cache against the raw string so getSnapshot stays referentially stable
// across renders where nothing changed.

const KEY = "lexicon.reading-progress.v1"

const listeners = new Set<() => void>()

function subscribe(callback: () => void) {
  listeners.add(callback)
  return () => listeners.delete(callback)
}

let cachedRaw: string | null = null
let cachedSet: Set<string> = new Set()

function readRaw(): string | null {
  try {
    return localStorage.getItem(KEY)
  } catch {
    return null
  }
}

export function readProgress(): Set<string> {
  const raw = readRaw()
  if (raw === cachedRaw) return cachedSet
  cachedRaw = raw
  try {
    cachedSet = new Set(raw ? (JSON.parse(raw) as string[]) : [])
  } catch {
    // Corrupt JSON — progress is a nicety, not load-bearing.
    cachedSet = new Set()
  }
  return cachedSet
}

export function toggleRead(key: string) {
  try {
    const next = new Set(readProgress())
    if (next.has(key)) next.delete(key)
    else next.add(key)
    const raw = JSON.stringify([...next])
    localStorage.setItem(KEY, raw)
    cachedRaw = raw
    cachedSet = next
    listeners.forEach((cb) => cb())
  } catch {
    // Storage disabled / quota exceeded — nothing to recover.
  }
}

export function useReadingProgress(): Set<string> {
  return useSyncExternalStore(subscribe, readProgress)
}
