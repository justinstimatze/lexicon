import { cn } from "@/lib/utils"
import { useGraphProgress } from "@/lib/graphStore"

function mb(bytes: number): string {
  return `${(bytes / 1048576).toFixed(1)} MB`
}

// A thin bar that fills when the total is known and slides when it isn't.
export function LoadingBar({ fraction, className }: { fraction: number | null; className?: string }) {
  return (
    <div className={cn("h-[3px] w-56 overflow-hidden rounded-full bg-ink/10", className)} aria-hidden>
      {fraction === null ? (
        <div className="loading-slide h-full w-1/3 rounded-full bg-primary/80" />
      ) : (
        <div className="h-full rounded-full bg-primary transition-[width] duration-150" style={{ width: `${Math.round(fraction * 100)}%` }} />
      )}
    </div>
  )
}

// What a route shows while the catalog (graph.json) and its own code are
// on the way. The catalog is the slow part on first visit — a few MB,
// once — so this says what is being fetched and how far along it is.
export function GraphLoading({ label, compact = false }: { label: string; compact?: boolean }) {
  const p = useGraphProgress()
  const fraction = p.total && p.total > 0 ? Math.min(1, p.received / p.total) : p.done ? 1 : null
  let status: string
  if (p.error) status = `couldn't load the catalog: ${p.error}`
  else if (p.done) status = "catalog ready"
  else if (fraction !== null) status = `catalog · ${Math.round(fraction * 100)}% of ${mb(p.total!)}`
  else if (p.received > 0) status = `catalog · ${mb(p.received)} so far`
  else status = "fetching the catalog"
  return (
    <div
      role="status"
      aria-live="polite"
      className={cn(
        "flex w-full flex-col items-center justify-center gap-3 border border-rule bg-bg-well font-mono text-xs text-ink-faint",
        compact ? "h-40 rounded-md" : "h-[70vh]"
      )}
    >
      <div className="text-ink-dim">loading {label}…</div>
      <LoadingBar fraction={fraction} />
      <div className="text-[10px] tabular-nums">{status}</div>
      {!compact && !p.done && !p.error && (
        <div className="max-w-[38ch] text-center text-[10px] leading-relaxed text-ink-faint/70">
          The whole catalog comes down once per visit and is kept for every tab after that.
        </div>
      )}
    </div>
  )
}
