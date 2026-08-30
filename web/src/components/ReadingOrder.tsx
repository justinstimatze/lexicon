import { useMemo, useState } from "react"
import readingOrderData from "@/data/reading-order.json"
import type { ReadingOrderData, ReadingOrderFurther, ReadingOrderNode } from "@/lib/readingOrder"
import { TechTree } from "@/components/TechTree"
import { toggleRead, useReadingProgress } from "@/lib/readingProgress"

const data = readingOrderData as unknown as ReadingOrderData

interface EraCopy {
  label: string
  framing: string
}

// Era placement is now a MIX of two signals, and the TechTree diagram
// above draws the difference (see its legend). Where a real scaffolds-
// from prerequisite exists between two sources, an era boundary is a
// genuine "grasping A primes B" claim. Everywhere else it's still the
// old fallback: cross-reference DENSITY within this corpus's own
// `related` graph, which SCHEMA.md defines as a symmetric/undirected
// field — "type-compatible neighbors, suitable for composition" — with
// the corpus's own reciprocation lint gate enforcing that symmetry on
// nearly every edge, so that half of the tree still isn't a "read A
// before B" claim. See render/cmd/lexicon/cmd_reading_order.go for
// exactly how each edge picks its direction. Read any era in any order
// regardless. A tier's contents are generated, never hand-picked; only
// this one-line description of what the tier IS gets written by hand —
// rewritten 2026-08-30 once the scaffolds-from data below finished
// reshaping which sources land where.
const ERA_COPY: Record<number, EraCopy> = {
  0: {
    label: "Era I — Roots",
    framing:
      "Nothing here is primed or cross-referenced by anything else in this list. The Panchatantra is the deepest vein among them — start with whichever looks most interesting.",
  },
  1: {
    label: "Era II",
    framing:
      "Altshuller's TRIZ carries this era by itself. Rosch's Natural Categories is a tiny keystone here, but its reach forward is a real scaffolds-from edge — see the solid lines in the diagram above.",
  },
  2: {
    label: "Era III",
    framing:
      "Walton, Reed & Macagno's Argumentation Schemes leads this era. Mill's System of Logic sits here too, primed in from Rosch above — one of the diagram's confirmed links.",
  },
  3: {
    label: "Era IV — the deep vein",
    framing:
      "Merton's Social Theory and Social Structure alone outpaces every other source in this list. Its own confirmed prerequisites reach forward into the next era.",
  },
  4: {
    label: "Era V",
    framing:
      "The Jataka lands here — the corpus's single largest source by far, following on from Merton above via a confirmed prerequisite. Gilman's Women and Economics and Popper's Conjectures and Refutations are the other two major veins in this era.",
  },
  5: {
    label: "Era VI",
    framing:
      "The largest era: Aesop's Fables, Cushing's Zuñi Folk Tales, Radin's Trickster, and Kautilya's Arthashastra are all major veins that land here once their strongest links are followed forward.",
  },
  6: {
    label: "Era VII — the outer edge",
    framing:
      "Black Elk Speaks and Jackall's Moral Mazes are both major, largely self-contained veins that end up furthest out — heavily leaned on by earlier eras without leaning on any single one of them in turn.",
  },
}

function eraCopy(tier: number): EraCopy {
  return ERA_COPY[tier] ?? { label: `Era ${tier + 1}`, framing: "" }
}

function SourceCard({ node, isRead }: { node: ReadingOrderNode; isRead: boolean }) {
  return (
    <div
      className={
        "flex flex-col gap-1 border border-rule bg-bg-raised p-3 transition-opacity " + (isRead ? "opacity-50" : "")
      }
    >
      <div className="flex items-start justify-between gap-2">
        <label className="flex min-w-0 items-start gap-2">
          <input
            type="checkbox"
            checked={isRead}
            onChange={() => toggleRead(node.key)}
            className="mt-0.5 shrink-0 accent-primary"
            aria-label={`mark "${node.title}" as read`}
          />
          <span className={"text-[13px] font-semibold text-foreground " + (isRead ? "line-through" : "")}>
            {node.title}
          </span>
        </label>
        <span
          className={
            "shrink-0 rounded-sm px-1.5 py-0.5 font-mono text-[9px] uppercase tracking-wide " +
            (node.kind === "core" ? "bg-primary/15 text-primary" : "border border-dashed border-ink-dim text-ink-faint")
          }
        >
          {node.kind === "core" ? "core read" : "keystone"}
        </span>
      </div>
      <span className="text-[12px] text-ink-dim">
        {node.author} — {node.edition}
      </span>
      <span className="font-mono text-[10px] text-ink-faint">
        {node.atom_count} atom{node.atom_count === 1 ? "" : "s"} drawn · cited back by {node.reach} other{" "}
        {node.reach === 1 ? "source" : "sources"} in this list
      </span>
      {node.note && <span className="text-[11px] text-ink-dim italic">{node.note}</span>}
      {node.url ? (
        <a
          href={node.url}
          target="_blank"
          rel="noreferrer"
          className="mt-1 self-start text-[11px] text-primary underline decoration-dotted underline-offset-2 hover:text-accent-soft"
        >
          read it →
        </a>
      ) : (
        <span className="mt-1 text-[11px] text-ink-faint italic">no verified free or purchasable link found yet</span>
      )}
    </div>
  )
}

export function ReadingOrder() {
  const eras = useMemo(() => {
    const byTier = new Map<number, ReadingOrderNode[]>()
    for (const n of data.nodes) {
      const arr = byTier.get(n.tier) ?? []
      arr.push(n)
      byTier.set(n.tier, arr)
    }
    return [...byTier.entries()].sort((a, b) => a[0] - b[0])
  }, [])

  const coreCount = data.nodes.filter((n) => n.kind === "core").length
  const sparkCount = data.nodes.filter((n) => n.kind === "spark").length
  const progress = useReadingProgress()
  const readCount = data.nodes.filter((n) => progress.has(n.key)).length
  const [openStatus, setOpenStatus] = useState<string | null>(null)

  // Bookshop.org has no bulk "add these N books to a cart" link or API —
  // their one feature shaped like this (Book Lists) lives inside the
  // affiliate program, which this page deliberately doesn't use. Dumping
  // every unread book into its own tab was the first attempt at an
  // honest substitute; at this list's size that's 50+ tabs at once,
  // which is worse than the problem it solved. Copying a plain list is
  // the actually-honest version: one paste wherever you'd rather browse
  // them, no tab flood, no popup-blocker roulette.
  function copyUnreadLinks() {
    const unread = [...data.nodes, ...data.further].filter((n) => n.url && !progress.has(n.key))
    if (unread.length === 0) {
      setOpenStatus("Everything with a link is already checked off.")
      return
    }
    navigator.clipboard
      .writeText(unread.map((n) => `${n.title} — ${n.url}`).join("\n"))
      .then(() => setOpenStatus(`Copied ${unread.length} unread ${unread.length === 1 ? "link" : "links"} to your clipboard.`))
      .catch(() => setOpenStatus("Couldn't reach the clipboard — your browser may be blocking it."))
  }

  return (
    <div className="mx-auto flex max-w-[1400px] flex-col gap-10">
      <div className="max-w-[70ch]">
        <h1 className="font-display text-[clamp(22px,2.6vw,32px)] leading-[1.05] font-black tracking-tight text-balance">
          A reading order for the primary sources
        </h1>
        <p className="mt-3 text-[14px] text-ink-dim">
          {coreCount} pattern-dense primary texts, plus {sparkCount} shorter foundational keystones — single ideas
          whose reach into the rest of this list far exceeds how much of it they were drawn from. Selection is
          computed: which sources qualify comes from how many atoms they contributed and how many
          other sources' atoms cross-reference them. Era placement is a mix of two signals — a real "grasping this
          primes that" prerequisite where one has been confirmed, and cross-reference density everywhere else,
          since this corpus's own `related` field is symmetric — every edge points both ways. The diagram below draws the
          difference; "Era I" always means "nothing else in this list leans on it," never "skip if you're short on
          time." Start anywhere in Era I if you just want to begin. Buy the books that still have someone to pay;
          the public-domain ones link to where they're free to read well.
        </p>
        <p className="mt-2 text-[12px] text-ink-faint">
          None of these are affiliate links — nobody here makes anything if you buy through them.
        </p>
        <div className="mt-2 flex flex-wrap items-center gap-3 font-mono text-[11px] text-ink-faint">
          <span>
            {readCount} of {data.nodes.length} checked off — saved in this browser only, nowhere else.
          </span>
          <button
            type="button"
            onClick={copyUnreadLinks}
            className="border border-rule-light px-1.5 py-0.5 text-ink-dim uppercase tracking-wide hover:border-primary/60 hover:text-foreground"
          >
copy every unread link →
          </button>
        </div>
        {openStatus && <p className="mt-1 font-mono text-[11px] text-primary">{openStatus}</p>}
      </div>

      <TechTree />

      <div className="flex flex-col gap-8">
        {eras.map(([tier, nodes]) => {
          const copy = eraCopy(tier)
          return (
            <section key={tier} className="flex flex-col gap-3">
              <div className="border-b border-rule pb-2">
                <h2 className="font-mono text-[12px] tracking-[0.1em] text-foreground uppercase">{copy.label}</h2>
                {copy.framing && <p className="mt-1 text-[13px] text-ink-dim">{copy.framing}</p>}
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {nodes
                  .slice()
                  .sort((a, b) => b.atom_count - a.atom_count)
                  .map((n) => (
                    <SourceCard key={n.key} node={n} isRead={progress.has(n.key)} />
                  ))}
              </div>
            </section>
          )
        })}
      </div>

      {data.further.length > 0 && (
        <section className="flex flex-col gap-3">
          <div className="border-b border-rule pb-2">
            <h2 className="font-mono text-[12px] tracking-[0.1em] text-foreground uppercase">Further keystones</h2>
            <p className="mt-1 max-w-[70ch] text-[13px] text-ink-dim">
              These didn't make the tree above — not enough of the tree's OWN sources cite back into them to earn a
              placed, ordered slot — but the rest of the corpus leans on them plenty. Worth having on hand; just not
              worth crowding the diagram for.
            </p>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {data.further.map((n) => (
              <FurtherCard key={n.key} node={n} isRead={progress.has(n.key)} />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

function FurtherCard({ node, isRead }: { node: ReadingOrderFurther; isRead: boolean }) {
  return (
    <div
      className={
        "flex flex-col gap-1 border border-dashed border-ink-dim bg-bg-raised p-3 transition-opacity " +
        (isRead ? "opacity-50" : "")
      }
    >
      <label className="flex min-w-0 items-start gap-2">
        <input
          type="checkbox"
          checked={isRead}
          onChange={() => toggleRead(node.key)}
          className="mt-0.5 shrink-0 accent-primary"
          aria-label={`mark "${node.title}" as read`}
        />
        <span className={"text-[13px] font-semibold text-foreground " + (isRead ? "line-through" : "")}>
          {node.title}
        </span>
      </label>
      <span className="text-[12px] text-ink-dim">
        {node.author} — {node.edition}
      </span>
      <span className="font-mono text-[10px] text-ink-faint">
        {node.atom_count} atom{node.atom_count === 1 ? "" : "s"} drawn · leaned on by {node.total_in_degree} atoms
        corpus-wide
      </span>
      {node.note && <span className="text-[11px] text-ink-dim italic">{node.note}</span>}
      {node.url ? (
        <a
          href={node.url}
          target="_blank"
          rel="noreferrer"
          className="mt-1 self-start text-[11px] text-primary underline decoration-dotted underline-offset-2 hover:text-accent-soft"
        >
          read it →
        </a>
      ) : (
        <span className="mt-1 text-[11px] text-ink-faint italic">no verified free or purchasable link found yet</span>
      )}
    </div>
  )
}
