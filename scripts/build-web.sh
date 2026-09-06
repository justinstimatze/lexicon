#!/usr/bin/env bash
# build-web.sh — regenerate the static elements pages under public/.
#
# Emits:
#   public/            — web/ SPA (Vite+React+Tailwind+shadcn) at the
#                        root: Graph (3D, zoom-driven LOD, idle tour),
#                        Pivot, and Reading Order (computed tech-tree
#                        over the corpus's pattern-densest primary
#                        sources) tabs. Landing page as of the
#                        2026-08-20 viz rebuild, Phase D — it supersedes
#                        the legacy shell as the default entry point.
#   public/shell.html  — legacy composed view (matrix + pivot tabs,
#                        persistent detail pane). Kept for its matrix
#                        Canvas2D tab, which the SPA doesn't have yet.
#   public/matrix.html — standalone adjacency-matrix view (legacy stable URL)
#   public/pivot.html  — standalone type-in × type-out pivot table (legacy
#                        URL; superseded by the SPA's Pivot tab but kept
#                        working per the CLI's own "stable legacy URL"
#                        commitment)
#   public/lexicon-anki.tsv — Anki-importable deck (two cards per atom:
#                        recognition from a scenario, recall of the
#                        agent-instruction), a static downloadable file,
#                        not part of the SPA build.
#   public/graph.json  — the full export as a standalone fetchable file,
#                        not just inlined into the SPA's JS bundle. This
#                        is what llms.txt (web/public/llms.txt, copied
#                        into public/ by the SPA build below) points
#                        agents at — a plain GET, no scraping the app.
#   public/atoms/*.json — one file per atom (canonical_instances,
#                        agent_instruction, critical_questions, lineage),
#                        fetched by the SPA's detail panel on open. The
#                        SPA's own bundled graph.json is the trimmed
#                        export (core fields only, ~270KB) — those
#                        detail fields are 97% of a -full export's size
#                        and only ever needed for the one atom currently
#                        open, so they don't belong in every visitor's
#                        first-paint bundle.
#   web/src/data/reading-order.json — bundled straight into the SPA (no
#                        standalone public/ copy, unlike graph.json —
#                        nothing outside the app needs this one yet).
#                        Nodes/tiers/edges are computed fresh from
#                        elements/*.yaml every run by `lexicon
#                        reading-order`; see that command's source for
#                        the curated source list and the reach/tiering
#                        algorithm.
#
# The legacy trio is self-contained HTML with inline elements JSON;
# the SPA inlines the same graph (`lexicon export-graph`) at its own
# build time. GitHub Actions (.github/workflows/pages.yml) runs this
# on workflow_dispatch (manual only — see that file's header for why)
# and deploys public/ to GitHub Pages.
#
# Usage: bash scripts/build-web.sh
set -euo pipefail
cd "$(dirname "$0")/.."
rm -rf public
mkdir -p public
cd render
go build -o ./lexicon ./cmd/lexicon
cd ..
render/lexicon shell  -out public/shell.html
render/lexicon matrix -out public/matrix.html
render/lexicon pivot  -out public/pivot.html
render/lexicon anki   -out public/lexicon-anki.tsv
render/lexicon export-graph -out web/src/data/graph.json -details-dir web/public/atoms
render/lexicon reading-order -out web/src/data/reading-order.json
render/lexicon document-trace -manifest documents/manifest.json -out web/src/data/document-traces.json
echo "public/shell.html:  $(wc -c < public/shell.html)  bytes (legacy composed shell)"
echo "public/matrix.html: $(wc -c < public/matrix.html) bytes (standalone matrix)"
echo "public/pivot.html:  $(wc -c < public/pivot.html)  bytes (standalone pivot)"
echo "public/lexicon-anki.tsv: $(wc -c < public/lexicon-anki.tsv) bytes (anki deck)"
echo "web/src/data/graph.json (bundled, trimmed): $(wc -c < web/src/data/graph.json) bytes"
echo "web/src/data/document-traces.json: $(wc -c < web/src/data/document-traces.json) bytes"
echo "web/public/atoms/: $(find web/public/atoms -type f | wc -l) atom detail files"

( cd web && npm ci && npm run build )
cp -r web/dist/. public/
render/lexicon export-graph -out public/graph.json -full
echo "public/ (SPA root): $(du -sh public | cut -f1)"
echo "public/graph.json:  $(wc -c < public/graph.json) bytes (full standalone export for llms.txt)"
