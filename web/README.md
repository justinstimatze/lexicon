# web

The lexicon.elements catalog, browsable. React + Vite + Tailwind + shadcn/ui, consuming the same `viz.Graph` JSON (`lexicon export-graph`) every renderer in `render/internal/viz/` shares.

Three tabs, three ways to read the same catalog: `Graph3D.tsx` places every entry by strength of connection to everything else, `PivotTable.tsx` groups them by the shape of claim they make (type-in × type-out), `ListView.tsx` is the plain sortable index. `AtomCard.tsx` renders one entry's detail — name, agent-instruction, canonical instances, cited sources — and is shared by all three.

```
npm install
npm run dev      # dev server, hot reload
npm run build     # static bundle to dist/
```

`npm run lint` runs oxlint; `npx tsc --noEmit` type-checks. No test suite yet — the data layer is generated (`lexicon export-graph`), not hand-authored, so the higher-value check is `lexicon lint`/`db lint` on the Go side before the JSON is ever produced.
