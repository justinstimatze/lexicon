import { lazy, Suspense } from "react"
import { HashRouter, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { TooltipProvider } from "@/components/ui/tooltip"

// Three.js + react-force-graph-3d is ~2.2MB gzipped on their own — real
// weight on a mobile network. Code-split so the Pivot tab (and the app
// shell generally) never pays for it, and the Graph tab pays for it once,
// in parallel with the rest of the page rather than blocking first paint.
const Graph3D = lazy(() => import("@/components/Graph3D").then((m) => ({ default: m.Graph3D })))
const PivotTable = lazy(() => import("@/components/PivotTable").then((m) => ({ default: m.PivotTable })))
const ListView = lazy(() => import("@/components/ListView").then((m) => ({ default: m.ListView })))
const ReadingOrder = lazy(() => import("@/components/ReadingOrder").then((m) => ({ default: m.ReadingOrder })))
// Split out too, same reason as the three above: it's the only thing on
// the About tab that needs graph.json, and About is part of the eager
// app shell — inlining the dataset there would defeat the split.
const AboutPreview = lazy(() => import("@/components/AboutPreview").then((m) => ({ default: m.AboutPreview })))

type Tab = "about" | "list" | "pivot" | "graph" | "reading-order"

const TAB_CLASS =
  "h-auto shrink-0 rounded-none px-3 py-2.5 font-mono text-[11px] tracking-[0.08em] text-ink-dim uppercase after:bg-primary data-active:bg-transparent data-active:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/60"

const ABOUT_NAV: { tab: Tab; label: string; desc: string }[] = [
  { tab: "list", label: "List", desc: "The plain index — sortable, searchable, read straight through." },
  { tab: "pivot", label: "Pivot", desc: "Grouped by the shape of claim each entry makes." },
  { tab: "graph", label: "Graph", desc: "Placed by strength of connection to everything else." },
  { tab: "reading-order", label: "Reading Order", desc: "A computed tour through the primary sources themselves." },
]

// GitHub Pages serves this as a static file tree with no server-side
// rewrite — a real path like /lexicon/list/lex-abc123 renders fine via
// client-side nav but 404s on a hard refresh or a pasted link, since
// there's nothing on the server to fall back to index.html. HashRouter's
// #/list/lex-abc123 never leaves the client to resolve, so deep links and
// refreshes both just work with zero extra GH Pages config.
function tabFromPath(pathname: string): Tab {
  const seg = pathname.split("/")[1] ?? ""
  return seg === "list" || seg === "pivot" || seg === "graph" || seg === "reading-order" ? seg : "about"
}

const LOADING_FALLBACK = (label: string) => (
  <div className="flex h-[70vh] w-full items-center justify-center border border-rule bg-bg-well font-mono text-xs text-ink-faint">
    loading {label}…
  </div>
)

function AboutPane({ onNav }: { onNav: (tab: Tab) => void }) {
  return (
    <div className="mx-auto flex max-w-[1200px] flex-col gap-10 md:flex-row md:items-start md:gap-20">
      <div className="md:max-w-[62ch]">
        <h1 className="wordmark font-display text-[clamp(26px,3.4vw,40px)] leading-[1.05] font-black tracking-tight text-balance">
          Patterns worth
          <br />
          <em>reaching for twice.</em>
        </h1>
        <p className="mt-4 mb-6 text-[15px] text-ink-dim">
          Named cognitive and structural moves — reasoning, strategy, narrative, power — each
          traced to a primary source.
        </p>
        <div className="space-y-3 text-[13px] text-ink-dim">
          <p>
            Each entry is an atom, or a molecule composed of a few atoms. Every claim traces to
            where it was first observed, quoted, and checked against the source — no invented
            quotes, no secondhand paraphrase presented as the author's own words.
          </p>
        </div>
        <div className="mt-6 border-t border-rule pt-6">
          <p className="mb-3 text-[11px] text-ink-faint uppercase tracking-wide">ways to read the catalog</p>
          <div className="flex flex-col gap-2">
            {ABOUT_NAV.map((n) => (
              <button
                key={n.tab}
                type="button"
                onClick={() => onNav(n.tab)}
                className="group flex items-start justify-between gap-4 rounded-sm border border-rule px-3 py-2.5 text-left transition-colors hover:border-primary/50 hover:bg-bg-well"
              >
                <span>
                  <span className="font-mono text-[11px] tracking-[0.08em] text-foreground uppercase">
                    {n.label}
                  </span>
                  <span className="mt-0.5 block text-[13px] text-ink-dim">{n.desc}</span>
                </span>
                <span className="mt-0.5 shrink-0 text-ink-faint transition-colors group-hover:text-primary">
                  →
                </span>
              </button>
            ))}
          </div>
        </div>
        <div className="mt-6 border-t border-rule pt-6">
          <p className="mb-3 text-[11px] text-ink-faint uppercase tracking-wide">or actually retain it</p>
          <a
            href="https://justinstimatze.github.io/lexicon/lexicon-anki.tsv"
            className="group flex items-start justify-between gap-4 rounded-sm border border-rule px-3 py-2.5 text-left transition-colors hover:border-primary/50 hover:bg-bg-well"
          >
            <span>
              <span className="font-mono text-[11px] tracking-[0.08em] text-foreground uppercase">Anki deck</span>
              <span className="mt-0.5 block text-[13px] text-ink-dim">
                Two cards per entry, not one — a recognition card (scenario in, pattern name out) and a
                recall card (name in, the single operational move out). Regenerated from the same source
                as the catalog itself. Import the .tsv directly.
              </span>
            </span>
            <span className="mt-0.5 shrink-0 text-ink-faint transition-colors group-hover:text-primary">→</span>
          </a>
        </div>
      </div>
      <div className="w-full md:sticky md:top-20 md:max-h-[min(520px,calc(100vh-6rem))] md:w-96 md:shrink-0 md:overflow-y-auto">
        <Suspense
          fallback={
            <div className="flex h-40 items-center justify-center rounded-md border border-rule bg-bg-well font-mono text-xs text-ink-faint">
              loading…
            </div>
          }
        >
          <AboutPreview />
        </Suspense>
      </div>
    </div>
  )
}

function AppShell() {
  const navigate = useNavigate()
  const location = useLocation()
  const tab = tabFromPath(location.pathname)

  function goTab(t: Tab) {
    navigate(t === "about" ? "/" : `/${t}`)
  }

  return (
    <TooltipProvider>
      <Tabs
        value={tab}
        onValueChange={(v) => goTab(v as Tab)}
        className="min-h-screen flex-col gap-0 bg-background"
      >
        <header className="border-b border-rule">
          <div className="mx-auto flex max-w-[1800px] flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3 sm:px-6 md:px-9">
            <span className="shrink-0 font-mono text-[11px] tracking-[0.12em] text-ink-dim uppercase">
              lexicon.elements
            </span>
            {/* min-w-0 lets this shrink below its own content width inside the
                header's flex row (the browser default is min-width:auto, i.e.
                "never smaller than my content") -- without it, five tabs at
                flex-nowrap forced the whole page into horizontal scroll on
                every view, at every phone width. overflow-x-auto then makes
                whatever doesn't fit scroll inside the tab strip itself
                instead of dragging the document with it.
                Two more things this needs, both from the same CSS rule: setting
                overflow on one axis promotes the OTHER axis's default `visible`
                to `auto` too, not left alone -- so overflow-y-hidden is explicit,
                not redundant. And the active tab's underline (tabs.tsx's `after:`
                pseudo-element) sits at bottom:-5px, genuinely outside this box's
                content area -- pb-1.5 gives it room to live inside the box
                instead of past it, which is what was creating real (if tiny)
                vertical overflow for that promoted auto to find and turn into a
                scrollbar. */}
            <TabsList
              variant="line"
              className="!h-auto min-w-0 flex-nowrap items-center gap-4 overflow-x-auto overflow-y-hidden overscroll-x-contain !pt-0 !px-0 !pb-1.5"
            >
              <TabsTrigger value="about" className={TAB_CLASS}>
                About
              </TabsTrigger>
              <TabsTrigger value="list" className={TAB_CLASS}>
                List
              </TabsTrigger>
              <TabsTrigger value="pivot" className={TAB_CLASS}>
                Pivot
              </TabsTrigger>
              <TabsTrigger value="graph" className={TAB_CLASS}>
                Graph
              </TabsTrigger>
              <TabsTrigger value="reading-order" className={TAB_CLASS}>
                Reading Order
              </TabsTrigger>
            </TabsList>
          </div>
        </header>

        <main className="mx-auto w-full max-w-[1800px] px-4 pt-5 pb-10 sm:px-6 sm:pt-7 sm:pb-16 md:px-9">
          <Routes>
            <Route path="/" element={<AboutPane onNav={goTab} />} />
            <Route
              path="/list/:id?"
              element={<Suspense fallback={LOADING_FALLBACK("list")}>{<ListView />}</Suspense>}
            />
            <Route
              path="/pivot"
              element={<Suspense fallback={LOADING_FALLBACK("pivot")}>{<PivotTable />}</Suspense>}
            />
            <Route
              path="/graph/:lens?"
              element={<Suspense fallback={LOADING_FALLBACK("graph")}>{<Graph3D />}</Suspense>}
            />
            <Route
              path="/reading-order"
              element={<Suspense fallback={LOADING_FALLBACK("reading order")}>{<ReadingOrder />}</Suspense>}
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>

        <footer className="border-t border-rule">
          <div className="mx-auto flex max-w-[1800px] items-center justify-between px-4 py-3 font-mono text-[11px] tracking-[0.04em] text-ink-faint sm:px-6 md:px-9">
            <span className="min-w-0 flex-1 truncate">
              a citation-grounded catalog of recurring patterns — reasoning, strategy, narrative, power
            </span>
            <a
              href="https://github.com/justinstimatze/lexicon"
              className="shrink-0 rounded-sm border border-rule-light px-2 py-1 text-ink-faint no-underline hover:border-primary/50 hover:text-primary"
            >
              SOURCE
            </a>
          </div>
        </footer>
      </Tabs>
    </TooltipProvider>
  )
}

function App() {
  return (
    <HashRouter>
      <AppShell />
    </HashRouter>
  )
}

export default App
