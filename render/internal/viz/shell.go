package viz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
)

// shellTemplate composes the MatrixPanel and PivotPanel into a single
// app with three regions on desktop (tabs sidebar · active panel ·
// persistent detail pane) and a mobile layout (top bar · single
// active view · bottom tab bar). State lives in `window.LexiconShell`
// — selected atom, status filters, search query, active tab. Hash
// routing keeps deep-links (`#tab=pivot&id=lex-2sdqf`) working. The
// two panels coexist in the DOM and switch via `display: none` so
// canvas zoom / scroll / pivot filter state persists across taps.
const shellTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Lexicon — elements</title>
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<style>
  :root {
    /* cupel-family palette (github.com/justinstimatze/cupel) — warm dark sepia */
    --bg: #16130f;
    --bg-panel: #1d1812;
    --bg-card: #211c16;
    --border: #322a20;
    --border-line: #4a3f30;
    --text: #ece3d4;
    --text-soft: #d7c9b1;
    --text-mute: #9c8f7a;
    --accent: #c9a45e;
    --accent-mute: #2a2118;
    --accent-strong: #e0b56b;
    --select: #d44a3a;
    --gap: #b87a1a;
    --gap-bg: #2a2418;
    --gap-border: #4a3f20;
    --mark-bg: #c9a45e;
    --mark-fg: #16130f;
    --shell-topbar-h: 40px;
    --shell-tabs-w: 64px;
    --shell-detail-w: 380px;
    --shell-mobile-bottom-h: 56px;
  }
  * { box-sizing: border-box; }
  html, body {
    margin: 0; padding: 0;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
    color: var(--text); background: var(--bg); font-size: 13px;
    height: 100%;
    overscroll-behavior: none;
  }
  /* Themed scrollbars — sepia track, warm thumb, gold on hover. */
  * { scrollbar-color: var(--border-line) var(--bg-panel); scrollbar-width: thin; }
  *::-webkit-scrollbar { width: 11px; height: 11px; }
  *::-webkit-scrollbar-track { background: var(--bg-panel); }
  *::-webkit-scrollbar-thumb { background: var(--border-line); border: 2px solid var(--bg-panel); border-radius: 6px; }
  *::-webkit-scrollbar-thumb:hover { background: var(--accent); }
  *::-webkit-scrollbar-corner { background: var(--bg-panel); }
  /* Warm-toned text selection so highlighted text stays on-palette. */
  ::selection { background: var(--accent); color: var(--bg); }
  /* Form-control defaults — make native inputs sit inside the palette. */
  input, textarea, select, button { font-family: inherit; }
  input[type="checkbox"], input[type="radio"] { accent-color: var(--accent); }
  input[type="text"], input[type="search"], textarea, select {
    background: var(--bg-card); color: var(--text);
    border: 1px solid var(--border); border-radius: 3px;
    padding: 4px 8px; font-size: 13px;
  }
  input[type="text"]::placeholder, input[type="search"]::placeholder, textarea::placeholder { color: var(--text-mute); }
  input[type="text"]:focus, input[type="search"]:focus, textarea:focus, select:focus {
    outline: 2px solid var(--accent); outline-offset: -1px; border-color: var(--accent);
  }
  #shell {
    display: grid;
    grid-template-rows: var(--shell-topbar-h) 1fr;
    grid-template-columns: var(--shell-tabs-w) 1fr 6px var(--shell-detail-w);
    grid-template-areas:
      "topbar topbar        topbar        topbar"
      "tabs   panel  detail-handle detail";
    height: 100vh;
    height: 100dvh;
  }
  #shell-topbar {
    grid-area: topbar;
    background: var(--bg-panel); border-bottom: 1px solid var(--border);
    display: flex; align-items: center; gap: 12px; padding: 0 12px;
  }
  #shell-topbar .brand { font-weight: 600; color: var(--accent); letter-spacing: 0.02em; }
  #shell-topbar .brand .ver { color: var(--text-mute); font-weight: 400; font-size: 11px; margin-left: 6px; }
  #shell-topbar .group { display: flex; align-items: center; gap: 6px; }
  #shell-topbar .group-label { font-weight: 600; color: var(--text-mute); font-size: 12px; }
  #shell-topbar input[type="text"] { padding: 4px 8px; border: 1px solid var(--border); border-radius: 3px; font-size: 13px; width: 320px; background: var(--bg-card); color: var(--text); }
  #shell-topbar input[type="text"]::placeholder { color: var(--text-mute); }
  #shell-topbar input[type="text"]:focus { outline: 2px solid var(--accent); outline-offset: -1px; border-color: var(--accent); }
  /* Tinted checkboxes so status filters read warm rather than browser-blue. */
  #shell-topbar input[type="checkbox"], #shell-drawer input[type="checkbox"] { accent-color: var(--accent); }
  #shell-topbar label { display: inline-flex; align-items: center; gap: 3px; cursor: pointer; font-size: 12px; }
  #shell-topbar .shell-stats { color: var(--text-mute); font-size: 12px; margin-left: auto; }
  #shell-topbar .icon-btn { display: none; background: none; border: 1px solid var(--border); border-radius: 3px; padding: 4px 8px; font-size: 16px; cursor: pointer; line-height: 1; }
  #shell-tabs {
    grid-area: tabs;
    background: var(--bg-panel); border-right: 1px solid var(--border);
    display: flex; flex-direction: column; align-items: stretch; padding: 6px 0;
  }
  #shell-tabs button {
    background: none; border: 0; border-left: 3px solid transparent;
    padding: 12px 8px; cursor: pointer; color: var(--text-mute);
    font-size: 11px; text-align: center; line-height: 1.3;
    display: flex; flex-direction: column; align-items: center; gap: 4px;
    font-family: inherit;
  }
  #shell-tabs button:hover { background: var(--accent-mute); color: var(--text); }
  #shell-tabs button.active { border-left-color: var(--accent); color: var(--accent); font-weight: 600; background: var(--accent-mute); }
  #shell-tabs button .glyph { font-size: 18px; line-height: 1; }
  #shell-panel {
    grid-area: panel;
    overflow: hidden;
    position: relative;
    min-width: 0;
    min-height: 0;
  }
  #shell-panel > .panel-host { width: 100%; height: 100%; min-height: 0; min-width: 0; }
  #shell-panel > .panel-host.hidden { display: none; }
  #shell-detail {
    grid-area: detail;
    background: var(--bg-panel); border-left: 1px solid var(--border);
    overflow: auto;
    padding: 16px;
    font-size: 13px; line-height: 1.5;
  }
  #shell-detail h2 { margin: 0 0 4px 0; font-size: 15px; font-family: monospace; color: var(--accent); overflow-wrap: anywhere; }
  #shell-detail h3 { margin: 14px 0 4px 0; font-size: 11px; font-weight: 600; text-transform: uppercase; color: var(--text-mute); letter-spacing: 0.05em; }
  #shell-detail .meta { color: var(--text-mute); font-size: 12px; overflow-wrap: anywhere; }
  #shell-detail .name { color: var(--text); margin: 4px 0 8px 0; font-size: 14px; overflow-wrap: anywhere; }
  #shell-detail .ci-list p { margin: 0 0 6px 0; }
  #shell-detail .ci-list p:last-child { margin-bottom: 0; }
  #shell-detail .ci-list code { background: var(--bg-panel); color: var(--accent-strong); padding: 0 4px; border-radius: 2px; font-size: 12px; }
  #shell-detail .ci-list em { color: var(--text-mute); font-style: italic; }
  #shell-detail .ci-list strong { font-weight: 600; }
  #shell-detail .ci-list a { color: var(--accent); }
  #shell-detail ul { margin: 4px 0; padding-left: 18px; }
  #shell-detail li { margin: 4px 0; }
  #shell-detail ul.ci-list { list-style: none; padding-left: 0; margin: 6px 0; }
  #shell-detail ul.ci-list li { margin: 14px 0; padding: 8px 12px; background: var(--bg-card); border-left: 3px solid var(--accent); border-radius: 0 3px 3px 0; line-height: 1.5; color: var(--text-soft); }
  #shell-detail ul.ci-list li::first-letter { text-transform: uppercase; }
  #shell-detail ul.evokes-list { list-style: none; padding-left: 0; margin: 4px 0; display: flex; flex-wrap: wrap; gap: 4px; }
  #shell-detail ul.evokes-list li { margin: 0; padding: 2px 8px; background: var(--bg-card); border: 1px solid var(--border-line); border-radius: 12px; font-size: 11px; color: var(--text-mute); }
  #shell-detail .empty { color: var(--text-mute); font-style: italic; font-size: 12px; }
  #shell-detail .chips { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
  #shell-detail .chip {
    display: inline-flex; align-items: center; gap: 4px;
    padding: 4px 8px; border: 1px solid var(--border-line); border-radius: 3px;
    font-family: monospace; font-size: 11px; color: var(--text);
    cursor: pointer; text-decoration: none; min-height: 28px;
    transition: transform 0.05s;
  }
  #shell-detail .chip:hover { transform: translateY(-1px); border-color: var(--accent); }
  #shell-detail .chip .chip-name { font-family: inherit; color: var(--text-mute); font-size: 11px; }
  #shell-detail .legend { display: flex; flex-direction: column; gap: 4px; margin-top: 6px; font-size: 12px; }
  #shell-detail .legend .row { display: flex; align-items: center; gap: 6px; }
  #shell-detail .legend .swatch { display: inline-block; width: 14px; height: 10px; border: 1px solid var(--border-line); }
  #shell-detail .placeholder { color: var(--text-soft); padding: 0; }
  #shell-detail .placeholder h2 { margin: 0 0 8px 0; font-family: inherit !important; color: var(--text) !important; font-size: 14px !important; }
  #shell-detail .placeholder p { margin: 0 0 8px 0; line-height: 1.5; }
  #shell-detail .placeholder code { background: var(--bg-card); color: var(--accent-strong); padding: 0 4px; border-radius: 2px; font-size: 12px; }
  #shell-detail .pair-header { display: flex; align-items: center; gap: 6px; padding: 8px; margin-bottom: 10px; background: var(--gap-bg); border: 1px solid var(--gap-border); border-radius: 4px; flex-wrap: wrap; color: var(--text-soft); }
  #shell-detail .pair-header .pair-chip { display: inline-flex; align-items: center; padding: 4px 8px; border: 1px solid var(--border-line); border-radius: 3px; font-family: monospace; font-size: 11px; cursor: pointer; transition: transform 0.05s; }
  #shell-detail .pair-header .pair-chip:hover { transform: translateY(-1px); border-color: var(--accent); }
  #shell-detail .pair-header .pair-chip.primary { border: 2px solid var(--select); font-weight: 600; cursor: default; }
  #shell-detail .pair-header .pair-edge { font-family: monospace; font-size: 11px; color: var(--accent); font-style: italic; }
  #shell-detail .pair-header .pair-name { display: block; width: 100%; font-size: 11px; color: var(--text-mute); margin-top: 2px; }
  #shell-detail .chip.paired { outline: 2px solid var(--select); outline-offset: 1px; }
  #shell-detail .gap-detail h2 { color: var(--gap); }
  #shell-detail .gap-detail .gap-arrow { font-family: monospace; font-size: 13px; }
  #shell-detail ul.gap-suggestions { list-style: none; padding: 0; margin: 8px 0; }
  #shell-detail ul.gap-suggestions li { padding: 8px 10px; background: var(--gap-bg); border-left: 3px solid var(--gap-border); border-radius: 0 3px 3px 0; margin: 6px 0; line-height: 1.5; color: var(--text-soft); }
  #shell-detail ul.gap-suggestions strong { font-family: monospace; color: var(--accent); }

  #shell-search-host { overflow-y: auto; background: var(--bg-panel); display: flex; flex-direction: column; }
  #shell-search-host .search-meta { position: sticky; top: 0; background: var(--bg-panel); border-bottom: 1px solid var(--border); padding: 12px 24px; z-index: 2; font-size: 12px; color: var(--text-mute); }
  #shell-search-host .search-bar { position: sticky; top: 0; background: var(--bg-panel); border-bottom: 1px solid var(--border); padding: 14px 24px 12px 24px; z-index: 2; }
  #shell-search-host .search-bar input {
    width: 100%; padding: 10px 14px; font-size: 15px;
    border: 1px solid var(--border); border-radius: 4px;
    font-family: inherit; box-sizing: border-box;
  }
  #shell-search-host .search-bar input:focus { outline: 2px solid var(--accent); outline-offset: 0; }
  #shell-search-host .search-meta { font-size: 12px; color: var(--text-mute); margin-top: 6px; }
  #shell-search-host .search-meta kbd { font-family: monospace; background: var(--bg-card); color: var(--accent-strong); padding: 1px 4px; border-radius: 2px; font-size: 11px; border: 1px solid var(--border); }
  #shell-search-host .search-results { padding: 8px 24px 24px 24px; }
  #shell-search-host .search-row { display: flex; align-items: baseline; gap: 10px; padding: 7px 10px; margin: 2px 0; border-radius: 3px; cursor: pointer; border-left: 4px solid transparent; }
  #shell-search-host .search-row:hover { background: var(--accent-mute); }
  #shell-search-host .search-row .id { font-family: monospace; color: var(--text); font-weight: 600; flex: 0 0 auto; min-width: 72px; }
  #shell-search-host .search-row .name { flex: 1 1 auto; color: var(--text); overflow-wrap: anywhere; }
  #shell-search-host .search-row .meta { flex: 0 0 auto; font-size: 11px; color: var(--text-mute); font-family: monospace; }
  #shell-search-host .search-empty { color: var(--text-mute); font-style: italic; padding: 20px 0; text-align: center; }
  #shell-search-host mark { background: var(--mark-bg); color: var(--mark-fg); padding: 0 1px; border-radius: 1px; }
  #shell-detail mark.search-hit { background: #fff2a8; padding: 0 1px; border-radius: 1px; }
  #shell-resize-handle {
    grid-area: detail-handle;
    cursor: col-resize;
    background: transparent;
    border-left: 1px solid var(--border);
    width: 6px;
    transition: background 0.1s;
    user-select: none;
  }
  #shell-resize-handle:hover, #shell-resize-handle.dragging { background: var(--accent-mute); }

  #shell-clusters-host { overflow-y: auto; padding: 16px 24px; background: var(--bg-panel); scroll-behavior: smooth; }
  .clusters-intro { color: var(--text-mute); font-size: 13px; line-height: 1.5; max-width: 720px; margin-bottom: 22px; }
  .cluster-section { margin-bottom: 28px; scroll-margin-top: 12px; }
  .cluster-section header {
    display: flex; align-items: center; gap: 10px;
    margin: 0 0 10px 0; padding: 6px 0 6px 0;
    font-weight: 600; font-size: 16px; color: var(--text);
    border-bottom: 4px solid var(--cluster-c, var(--border));
    position: sticky; top: -16px; background: var(--bg-panel); z-index: 1;
  }
  .cluster-section .swatch { width: 18px; height: 18px; border-radius: 3px; display: inline-block; border: 1px solid rgba(0,0,0,0.4); flex: 0 0 auto; }
  .cluster-section .cluster-label { overflow-wrap: anywhere; }
  .member-chips {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: 4px;
  }
  .member-chips a.chip {
    padding: 5px 8px; border-radius: 3px; font-size: 11px; text-decoration: none;
    background: var(--bg-card); color: var(--text-soft);
    border: 1px solid var(--border); font-family: monospace; cursor: pointer;
    display: flex; align-items: baseline; gap: 6px;
    min-width: 0; overflow: hidden;
  }
  .member-chips a.chip:hover { border-color: var(--accent); color: var(--text); transform: translateY(-1px); }
  .member-chips a.chip .chip-name {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; color: var(--text-mute); font-size: 11px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1 1 auto;
  }
  /* Legend rows in the detail sidebar + drawer become clickable TOC anchors into the Clusters tab */
  #shell-detail .legend .row[data-cluster] { cursor: pointer; padding: 2px 4px; margin: -2px -4px; border-radius: 2px; }
  #shell-detail .legend .row[data-cluster]:hover { background: var(--accent-mute); }
  #shell-drawer .legend .row[data-cluster] { cursor: pointer; padding: 4px 6px; margin: -2px -4px; border-radius: 2px; }
  #shell-drawer .legend .row[data-cluster]:hover { background: var(--accent-mute); }

  #shell-tooltip {
    position: fixed; pointer-events: none; background: var(--bg-card); border: 1px solid var(--border-line);
    border-radius: 3px; padding: 4px 8px; font-size: 11px; box-shadow: 0 4px 14px rgba(0,0,0,0.45);
    opacity: 0; transition: opacity 0.06s; white-space: nowrap; max-width: 360px; z-index: 30; color: var(--text-soft);
  }
  #shell-tooltip.visible { opacity: 1; }
  #shell-tooltip .id { font-family: monospace; color: var(--text-mute); }
  #shell-tooltip .name { display: block; color: var(--text); margin-top: 2px; white-space: normal; line-height: 1.3; }
  #shell-tooltip .tag { color: var(--gap); font-style: italic; }

  #shell-bottom { display: none; }
  #shell-drawer { display: none; }

  /* Tablet: 600–899px — collapse tabs to icon-only + narrow detail */
  @media (max-width: 899px) and (min-width: 600px) {
    :root {
      --shell-tabs-w: 56px;
      --shell-detail-w: 260px;
    }
    #shell-topbar input[type="text"] { width: 140px; }
    #shell-topbar .shell-stats { display: none; }
    #shell-tabs button .label { display: none; }
  }

  /* Mobile: <600px — single-view + bottom tab bar + hamburger drawer */
  @media (max-width: 599px) {
    #shell {
      grid-template-rows: var(--shell-topbar-h) 1fr var(--shell-mobile-bottom-h);
      grid-template-columns: 1fr;
      grid-template-areas:
        "topbar"
        "panel"
        "bottom";
    }
    #shell-tabs { display: none; }
    #shell-resize-handle { display: none; }
    #shell-detail {
      grid-area: panel;
      border-left: 0;
      border-top: 0;
    }
    #shell-panel { grid-area: panel; }
    #shell.detail-active #shell-panel { display: none; }
    #shell.detail-active #shell-detail { display: block; }
    #shell:not(.detail-active) #shell-detail { display: none; }
    #shell-topbar .shell-stats,
    #shell-topbar #shell-status-group,
    #shell-topbar #shell-search { display: none; }
    #shell-topbar .icon-btn { display: inline-flex; align-items: center; justify-content: center; }
    #shell-topbar.search-open #shell-search { display: inline-flex; flex: 1 1 0; min-width: 0; }
    #shell-topbar.search-open input[type="text"] { width: 100%; }
    #shell-bottom {
      grid-area: bottom; display: flex;
      background: var(--bg-panel); border-top: 1px solid var(--border);
    }
    #shell-bottom button {
      flex: 1 1 0;
      background: none; border: 0; border-top: 3px solid transparent;
      padding: 6px 4px; cursor: pointer; color: var(--text-mute);
      font-size: 11px; font-family: inherit;
      display: flex; flex-direction: column; align-items: center; gap: 2px; line-height: 1.2;
    }
    #shell-bottom button.active { border-top-color: var(--accent); color: var(--accent); font-weight: 600; }
    #shell-bottom button .glyph { font-size: 18px; line-height: 1; }
    #shell-drawer {
      display: block;
      position: fixed; top: 0; left: 0; bottom: 0; width: 80%; max-width: 320px;
      background: var(--bg-panel); border-right: 1px solid var(--border);
      transform: translateX(-100%); transition: transform 0.18s ease-out;
      z-index: 20; padding: 16px; overflow: auto;
      box-shadow: 2px 0 12px rgba(0,0,0,0.15);
    }
    #shell-drawer.open { transform: translateX(0); }
    #shell-drawer-backdrop {
      display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.3); z-index: 19;
    }
    #shell-drawer-backdrop.open { display: block; }
    #shell-drawer h3 { margin: 12px 0 6px 0; font-size: 11px; font-weight: 600; text-transform: uppercase; color: var(--text-mute); }
    #shell-drawer label { display: block; padding: 8px 0; font-size: 14px; cursor: pointer; }
  }
{{ .MatrixCSS }}
{{ .PivotCSS }}
</style>
</head>
<body>
<div id="shell">
  <div id="shell-topbar">
    <span class="brand">lexicon <span class="ver">elements</span></span>
    <button class="icon-btn" id="shell-hamburger" aria-label="open filters">☰</button>
    <button class="icon-btn" id="shell-search-toggle" aria-label="toggle search">🔍</button>
    <div class="group" id="shell-status-group">
      <span class="group-label">status:</span>
      <label><input type="checkbox" id="shell-filter-active" checked> active</label>
      <label><input type="checkbox" id="shell-filter-under-review"> under-review</label>
    </div>
    <div class="group" id="shell-search">
      <input type="text" id="shell-search-input" placeholder="search atoms — id, name, type, evokes, instances…">
    </div>
    <div class="shell-stats" id="shell-stats"></div>
  </div>
  <div id="shell-tabs">
    <button data-tab="matrix"><span class="glyph">▦</span><span class="label">Matrix</span></button>
    <button data-tab="pivot" class="active"><span class="glyph">⊞</span><span class="label">Pivot</span></button>
    <button data-tab="clusters"><span class="glyph">⌘</span><span class="label">Clusters</span></button>
    <button data-tab="search"><span class="glyph">≡</span><span class="label">List</span></button>
  </div>
  <div id="shell-panel">
    <div class="panel-host hidden" id="shell-matrix-host">
{{ .MatrixHTML }}
    </div>
    <div class="panel-host" id="shell-pivot-host">
{{ .PivotHTML }}
    </div>
    <div class="panel-host hidden" id="shell-clusters-host"></div>
    <div class="panel-host hidden" id="shell-search-host">
      <div class="search-meta" id="shell-search-meta">All atoms. Type in the top-bar search to filter across id, name, type, tier, status, cluster, evokes, agent-instruction, and canonical-instances. Click any row to open in the detail pane.</div>
      <div class="search-results" id="shell-search-results"></div>
    </div>
  </div>
  <div id="shell-resize-handle" title="drag to resize"></div>
  <div id="shell-detail">
    <div class="placeholder" id="shell-detail-placeholder"></div>
    <div id="shell-detail-body" style="display:none"></div>
    <h3>clusters</h3>
    <div class="legend" id="shell-cluster-legend"></div>
  </div>
  <div id="shell-bottom">
    <button data-tab="matrix"><span class="glyph">▦</span>Matrix</button>
    <button data-tab="pivot" class="active"><span class="glyph">⊞</span>Pivot</button>
    <button data-tab="clusters"><span class="glyph">⌘</span>Clusters</button>
    <button data-tab="search"><span class="glyph">≡</span>List</button>
    <button data-tab="detail"><span class="glyph">ⓘ</span>Detail</button>
  </div>
  <div id="shell-drawer">
    <h3>status filter</h3>
    <label><input type="checkbox" id="drawer-filter-active" checked> active</label>
    <label><input type="checkbox" id="drawer-filter-under-review"> under-review</label>
    <h3>clusters</h3>
    <div class="legend" id="drawer-cluster-legend"></div>
  </div>
  <div id="shell-drawer-backdrop"></div>
  <div id="shell-tooltip"></div>
</div>
<script>
window.DATA = {{ .DataJSON }};
window.LexiconPivot = { ROW_ORDER: {{ .RowOrderJSON }}, COL_ORDER: {{ .ColOrderJSON }} };

window.LexiconShell = (function() {
  const state = {
    selectedAtomId: null,
    pairedAtomId: null,
    pairedEdgeType: null,
    gapCell: null,
    statusFilters: { active: true, underReview: false },
    searchQuery: '',
    activeTab: 'pivot',
  };

  const clusterById = {};
  window.DATA.clusters.forEach(c => { clusterById[c.id] = c; });

  // Suggestions for the pivot's 8 currently-empty (type-in × type-out) cells.
  // Each candidate is named at the conceptual level — what an atom that occupied
  // that cell might be called — with a one-line justification. Some cells have
  // no obvious candidate; that's recorded too.
  const gapSuggestions = {
    'question__posture': [
      { name: 'aporia-from-elenchus', why: 'Plato — Socratic question induces the perplexed-suspended stance' },
      { name: 'premortem-question-shifts-stance', why: 'Klein — "imagine this has failed" question shifts the asker from optimist to forensic posture' },
      { name: 'reductio-question-forces-disavowal', why: '"does X really imply Y?" locks the interlocutor into either rejecting X or accepting Y — the question shapes their stance' },
      { name: 'naive-question-permits-explanation', why: 'Feynman — asking the dumb question opens a stance in which expertise can be talked about without performing it' },
    ],
    'frame__process': [
      { name: 'paradigm-determines-method', why: 'Kuhnian — the frame dictates what counts as a legitimate experimental process' },
      { name: 'diagnosis-determines-treatment', why: 'medical-reasoning canonical: the diagnostic frame entails the treatment process' },
      { name: 'threat-frame-triggers-fight-flight-freeze', why: 'Cannon — appraising a stimulus as threat selects the SNS-mediated mobilization process' },
      { name: 'design-brief-shapes-search', why: 'the framing of the design problem determines which solution-search procedure makes sense' },
    ],
    'frame__frame': [
      { name: 'aspect-flip', why: 'Wittgenstein duck-rabbit — one frame transforms into another via gestalt switch' },
      { name: 'metaphor-extension', why: 'extending an existing frame as a metaphor for a new domain produces a new frame' },
      { name: 'zoom-out-reframes-political-as-structural', why: 'change of time-horizon transforms one frame to another (politics ↔ economics, mood ↔ physiology)' },
      { name: 'perspective-swap-via-role-taking', why: 'Mead — taking the role of the other generates a new interpretive frame' },
    ],
    'frame__claim': [
      { name: 'abductive-inference', why: 'Peirce — the frame ("there must be a reason") yields the best-explanation claim' },
      { name: 'theory-laden-observation', why: 'Hanson — the frame determines what claims are observable as data' },
      { name: 'framing-effect-shifts-decision', why: 'Tversky-Kahneman — the same data produces different claims when the frame (gain vs loss) shifts' },
      { name: 'analogy-transfers-claims-along-mapped-structure', why: 'analogical inference — claims from the source frame inherit into the target via the mapping' },
    ],
    'posture__state': [
      { name: 'stoic-equanimity-from-acceptance', why: 'Aurelius — amor-fati posture induces the equanimity state' },
      { name: 'embodiment-shapes-physiology', why: 'a sustained posture (slow breath, upright stance) shifts the autonomic state' },
      { name: 'anxious-posture-amplifies-threat-perception', why: 'bodily tension biases the perceived state — the world reads as more threatening' },
      { name: 'savoring-stance-extends-pleasure-state', why: 'deliberately attending to enjoyable feeling extends the subjective duration of the state' },
    ],
    'posture__process': [
      { name: 'growth-mindset-yields-iteration', why: 'Dweck — the I-can-improve posture commits you to the practice process' },
      { name: 'curiosity-yields-investigation', why: 'the curious stance licenses (and structures) the investigation process' },
      { name: 'humility-licenses-revision', why: 'the humble stance enables update-iteration; certainty forecloses it' },
      { name: 'inquiry-stance-yields-search', why: '"I don\'t know" as a stance structures the seeking process' },
    ],
    'posture__frame': [
      { name: 'standpoint-yields-perspective', why: 'standpoint-epistemology — social position (posture) shapes interpretive frame' },
      { name: 'embodied-perspective', why: 'Merleau-Ponty — the bodily stance constitutes the perceptual frame' },
      { name: 'outsider-stance-foregrounds-implicit-norms', why: 'naive-eye reveals what insiders take for granted — stance opens the frame' },
      { name: 'marginal-stance-yields-double-vision', why: 'being inside-and-outside enables a comparative frame the dominant cannot see' },
    ],
    'posture__claim': [
      { name: 'bullshit-as-truth-indifferent-stance', why: 'Frankfurt — the careless posture produces claims without truth-tracking' },
      { name: 'commitment-precedes-evidence', why: 'belief-stance generates assertions in advance of supporting evidence' },
      { name: 'principled-stance-locks-in-claims', why: 'committed advocacy produces high-conviction claims even on weak evidence' },
      { name: 'ironic-distance-undercuts-own-claims', why: 'the posture itself disclaims the claim — every assertion is held with a wink' },
    ],
    // Below: 5-cell triage sample (2026-08-20) out of 28 uncovered gap
    // cells, picked to demonstrate the triage bar (a real, exactly-fitting
    // canonical source, not a strained post-hoc rationalization — the
    // type-signature grid is coarse enough that almost any cell CAN be
    // rationalized, which is not the same as being a genuine gap) rather
    // than to cover the grid. 23 uncovered cells remain untriaged.
    'state__question': [
      { name: 'felt-information-gap-generates-question-asking', why: "Loewenstein 1994 — the felt state of an information gap is itself the mechanism that produces question-asking behavior; distinct from lex-dvydj's uncertainty-reduction *process* (that cell's output is the reduction process, this one's is the question artifact itself)" },
      { name: 'anomaly-state-prompts-explanatory-question', why: 'Kuhnian anomaly — noticing an anomalous state ("this doesn\'t fit") is the trigger that produces a why-question, prior to and separate from any investigation process' },
    ],
    'process__typology': [
      { name: 'ideal-type-construction-yields-typology', why: "Weber's method — selective accentuation of features across cases (a comparative-abstraction process) is how an ideal-type typology gets constructed, not discovered ready-made" },
      { name: 'free-listing-elicitation-yields-folk-taxonomy', why: "cognitive anthropology (D'Andrade) — a structured elicitation procedure run across informants is the process that produces a folk-taxonomy as its output" },
    ],
    'question__warning': [
      { name: 'loaded-question-triggers-presupposition-warning', why: 'the classical complex-question/loaded-question fallacy — a question that smuggles an unproven premise is the input, and the correct response is a warning flagging the presupposition rather than answering directly' },
      { name: 'leading-question-in-testimony-triggers-reliability-warning', why: "eyewitness-testimony research (Loftus) — a question's leading form, independent of its content, is what should trigger a reliability warning about the answer it elicits" },
    ],
    'composition__claim': [
      { name: 'assembled-scale-produces-emergent-claim', why: '"More Is Different" (Anderson 1972) — a composition of many simple assembled parts, at sufficient scale, yields a claim not derivable from any single part\'s properties; the elements already has this idea from the claim-in side (lex-eybad), this is the composition-as-input framing specifically' },
      { name: 'gestalt-whole-yields-claim-parts-cant', why: "Wertheimer — a perceptual composition (the assembled whole) supports a claim about its own organization that no enumeration of its parts individually would license" },
    ],
    'structure__posture': [
      { name: 'panoptic-structure-induces-self-disciplining-posture', why: "Foucault, Discipline and Punish — the architectural structure of visibility (real or merely possible observation) produces the self-monitoring posture in the observed, independent of whether anyone is actually watching" },
      { name: 'built-environment-shapes-behavioral-stance', why: 'Churchill\'s dictum ("we shape our buildings, and afterwards our buildings shape us") — a structural/spatial configuration is the mechanism, the posture it produces is the dependent variable' },
    ],
    // Below: the remaining 23 cells from the same 2026-08-20 triage pass,
    // same bar (an exactly-fitting canonical source, checked against the
    // nearest existing elements atom by type-signature, not just by
    // shared author/topic). All 31 empty cells are now triaged.
    'state__structure': [
      { name: 'far-from-equilibrium-state-organizes-into-dissipative-structure', why: "Prigogine — a sustained non-equilibrium state (energy/matter flux) is the mechanism that produces an ordered spatial or temporal structure (convection cells, chemical oscillators), not an external designer" },
      { name: 'supersaturated-state-precipitates-lattice-structure', why: 'basic crystallization chemistry — a state past its solubility threshold is the direct mechanism producing a specific crystal-lattice structure, no equilibrium-thermodynamics apparatus required' },
    ],
    'situation__composition': [
      { name: 'messy-situation-decomposes-into-interacting-sub-system-composition', why: "Checkland's Soft Systems Methodology — a real-world situation too ill-defined to solve directly is rendered tractable by representing it as a composition of interacting sub-systems" },
      { name: 'a-mess-is-a-system-of-interacting-problems', why: "Ackoff — a situation of many entangled difficulties resolves into a composition once its component problems and their interactions are made explicit, rather than being solved problem-by-problem" },
    ],
    'process__question': [
      { name: 'iterated-why-process-generates-successive-questions', why: "Toyota's Five Whys — the procedure itself, not any single answer, is what generates the next diagnostic question, terminating only when a process step stops producing a new one" },
      { name: 'bisection-debugging-process-emits-a-question-per-step', why: "binary-search debugging (e.g. git bisect) — each step of the process is structured as a single yes/no diagnostic question narrowing the search space, the question is the process's per-step output" },
    ],
    'question__composition': [
      { name: 'posed-question-decomposes-into-sub-problem-composition', why: "Pólya, How to Solve It — a problem stated as a question is worked by decomposing it into a composition of more tractable sub-problems, then re-assembling their solutions" },
      { name: 'requirement-question-decomposes-into-functional-composition', why: 'systems-engineering functional decomposition — a top-level "what must this do" question is answered by decomposing it into a composition of discrete sub-requirements' },
    ],
    'question__typology': [
      { name: 'dichotomous-key-question-sequence-partitions-into-typology', why: "field-biology identification keys — a fixed sequence of yes/no questions is the mechanism that sorts specimens into a typology; close neighbor lex-ts4qp (diairesis, question→process) is the same recursive-bisection idea aimed at a definition rather than a classification output" },
      { name: 'optimal-question-sequence-partitions-search-space', why: "Shannon-style twenty-questions — an information-theoretically optimal sequence of binary questions is what produces a classification tree of the domain" },
    ],
    'frame__question': [
      { name: 'how-might-we-reframing-generates-investigation-questions', why: '"How Might We" (IDEO) — deliberately recasting a problem into this specific frame is what generates the divergent-search questions a design team investigates next' },
      { name: 'legal-characterization-frame-determines-discovery-questions', why: 'characterizing a dispute under one legal frame rather than another (contract vs. tort) is what determines which discovery and precedent questions become relevant to ask' },
    ],
    'frame__warning': [
      { name: 'detecting-a-loaded-frame-triggers-persuasion-warning', why: "Lakoff — noticing that a debate has already been framed a specific way (e.g. 'tax relief' presupposes taxation is an affliction) is itself the mechanism that should trigger a warning that persuasive work is happening invisibly" },
      { name: 'named-propaganda-device-triggers-recognition-warning', why: "the Institute for Propaganda Analysis's classic device checklist (bandwagon, glittering generalities, etc.) — recognizing a rhetorical frame by name is the warning-generating mechanism, prior to evaluating its content" },
    ],
    'claim__structure': [
      { name: 'shared-derived-character-claims-assemble-into-cladogram-structure', why: "Hennig's cladistics — claims of shared derived characters across taxa are the input, and systematizing them produces a branching-tree structure (the cladogram) as output; near neighbor lex-eadyj (claim→frame) fossilizes descent into an interpretive frame rather than a structural diagram" },
      { name: 'case-holdings-synthesize-into-doctrinal-structure', why: 'common-law doctrinal synthesis — a body of individual case-claims (holdings), once systematized by commentators, is what produces the structure of a legal doctrine' },
    ],
    'posture__question': [
      { name: 'methodological-doubt-posture-generates-systematic-questioning', why: "Descartes' Meditations — deliberately sustaining the posture of doubt toward everything is the mechanism that produces the systematic sequence of questions, not any single doubted belief" },
      { name: 'professional-skepticism-posture-generates-audit-questions', why: 'auditing standards (ISA 200 "professional skepticism") — a formally required skeptical stance is what an auditor is expected to hold as the generative mechanism for probing questions, independent of any specific red flag' },
    ],
    'posture__composition': [
      { name: 'improvisational-stance-assembles-real-time-composition', why: "jazz improvisation (Berliner, Thinking in Jazz) — the performer's real-time, uncertainty-tolerant stance is the generative mechanism that assembles the composition in the moment, not a pre-written score" },
      { name: 'adaptive-command-posture-assembles-ad-hoc-unit-composition', why: "Boyd's maneuver-warfare doctrine — a command posture built for rapid adaptation is what assembles ad hoc force compositions on the fly, rather than executing a fixed order of battle" },
    ],
    'posture__structure': [
      { name: 'defensive-posture-produces-alliance-structure', why: 'the security dilemma (Herz 1950, Jervis 1978) — a state\'s defensive posture (arming for its own security) is the mechanism that produces alliance and balance-of-power structure across a system of states' },
      { name: 'founding-posture-toward-power-produces-constitutional-structure', why: "Arendt, On Revolution — the founders' own stance toward the power they hold is what determines the constitutional structure (checks, balances) that gets built, not the other way around" },
    ],
    'posture__typology': [
      { name: 'splitter-lumper-disposition-determines-taxonomy-produced', why: "Mayr's systematics terminology — the same specimen data, classified by a splitting vs. lumping disposition, yields a different typology; the posture is upstream of the typology, not a comment on it" },
      { name: 'clinical-vs-actuarial-posture-determines-diagnostic-typology', why: "Meehl 1954 — a practitioner's clinical-judgment vs. statistical-prediction stance determines which diagnostic typology they end up applying to the same case data" },
    ],
    'posture__warning': [
      { name: 'institutionalized-dissent-posture-produces-warning', why: "Janis's groupthink research — a deliberately assigned devil's-advocate posture is the mechanism designed to produce a warning that would otherwise be suppressed by group cohesion pressure" },
      { name: 'surveillance-posture-produces-anomaly-warning', why: 'immunological surveillance — a constant background-monitoring posture (not a triggered response) is the mechanism that produces the inflammatory warning signal once something anomalous is detected' },
    ],
    'composition__state': [
      { name: 'team-composition-determines-group-state', why: "group-diversity/faultline research (Lau & Murnighan 1998) — a team's demographic and skill composition is a direct predictor of its resulting state (cohesion, psychological safety), independent of any single member" },
      { name: 'alloy-composition-determines-material-state', why: 'basic metallurgy — the specific composition of an alloy (e.g. carbon content in steel) is the direct mechanism determining its resulting physical state (brittle vs. ductile)' },
    ],
    'composition__process': [
      { name: 'assembled-composition-runs-as-emergent-process', why: "Herbert Simon, The Architecture of Complexity — a hierarchically composed system's higher-level process behavior emerges once its components are correctly assembled (near-decomposability), not from any single component" },
      { name: 'enzyme-composition-determines-active-metabolic-pathway', why: 'biochemistry — the specific composition of enzymes present in a cell is what determines which metabolic process (pathway) actually runs' },
    ],
    'composition__question': [
      { name: 'fragment-composition-generates-reconstructive-question', why: "Cuvier's principle of the correlation of parts — an assembled composition of skeletal fragments is what generates the reconstructive question (what animal produced this), not any single fragment" },
      { name: 'excavated-assemblage-generates-site-function-question', why: 'archaeological method — a composition of artifacts recovered together (an assemblage) is what generates the question of the site\'s function or dating' },
    ],
    'composition__structure': [
      { name: 'atomic-composition-determines-crystal-structure', why: "crystallography — the specific composition and arrangement of atoms is the direct mechanism determining the resulting lattice structure (unit cell, symmetry group)" },
      { name: 'material-composition-determines-load-bearing-structure', why: "structural engineering — the composition of a building's material elements (walls, trusses) is what determines its engineered structure, at a different scale than the crystallography candidate above" },
    ],
    'composition__warning': [
      { name: 'drug-composition-triggers-interaction-warning', why: 'pharmacology — a specific combination (composition) of substances triggers a contraindication warning that neither substance alone would produce' },
      { name: 'incompatible-chemical-composition-triggers-safety-warning', why: 'industrial chemical safety (e.g. bleach + ammonia) — the same combination-triggers-warning logic as the drug-interaction candidate, in a different domain' },
    ],
    'structure__state': [
      { name: 'spatial-structure-determines-physiological-recovery-state', why: "Ulrich 1984 (Science) — a hospital room's physical structure (window view onto nature vs. a wall) is the mechanism producing a measurably different physiological recovery state in patients" },
      { name: 'room-acoustic-structure-determines-intelligibility-state', why: "architectural acoustics — a room's physical structure (reverberation time, materials) directly determines the resulting state of speech intelligibility and listener stress" },
    ],
    'structure__question': [
      { name: 'classification-structure-gaps-generate-existence-question', why: "Mendeleev's periodic table — an empty cell in a classificatory structure is the mechanism generating a specific existence question (is there an element with this atomic weight); near neighbor lex-3d2he (state→state) treats the same mechanism as a state-to-state prediction rather than a structure-to-question move" },
      { name: 'space-group-enumeration-gaps-generate-existence-question', why: 'crystallography\'s 230 space groups — historically, gaps in the systematic enumeration of possible crystal structures generated the question of whether physical crystals with those specific symmetries actually existed' },
    ],
    'structure__structure': [
      { name: 'accommodation-transforms-cognitive-structure-into-new-structure', why: "Piaget — an existing cognitive structure (schema), when it fails to fit new experience, is transformed via accommodation into a new structure, not discarded and rebuilt from scratch" },
      { name: 'transformation-rules-map-deep-structure-to-surface-structure', why: "Chomsky's transformational grammar — a deep syntactic structure is mapped by transformation rules onto a distinct surface structure; ties to the linguistics-foundations cluster (lex-d6f8b/1113/1114)" },
    ],
    'structure__typology': [
      { name: 'comparative-grammatical-structure-yields-language-typology', why: "Greenberg's linguistic universals — comparing grammatical structures (word order) across languages is the mechanism that produces a typology of language types (SVO/SOV/VSO)" },
      { name: 'comparative-floor-plan-structure-yields-building-typology', why: 'architectural history — comparing building structures (floor plans) across a corpus is how architectural historians construct a building typology (e.g. the Palladian villa type)' },
    ],
    'structure__warning': [
      { name: 'tight-coupling-structure-produces-systemic-risk-warning', why: "Perrow's Normal Accident Theory — a system's structural property of tight coupling plus interactive complexity is the diagnostic input that produces a risk warning, independent of any single component's reliability" },
      { name: 'crack-pattern-structure-triggers-safety-warning', why: 'structural engineering inspection — a literal structural signature (crack pattern, deflection) is the direct diagnostic input engineers use to issue a safety warning or condemnation notice' },
    ],
  };

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c]);
  }

  // Tiny inline markdown renderer. Elements canonical-instances and
  // similar prose entries use a small subset: backticks for code,
  // **bold**, *italic*, [text](http-link), paragraph breaks on blank
  // lines. Escapes first so raw HTML in source stays literal.
  const BT = String.fromCharCode(96);
  const reCode = new RegExp(BT + '([^' + BT + ']+)' + BT, 'g');
  function renderInlineMd(s) {
    let out = String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c]);
    out = out.replace(reCode, '<code>$1</code>');
    out = out.replace(/\[([^\]]+)\]\((https?:\/\/[^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
    out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    out = out.replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g, '$1<em>$2</em>');
    out = out.replace(/\n\n+/g, '</p><p>');
    out = out.replace(/\n/g, '<br>');
    return '<p>' + out + '</p>';
  }

  function isMobile() {
    return window.matchMedia('(max-width: 599px)').matches;
  }

  function setTab(name) {
    if (name === state.activeTab) return;
    state.activeTab = name;
    // Refresh the help text in the detail sidebar (visible only when no
    // atom is selected). Cheap; runs even when a body is displayed —
    // harmless because the placeholder is hidden in that case.
    if (typeof paintPlaceholder === 'function') paintPlaceholder();
    document.querySelectorAll('#shell-tabs button, #shell-bottom button').forEach(b => {
      b.classList.toggle('active', b.getAttribute('data-tab') === name);
    });
    const matrixHost   = document.getElementById('shell-matrix-host');
    const pivotHost    = document.getElementById('shell-pivot-host');
    const clustersHost = document.getElementById('shell-clusters-host');
    const searchHost   = document.getElementById('shell-search-host');
    matrixHost.classList.toggle('hidden', name !== 'matrix');
    pivotHost.classList.toggle('hidden',  name !== 'pivot');
    clustersHost.classList.toggle('hidden', name !== 'clusters');
    searchHost.classList.toggle('hidden', name !== 'search');
    document.getElementById('shell').classList.toggle('detail-active', name === 'detail');
    updateHash();
    // Re-fit matrix on tab switch — the panel may have just been
    // revealed and only now has non-zero dimensions.
    if (name === 'matrix' && window.MatrixPanel && window.MatrixPanel.fit) {
      requestAnimationFrame(() => window.MatrixPanel.fit());
    }
    // Lazy-render clusters on first open — avoid blocking init.
    if (name === 'clusters' && !clustersPanelRendered) {
      requestAnimationFrame(renderClustersPanel);
    }
    // Lazy-render list panel on first open + render with current query.
    if (name === 'search') {
      renderSearchPanel(state.searchQuery || '');
      // Focus the top-bar search so the user can start typing immediately.
      requestAnimationFrame(() => {
        const q = document.getElementById('shell-search-input');
        if (q) q.focus();
      });
    }
  }

  function selectAtom(id) {
    if (id === state.selectedAtomId && !state.pairedAtomId && !state.gapCell) return;
    state.selectedAtomId = id;
    state.pairedAtomId = null;
    state.pairedEdgeType = null;
    state.gapCell = null;
    renderDetail();
    if (window.MatrixPanel) window.MatrixPanel.onSelectionChanged(id, null);
    if (window.PivotPanel)  window.PivotPanel.onSelectionChanged(id);
    if (isMobile() && id) setTab('detail');
    updateHash();
  }

  function selectPair(rowId, colId, edgeType) {
    state.selectedAtomId = colId;
    state.pairedAtomId = rowId;
    state.pairedEdgeType = edgeType || null;
    state.gapCell = null;
    renderDetail();
    if (window.MatrixPanel) window.MatrixPanel.onSelectionChanged(colId, rowId);
    if (window.PivotPanel)  window.PivotPanel.onSelectionChanged(colId);
    if (isMobile()) setTab('detail');
    updateHash();
  }

  function selectGapCell(row, col) {
    state.selectedAtomId = null;
    state.pairedAtomId = null;
    state.pairedEdgeType = null;
    state.gapCell = { row: row, col: col };
    renderDetail();
    if (isMobile()) setTab('detail');
    // gap cells aren't hash-routable (transient inspect state); leave hash alone
  }

  // Fast hover tooltip used by both pivot and (future-style) any element
  // exposing rich hover info. Matrix has its own canvas-local tooltip.
  const tip = document.getElementById('shell-tooltip');
  function showTip(html, ev) {
    if (!html) return hideTip();
    tip.innerHTML = html;
    tip.classList.add('visible');
    positionTip(ev);
  }
  function positionTip(ev) {
    // Keep tooltip inside viewport — flip to upper-left when near right/bottom edges.
    const vw = window.innerWidth, vh = window.innerHeight;
    const rect = tip.getBoundingClientRect();
    let x = ev.clientX + 12;
    let y = ev.clientY + 12;
    if (x + rect.width  > vw - 8) x = ev.clientX - rect.width  - 12;
    if (y + rect.height > vh - 8) y = ev.clientY - rect.height - 12;
    tip.style.left = Math.max(4, x) + 'px';
    tip.style.top  = Math.max(4, y) + 'px';
  }
  function hideTip() { tip.classList.remove('visible'); }

  function applyFilters() {
    if (window.MatrixPanel) window.MatrixPanel.onFiltersChanged();
    if (window.PivotPanel)  window.PivotPanel.onFiltersChanged();
    updateStats();
  }

  function updateStats() {
    const f = state.statusFilters;
    const visibleNodes = window.DATA.nodes.filter(x =>
      (x.status === 'active' && f.active) || (x.status === 'under-review' && f.underReview)
    );
    // Cluster count respects the filter — a cluster with zero visible
    // atoms no longer counts toward the displayed total.
    const visibleClusters = new Set();
    visibleNodes.forEach(n => { if (n.cluster) visibleClusters.add(n.cluster); });
    document.getElementById('shell-stats').textContent = visibleNodes.length + ' atoms · ' + visibleClusters.size + ' clusters';
  }

  function relatedFor(id) {
    const out = [];
    window.DATA.edges.forEach(e => {
      if (e.source === id) out.push({id: e.target, type: e.type});
    });
    return out;
  }

  // Tab-aware help shown in the detail sidebar when no atom is selected.
  // The matrix/pivot panels no longer carry their own intro banners —
  // their text lives here, where the wider detail pane can render it
  // without truncation and where it doesn't compete with the panel itself
  // for vertical real estate.
  const TAB_INTROS = {
    matrix: '<h2 style="font-size:14px;font-family:inherit;color:var(--text);">The elements, as an adjacency matrix</h2>'
      + '<p>Each row and column is one atom; a shaded cell at (row, col) means the row atom links to the column atom. Atoms are sorted by cluster, then by in-degree, so cluster blocks emerge as dense squares on the diagonal and cross-cluster relations show as off-diagonal hotspots.</p>'
      + '<p style="margin-top:8px;">Hover for atom names; click any cell to open the relationship here.</p>',
    pivot: '<h2 style="font-size:14px;font-family:inherit;color:var(--text);">The elements, as a pivot table</h2>'
      + '<p>Atoms placed in a grid by their <code>type-in</code> (what the pattern takes as input) × <code>type-out</code> (what it produces). Each tile is one atom; tile color = its data-derived cluster (community detection on the related[] graph). The number is the cell count.</p>'
      + '<p style="margin-top:8px;">Cells marked <i>gap</i> are structural slots in the cartesian product that no primitive has been minted for yet — the gap-finding affordance. Click any gap cell to see candidate atoms that might fit; click any tile to open the atom here.</p>',
    clusters: '<h2 style="font-size:14px;font-family:inherit;color:var(--text);">Clusters tab</h2>'
      + '<p>Each cluster is a community detected by greedy modularity over the related[] + decomposes-into edge graph — atoms that share more edges with each other than with outside atoms. Color encodes cluster identity. Sorted by community size descending; the hub atom is the highest-degree member.</p>'
      + '<p style="margin-top:8px;">Click any atom chip to open it here. The cluster legend in this sidebar is also clickable; it jumps to a section in the Clusters tab.</p>',
    search: '<h2 style="font-size:14px;font-family:inherit;color:var(--text);">List / Search tab</h2>'
      + '<p>All atoms in lex-id order. Type in the top-bar search to filter across id, name, type, tier, status, cluster, evokes, agent-instruction, and canonical-instances — multi-token AND, debounced at ~60ms. Matched terms are highlighted in the rows and here in the detail pane when an atom is open.</p>'
      + '<p style="margin-top:8px;">Click any row to open the atom here.</p>',
    detail: '<p>Pick an atom from the Matrix, Pivot, Clusters, or List tab to see its details here.</p>',
  };
  function paintPlaceholder() {
    const placeholder = document.getElementById('shell-detail-placeholder');
    if (!placeholder) return;
    placeholder.innerHTML = TAB_INTROS[state.activeTab] || TAB_INTROS.detail;
  }

  function renderDetail() {
    const placeholder = document.getElementById('shell-detail-placeholder');
    const body = document.getElementById('shell-detail-body');
    if (state.gapCell) {
      placeholder.style.display = 'none';
      body.style.display = '';
      renderGapDetail(state.gapCell, body);
      return;
    }
    const id = state.selectedAtomId;
    if (!id) {
      paintPlaceholder();
      placeholder.style.display = '';
      body.style.display = 'none';
      body.innerHTML = '';
      return;
    }
    const n = window.DATA.nodes.find(x => x.id === id);
    if (!n) { placeholder.style.display = ''; body.style.display = 'none'; return; }
    placeholder.style.display = 'none';
    body.style.display = '';

    const cluster = clusterById[n.cluster] || {color: '#eee', label: n.cluster};
    const evokes = (n.evokes && n.evokes.length) ? '<ul class="evokes-list">' + n.evokes.map(e => '<li>' + escapeHtml(e) + '</li>').join('') + '</ul>' : '<p class="empty">none</p>';
    const ci = (n.canonical_instances && n.canonical_instances.length) ? '<ul class="ci-list">' + n.canonical_instances.map(e => '<li>' + renderInlineMd(e) + '</li>').join('') + '</ul>' : '<p class="empty">none</p>';

    const rel = relatedFor(id);
    let relHTML = '';
    if (rel.length > 0) {
      relHTML = '<div class="chips">';
      rel.forEach(r => {
        const target = window.DATA.nodes.find(x => x.id === r.id);
        const targetCluster = target ? (clusterById[target.cluster] || {color: '#5a4f3a'}) : {color: '#5a4f3a'};
        const targetName = target ? target.name : r.id;
        const pairedCls = (r.id === state.pairedAtomId) ? ' paired' : '';
        relHTML += '<a class="chip' + pairedCls + '" href="#' + encodeURIComponent(r.id) + '" data-id="' + escapeHtml(r.id) + '" style="background:' + targetCluster.color + '" title="' + escapeHtml(targetName) + '">';
        relHTML += escapeHtml(r.id.replace(/^lex-/, ''));
        if (r.type === 'decomposes-into') relHTML += ' <span style="font-size:9px;color:var(--bg)">↘</span>';
        relHTML += '</a>';
      });
      relHTML += '</div>';
    } else {
      relHTML = '<p class="empty">none</p>';
    }

    // Pair header — shown when the selection came from a matrix cell
    // click between two distinct atoms. Primary chip = current detail
    // body atom; swap chip = the other atom, click swaps them. Edge
    // type sits between them.
    let pairHTML = '';
    if (state.pairedAtomId) {
      const other = window.DATA.nodes.find(x => x.id === state.pairedAtomId);
      const otherCluster = other ? (clusterById[other.cluster] || {color: '#5a4f3a'}) : {color: '#5a4f3a'};
      const edgeLabel = state.pairedEdgeType || 'no edge';
      pairHTML = '<div class="pair-header">'
        + '<span class="pair-chip primary" style="background:' + (clusterById[n.cluster] || {color: '#5a4f3a'}).color + '">' + escapeHtml(n.id) + '</span>'
        + '<span class="pair-edge">' + escapeHtml(edgeLabel) + '</span>'
        + '<span class="pair-chip swap" data-id="' + escapeHtml(state.pairedAtomId) + '" style="background:' + otherCluster.color + '" title="' + escapeHtml(other ? other.name : state.pairedAtomId) + '">' + escapeHtml(state.pairedAtomId) + '</span>'
        + '<span class="pair-name">' + escapeHtml(other ? other.name : state.pairedAtomId) + '</span>'
        + '</div>';
    }

    body.innerHTML = pairHTML
      + '<h2>' + escapeHtml(n.id) + '</h2>'
      + '<div class="name">' + escapeHtml(n.name) + '</div>'
      + '<div class="meta">' + escapeHtml(n.type_in) + ' → ' + escapeHtml(n.type_out) + ' · ' + escapeHtml(n.tier) + ' · <strong>' + escapeHtml(n.status) + '</strong></div>'
      + '<div class="meta" style="display:flex;align-items:center;gap:6px;margin-top:4px">'
      +   '<span class="legend"><span class="row"><span class="swatch" style="background:' + cluster.color + '"></span></span></span>'
      +   '<span>' + escapeHtml(cluster.label) + '</span>'
      + '</div>'
      + '<div class="meta">in-degree ' + n.in_degree + (n.is_molecule ? ' · MOLECULE' : '') + '</div>'
      + '<h3>evokes</h3>' + evokes
      + '<h3>canonical instances</h3>' + ci
      + '<h3>related (' + rel.length + ')</h3>' + relHTML;

    body.querySelectorAll('a.chip').forEach(a => {
      a.addEventListener('click', (e) => {
        e.preventDefault();
        const targetId = a.getAttribute('data-id');
        if (targetId) selectAtom(targetId);
      });
    });
    const swap = body.querySelector('.pair-chip.swap');
    if (swap) {
      swap.addEventListener('click', () => {
        const other = swap.getAttribute('data-id');
        if (other) selectPair(state.selectedAtomId, other, state.pairedEdgeType);
      });
    }
    // Reset the detail pane's scroll on every render — the user just
    // navigated to a new atom; preserving scroll from the previous one
    // is rarely what they want.
    const pane = document.getElementById('shell-detail');
    if (pane) pane.scrollTop = 0;
    // Apply search-term highlights if a query is active.
    applyDetailHighlights();
  }

  // Walk the detail body's text nodes and wrap matches of the current
  // search query in <mark>. Skips text inside script/style and any
  // node that's already inside a <mark>. Cheap to run on every render.
  function applyDetailHighlights() {
    const body = document.getElementById('shell-detail-body');
    if (!body) return;
    const q = (state.searchQuery || '').trim().toLowerCase();
    // Always strip prior highlights first (in case the query changed).
    body.querySelectorAll('mark.search-hit').forEach(m => {
      const t = document.createTextNode(m.textContent);
      m.parentNode.replaceChild(t, m);
    });
    if (!q) { body.normalize(); return; }
    const tokens = q.split(/\s+/).filter(Boolean);
    if (!tokens.length) { body.normalize(); return; }
    const re = new RegExp('(' + tokens.map(t => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|') + ')', 'gi');
    const walker = document.createTreeWalker(body, NodeFilter.SHOW_TEXT, {
      acceptNode(node) {
        if (!node.nodeValue || !node.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
        let p = node.parentNode;
        while (p && p !== body) {
          const tag = p.nodeName;
          if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'MARK') return NodeFilter.FILTER_REJECT;
          p = p.parentNode;
        }
        return re.test(node.nodeValue) ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT;
      }
    });
    const targets = [];
    let cur;
    while ((cur = walker.nextNode())) targets.push(cur);
    targets.forEach(node => {
      const text = node.nodeValue;
      const frag = document.createDocumentFragment();
      let last = 0;
      text.replace(re, (m, _g, off) => {
        if (off > last) frag.appendChild(document.createTextNode(text.slice(last, off)));
        const mark = document.createElement('mark');
        mark.className = 'search-hit';
        mark.textContent = m;
        frag.appendChild(mark);
        last = off + m.length;
        return m;
      });
      if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
      node.parentNode.replaceChild(frag, node);
    });
    body.normalize();
  }

  function paintLegend(el) {
    el.innerHTML = window.DATA.clusters.map(c =>
      '<div class="row" data-cluster="' + escapeHtml(c.id) + '" title="jump to cluster"><span class="swatch" style="background:' + c.color + '"></span> ' + escapeHtml(c.label) + '</div>'
    ).join('');
    // Event delegation: clicking a legend row jumps to that cluster
    // in the Clusters tab (rendering lazily if needed) then scrolls.
    el.addEventListener('click', (e) => {
      const row = e.target.closest('.row[data-cluster]');
      if (!row) return;
      const cid = row.getAttribute('data-cluster');
      setTab('clusters');
      // Ensure the clusters panel is rendered before we try to scroll.
      if (!clustersPanelRendered) renderClustersPanel();
      requestAnimationFrame(() => {
        const target = document.getElementById('cluster-' + cid);
        if (target) target.scrollIntoView({behavior: 'smooth', block: 'start'});
      });
    });
  }

  function renderGapDetail(gap, body) {
    const key = gap.row + '__' + gap.col;
    const suggestions = gapSuggestions[key] || [];
    let html = '<div class="gap-detail">';
    html += '<h2>gap</h2>';
    html += '<div class="meta gap-arrow"><strong>' + escapeHtml(gap.row) + '</strong> → <strong>' + escapeHtml(gap.col) + '</strong></div>';
    html += '<div class="meta" style="margin-top:8px">No atom currently sits in this (type-in × type-out) cell — a structural slot no atom fills yet.</div>';
    html += '<h3>candidates that might fit</h3>';
    if (!suggestions.length) {
      html += '<p class="empty">No obvious candidate — open question.</p>';
    } else {
      html += '<ul class="gap-suggestions">';
      suggestions.forEach(s => {
        html += '<li><strong>' + escapeHtml(s.name) + '</strong> — ' + escapeHtml(s.why) + '</li>';
      });
      html += '</ul>';
    }
    html += '<div class="meta" style="margin-top:12px;font-size:11px">Hypotheses only; not minted atoms. They mark the conceptual shape a real atom could take.</div>';
    html += '</div>';
    body.innerHTML = html;
  }

  let searchPanelRendered = false;
  let searchIndex = null;
  function buildSearchIndex() {
    if (searchIndex) return;
    searchIndex = window.DATA.nodes.map(n => {
      const c = clusterById[n.cluster] || {label: ''};
      // Concatenate efficient-to-scan fields. Citation prose is excluded
      // (very long per atom; would blow per-keystroke cost).
      const parts = [
        n.id, n.name, n.type_in, n.type_out, n.tier, n.status,
        n.is_molecule ? 'molecule' : '',
        c.label || '',
        (n.evokes || []).join(' '),
        n.agent_instruction || '',
        (n.canonical_instances || []).join(' '),
      ];
      return { n: n, text: parts.join(' ').toLowerCase() };
    });
    // Sort by lex-id ascending — boring, predictable, matches the order
    // a future Dewey renumbering would settle into.
    searchIndex.sort((a, b) => a.n.id < b.n.id ? -1 : (a.n.id > b.n.id ? 1 : 0));
  }

  function renderSearchPanel(query) {
    buildSearchIndex();
    const results = document.getElementById('shell-search-results');
    const meta = document.getElementById('shell-search-meta');
    const q = (query || '').trim().toLowerCase();
    let matched = searchIndex;
    if (q) {
      // Multi-token AND: every whitespace-separated token must appear.
      const tokens = q.split(/\s+/).filter(Boolean);
      matched = searchIndex.filter(it => tokens.every(t => it.text.includes(t)));
    }
    meta.textContent = (q ? matched.length + ' match' + (matched.length === 1 ? '' : 'es') + ' of ' + searchIndex.length + ' atoms — top-bar search; click any row.' : 'All ' + searchIndex.length + ' atoms. Top-bar search filters by id, name, type, tier, status, cluster, evokes, agent-instruction, and canonical-instances. Click any row.');
    if (!matched.length) {
      results.innerHTML = '<div class="search-empty">No atoms match.</div>';
      return;
    }
    const tokens = q ? q.split(/\s+/).filter(Boolean) : [];
    const parts = matched.map(it => {
      const n = it.n;
      const cluster = clusterById[n.cluster] || {color: '#5a4f3a'};
      const idHTML   = highlightTokens(escapeHtml(n.id), tokens);
      const nameHTML = highlightTokens(escapeHtml(n.name), tokens);
      return '<div class="search-row" data-id="' + escapeHtml(n.id) + '" style="border-left-color:' + cluster.color + '">'
        + '<span class="id">' + idHTML + '</span>'
        + '<span class="name">' + nameHTML + '</span>'
        + '<span class="meta">' + escapeHtml(n.type_in) + ' → ' + escapeHtml(n.type_out) + ' · ' + escapeHtml(n.tier) + (n.status === 'under-review' ? ' · UR' : '') + '</span>'
        + '</div>';
    });
    results.innerHTML = parts.join('');
    if (!searchPanelRendered) {
      // One-time wiring: row clicks via event delegation. (Input is the
      // top-bar #shell-search-input — wired in wireControls.)
      results.addEventListener('click', (e) => {
        const row = e.target.closest('.search-row');
        if (!row) return;
        const id = row.getAttribute('data-id');
        if (id) selectAtom(id);
      });
      searchPanelRendered = true;
    }
  }

  // Wrap every case-insensitive match of any token in <mark>. Operates on
  // an already-escaped HTML string, splitting on text and reassembling so
  // we never wrap inside HTML tags.
  function highlightTokens(escapedHTML, tokens) {
    if (!tokens || !tokens.length) return escapedHTML;
    const re = new RegExp('(' + tokens.map(t => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|') + ')', 'gi');
    return escapedHTML.replace(re, '<mark>$1</mark>');
  }

  let clustersPanelRendered = false;
  function renderClustersPanel() {
    if (clustersPanelRendered) return;
    const host = document.getElementById('shell-clusters-host');
    const nodeById = {};
    window.DATA.nodes.forEach(n => { nodeById[n.id] = n; });
    const memberMap = {};
    window.DATA.nodes.forEach(n => {
      (memberMap[n.cluster] = memberMap[n.cluster] || []).push(n);
    });
    // Build as one string, parse once — DOM-per-chip was fine back when the
    // elements was in the hundreds
    // but quickly becomes a hang as the elements grows. Event delegation
    // (single listener on host) replaces 525 × 3 per-chip listeners.
    const parts = ['<div class="clusters-intro">Each cluster is a community detected by greedy modularity over the related[] + decomposes-into edge graph — atoms that share more edges with each other than with outside atoms. Color encodes cluster identity. Sorted by community size descending; the hub atom is the highest-degree member.</div>'];
    window.DATA.clusters.forEach(c => {
      const members = (memberMap[c.id] || []).slice().sort((a, b) => (b.in_degree || 0) - (a.in_degree || 0));
      parts.push('<section class="cluster-section" id="cluster-' + escapeHtml(c.id) + '" style="--cluster-c:' + c.color + '"><header><span class="swatch" style="background:' + c.color + '"></span><span class="cluster-label">' + escapeHtml(c.label) + '</span></header><div class="member-chips">');
      members.forEach(m => {
        parts.push('<a class="chip" data-id="' + escapeHtml(m.id) + '" href="#' + encodeURIComponent(m.id) + '">' + escapeHtml(m.id) + ' <span class="chip-name">' + escapeHtml(m.name) + '</span></a>');
      });
      parts.push('</div></section>');
    });
    host.innerHTML = parts.join('');

    // Event delegation: one listener on host for click + hover.
    host.addEventListener('click', (e) => {
      const a = e.target.closest('a.chip');
      if (!a) return;
      e.preventDefault();
      const id = a.getAttribute('data-id');
      if (id) selectAtom(id);
    });
    host.addEventListener('mouseover', (e) => {
      const a = e.target.closest('a.chip');
      if (!a) return;
      const id = a.getAttribute('data-id');
      const n = nodeById[id];
      if (!n) return;
      const c = clusterById[n.cluster] || {label: n.cluster};
      showTip('<span class="id">' + escapeHtml(n.id) + '</span><span class="name">' + escapeHtml(n.name) + '</span><span class="tag">' + escapeHtml(c.label) + '</span>', e);
    });
    host.addEventListener('mousemove', (e) => {
      if (e.target.closest('a.chip')) positionTip(e);
    });
    host.addEventListener('mouseout', (e) => {
      const a = e.target.closest('a.chip');
      if (!a) return;
      // hide when actually leaving the chip (not when moving to inner span)
      const rel = e.relatedTarget;
      if (rel && a.contains(rel)) return;
      hideTip();
    });
    clustersPanelRendered = true;
  }

  function parseHash() {
    const h = window.location.hash.replace(/^#/, '');
    if (!h) return;
    if (h.startsWith('lex-')) {
      // Bare atom id (legacy form from old anchor links)
      selectAtom(h);
      return;
    }
    const params = {};
    h.split('&').forEach(p => {
      const [k, v] = p.split('=');
      if (k) params[k] = decodeURIComponent(v || '');
    });
    if (params.tab) setTab(params.tab);
    if (params.id && params.pair) {
      selectPair(params.pair, params.id, params.edge || null);
    } else if (params.id) {
      selectAtom(params.id);
    }
  }

  function updateHash() {
    const parts = ['tab=' + state.activeTab];
    if (state.selectedAtomId) parts.push('id=' + encodeURIComponent(state.selectedAtomId));
    if (state.pairedAtomId)   parts.push('pair=' + encodeURIComponent(state.pairedAtomId));
    if (state.pairedEdgeType) parts.push('edge=' + encodeURIComponent(state.pairedEdgeType));
    const nh = '#' + parts.join('&');
    if (nh !== window.location.hash) {
      history.replaceState(null, '', nh);
    }
  }

  // Wire shell-owned controls
  function wireControls() {
    document.querySelectorAll('#shell-tabs button, #shell-bottom button').forEach(b => {
      b.addEventListener('click', () => setTab(b.getAttribute('data-tab')));
    });

    const active = document.getElementById('shell-filter-active');
    const ur = document.getElementById('shell-filter-under-review');
    const drawerActive = document.getElementById('drawer-filter-active');
    const drawerUR = document.getElementById('drawer-filter-under-review');
    function syncFilters(src) {
      state.statusFilters.active = src.active;
      state.statusFilters.underReview = src.underReview;
      [active, drawerActive].forEach(c => { if (c) c.checked = src.active; });
      [ur, drawerUR].forEach(c => { if (c) c.checked = src.underReview; });
      applyFilters();
    }
    active.addEventListener('input', () => syncFilters({active: active.checked, underReview: ur.checked}));
    ur.addEventListener('input',     () => syncFilters({active: active.checked, underReview: ur.checked}));
    if (drawerActive) drawerActive.addEventListener('input', () => syncFilters({active: drawerActive.checked, underReview: drawerUR.checked}));
    if (drawerUR)     drawerUR.addEventListener('input',     () => syncFilters({active: drawerActive.checked, underReview: drawerUR.checked}));

    const search = document.getElementById('shell-search-input');
    let searchDebounce = null;
    search.addEventListener('input', () => {
      state.searchQuery = search.value;
      applyFilters();
      // Switch to the List tab on first non-empty keystroke so the user
      // sees the filtered results immediately. Don't switch away if they
      // clear the query — let them stay where they were.
      if (search.value.trim() && state.activeTab !== 'search') setTab('search');
      // Re-render the list panel results (debounced).
      if (searchDebounce) clearTimeout(searchDebounce);
      searchDebounce = setTimeout(() => {
        if (typeof renderSearchPanel === 'function') renderSearchPanel(state.searchQuery);
        // Highlight terms in the currently-rendered detail body too.
        applyDetailHighlights();
      }, 60);
    });

    const hamburger = document.getElementById('shell-hamburger');
    const drawer = document.getElementById('shell-drawer');
    const backdrop = document.getElementById('shell-drawer-backdrop');
    function toggleDrawer(open) {
      drawer.classList.toggle('open', open);
      backdrop.classList.toggle('open', open);
    }
    hamburger.addEventListener('click', () => toggleDrawer(!drawer.classList.contains('open')));
    backdrop.addEventListener('click', () => toggleDrawer(false));

    const searchToggle = document.getElementById('shell-search-toggle');
    const topbar = document.getElementById('shell-topbar');
    searchToggle.addEventListener('click', () => {
      topbar.classList.toggle('search-open');
      if (topbar.classList.contains('search-open')) search.focus();
    });

    window.addEventListener('hashchange', parseHash);

    // Drag-resize the detail pane. Persist width via localStorage.
    const handle = document.getElementById('shell-resize-handle');
    const shell  = document.getElementById('shell');
    if (handle && shell) {
      const STORAGE_KEY = 'lex-shell-detail-w';
      // Restore previous width if any (desktop only; mobile ignores the CSS var anyway).
      const saved = parseInt(localStorage.getItem(STORAGE_KEY) || '', 10);
      if (saved && saved >= 240 && saved <= 1200) {
        shell.style.setProperty('--shell-detail-w', saved + 'px');
      }
      let dragging = false;
      function onMove(e) {
        if (!dragging) return;
        const rect = shell.getBoundingClientRect();
        // Distance from the right edge → detail width.
        const w = Math.max(240, Math.min(1200, rect.right - e.clientX));
        shell.style.setProperty('--shell-detail-w', w + 'px');
        // Repaint matrix on width change so the canvas refits.
        if (window.MatrixPanel && typeof window.MatrixPanel.onFiltersChanged === 'function') {
          // No dedicated resize hook — onFiltersChanged triggers a recompute + repaint.
          // Throttle via rAF to keep drag smooth.
          if (!onMove._raf) {
            onMove._raf = requestAnimationFrame(() => {
              onMove._raf = null;
              window.MatrixPanel.onFiltersChanged();
            });
          }
        }
      }
      function onUp() {
        if (!dragging) return;
        dragging = false;
        handle.classList.remove('dragging');
        document.body.style.userSelect = '';
        window.removeEventListener('mousemove', onMove);
        window.removeEventListener('mouseup', onUp);
        const w = parseInt(getComputedStyle(shell).getPropertyValue('--shell-detail-w'), 10);
        if (w) localStorage.setItem(STORAGE_KEY, w);
      }
      handle.addEventListener('mousedown', (e) => {
        e.preventDefault();
        dragging = true;
        handle.classList.add('dragging');
        document.body.style.userSelect = 'none';
        window.addEventListener('mousemove', onMove);
        window.addEventListener('mouseup', onUp);
      });
      // Double-click resets to default.
      handle.addEventListener('dblclick', () => {
        shell.style.removeProperty('--shell-detail-w');
        localStorage.removeItem(STORAGE_KEY);
        if (window.MatrixPanel) window.MatrixPanel.onFiltersChanged();
      });
    }
  }

  return {
    state,
    setTab,
    selectAtom,
    selectPair,
    selectGapCell,
    showTip,
    hideTip,
    positionTip,
    get statusFilters() { return state.statusFilters; },
    get searchQuery()   { return state.searchQuery; },
    init() {
      wireControls();
      paintLegend(document.getElementById('shell-cluster-legend'));
      const dl = document.getElementById('drawer-cluster-legend');
      if (dl) paintLegend(dl);
      // Clusters panel renders lazily on first tab activation (see setTab).
      updateStats();
      paintPlaceholder();
      parseHash();
    },
  };
})();
</script>
<script>{{ .MatrixJS }}</script>
<script>{{ .PivotJS }}</script>
<script>
window.LexiconShell.init();
</script>
</body>
</html>
`

type shellData struct {
	MatrixCSS    template.CSS
	MatrixHTML   template.HTML
	MatrixJS     template.JS
	PivotCSS     template.CSS
	PivotHTML    template.HTML
	PivotJS      template.JS
	DataJSON     template.JS
	RowOrderJSON template.JS
	ColOrderJSON template.JS
}

// RenderShell emits the unified app shell composing the matrix and
// pivot panels with a persistent detail pane and a single shared
// DATA blob. Used for `public/index.html` (the public landing page).
func RenderShell(g Graph) ([]byte, error) {
	jsonBytes, err := json.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("marshal graph: %w", err)
	}
	rowBytes, err := json.Marshal(PivotRowOrder)
	if err != nil {
		return nil, fmt.Errorf("marshal row order: %w", err)
	}
	colBytes, err := json.Marshal(PivotColOrder)
	if err != nil {
		return nil, fmt.Errorf("marshal col order: %w", err)
	}
	pivotHTML, err := renderPivotPanelHTML()
	if err != nil {
		return nil, fmt.Errorf("render pivot panel html: %w", err)
	}

	t := template.Must(template.New("shell").Parse(shellTemplate))
	var buf bytes.Buffer
	if err := t.Execute(&buf, shellData{
		MatrixCSS:    template.CSS(MatrixPanelCSS),
		MatrixHTML:   template.HTML(MatrixPanelHTML),
		MatrixJS:     template.JS(MatrixPanelJS),
		PivotCSS:     template.CSS(PivotPanelCSS),
		PivotHTML:    template.HTML(pivotHTML),
		PivotJS:      template.JS(PivotPanelJS),
		DataJSON:     template.JS(jsonBytes),
		RowOrderJSON: template.JS(rowBytes),
		ColOrderJSON: template.JS(colBytes),
	}); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
