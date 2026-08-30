package viz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
)

// PivotPanelCSS, PivotPanelHTML, PivotPanelJS are the three pieces of
// the type-in × type-out pivot view, factored out so the unified
// shell can compose them next to the matrix panel. Every CSS rule is
// scoped under `.pivot-panel`, JS is IIFE-wrapped, the only entry
// point is `window.PivotPanel`, and colliding IDs are prefixed with
// `pivot-`. Tier filter chrome lives in the panel; status filter and
// search migrate up to the shell via `window.LexiconShell`. The
// standalone `RenderPivot` wraps the three with a minimal top-bar so
// the legacy `/pivot.html` URL still works.
const PivotPanelCSS = `
.pivot-panel { display: flex; flex-direction: column; height: 100%; min-height: 0; background: var(--bg, #16130f); }
.pivot-panel * { box-sizing: border-box; }
.pivot-panel #pivot-controls { padding: 6px 12px; background: var(--bg-panel, #1d1812); border-bottom: 1px solid var(--border, #322a20); display: flex; gap: 14px; align-items: center; flex-wrap: wrap; position: sticky; top: 0; z-index: 5; }
.pivot-panel #pivot-controls .group { display: flex; gap: 6px; align-items: center; }
.pivot-panel #pivot-controls .group-label { font-weight: 600; color: var(--text-mute, #9c8f7a); }
.pivot-panel #pivot-controls label { display: inline-flex; align-items: center; gap: 3px; cursor: pointer; color: var(--text-soft, #d7c9b1); }
.pivot-panel #pivot-stats { color: var(--text-mute, #9c8f7a); font-size: 12px; margin-left: auto; }
.pivot-panel #pivot-scroll { flex: 1 1 0; min-height: 0; overflow: auto; }
/* Pivot intro now lives in the shell-detail placeholder slot. */
.pivot-panel table.pivot { border-collapse: collapse; width: 100%; max-width: 1600px; margin: 16px auto; background: var(--bg-panel, #1d1812); }
.pivot-panel table.pivot th, .pivot-panel table.pivot td { border: 1px solid var(--border, #322a20); vertical-align: top; padding: 8px; text-align: left; color: var(--text-soft, #d7c9b1); }
.pivot-panel table.pivot th { background: var(--bg-card, #211c16); font-weight: 600; color: var(--text, #ece3d4); }
.pivot-panel table.pivot th.corner { background: var(--bg, #16130f); text-align: center; color: var(--text-mute, #9c8f7a); font-weight: 400; font-style: italic; }
.pivot-panel table.pivot th.col-head { text-align: center; }
.pivot-panel table.pivot th.row-head { text-align: right; min-width: 90px; }
.pivot-panel table.pivot td.cell { min-width: 180px; max-width: 280px; padding: 6px; }
.pivot-panel table.pivot td.cell .cell-count { display: block; font-family: monospace; color: var(--text-mute, #9c8f7a); font-size: 11px; margin-bottom: 4px; }
.pivot-panel table.pivot td.cell .cell-count.has { color: var(--accent, #c9a45e); font-weight: 600; font-size: 16px; }
.pivot-panel table.pivot td.cell.empty { background: var(--bg, #16130f); border: 1px dashed var(--border-line, #4a3f30); cursor: pointer; }
.pivot-panel table.pivot td.cell.empty:hover { background: var(--gap-bg, #2a2418); border-color: var(--gap, #b87a1a); }
.pivot-panel table.pivot td.cell.empty::after { content: '— gap —'; display: block; color: var(--text-mute, #9c8f7a); font-style: italic; font-size: 11px; text-align: center; padding-top: 8px; }
.pivot-panel table.pivot td.cell.empty:hover::after { content: '— click for candidates —'; color: var(--gap, #b87a1a); }
.pivot-panel table.pivot td.cell .tiles { display: flex; flex-wrap: wrap; gap: 2px; margin-top: 4px; }
.pivot-panel table.pivot td.cell .tiles a.tile { display: inline-flex; align-items: center; justify-content: center; width: 30px; height: 16px; border: 1px solid rgba(0,0,0,0.4); box-sizing: border-box; text-decoration: none; cursor: pointer; transition: transform 0.05s; font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace; font-size: 9px; line-height: 1; color: rgba(0,0,0,0.75); letter-spacing: 0; }
/* Mobile: bigger tap targets — 44×24 hits the iOS HIG minimum for hit area. */
@media (max-width: 599px) {
  .pivot-panel table.pivot td.cell .tiles a.tile { width: 44px; height: 24px; font-size: 11px; }
  .pivot-panel table.pivot td.cell { min-width: 0; padding: 4px; }
  .pivot-panel table.pivot th { padding: 6px 4px; font-size: 11px; }
}
.pivot-panel table.pivot td.cell .tiles a.tile:hover { transform: scale(1.8); border-color: var(--accent, #c9a45e); position: relative; z-index: 5; color: #000; }
.pivot-panel table.pivot td.cell .tiles a.tile.molecule { border: 2px solid var(--accent, #c9a45e); }
.pivot-panel table.pivot td.cell .tiles a.tile.selected { outline: 2px solid var(--select, #d44a3a); outline-offset: 1px; z-index: 4; position: relative; }
.pivot-panel .pivot-footer { max-width: 1600px; margin: 16px auto; padding: 8px 16px; color: var(--text-mute, #9c8f7a); font-size: 11px; }
`

// PivotPanelHTML is the panel's outer DOM. The intro + table + footer
// live inside a scrollable region so the panel can fill any height
// the shell gives it. Tier filter is panel-local; status filter and
// search are owned by the shell.
const PivotPanelHTML = `<div class="pivot-panel">
  <div id="pivot-controls">
    <span class="group-label">pivot: type-in × type-out</span>
    <div class="group">
      <span class="group-label">tier:</span>
      <label><input type="checkbox" data-tier="atomic" checked> atomic</label>
      <label><input type="checkbox" data-tier="composition" checked> composition</label>
      <label><input type="checkbox" data-tier="molecule" checked> molecule</label>
      <label><input type="checkbox" data-tier="reaction" checked> reaction</label>
    </div>
    <div id="pivot-stats"></div>
  </div>
  <div id="pivot-scroll">
    <table class="pivot" id="pivot"></table>
    <div class="pivot-footer">Cells fill as the elements grows. Filters narrow the cell contents but the row/col grid stays the same so the gap structure stays visible.</div>
  </div>
</div>`

// PivotPanelJS is the panel's JS. Reads `window.LexiconShell` for
// status filter + search query; exposes `window.PivotPanel` so the
// shell can drive selection + filter changes.
const PivotPanelJS = `(function() {
  const DATA = window.DATA;
  const Shell = window.LexiconShell;
  const ROW_ORDER = window.LexiconPivot.ROW_ORDER;
  const COL_ORDER = window.LexiconPivot.COL_ORDER;

  const nodesByCell = {};
  const nodesById = {};
  DATA.nodes.forEach(n => {
    nodesById[n.id] = n;
    const k = n.type_in + '__' + n.type_out;
    (nodesByCell[k] = nodesByCell[k] || []).push(n);
  });
  Object.values(nodesByCell).forEach(arr => arr.sort((a, b) => {
    if (a.is_molecule !== b.is_molecule) return a.is_molecule ? -1 : 1;
    if (a.in_degree !== b.in_degree) return b.in_degree - a.in_degree;
    return a.id < b.id ? -1 : 1;
  }));

  let selectedId = null;

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c]);
  }

  function passesFilters(n) {
    const f = Shell.statusFilters;
    const okStatus = (n.status === 'active' && f.active) || (n.status === 'under-review' && f.underReview);
    if (!okStatus) return false;
    const tierCb = document.querySelector('.pivot-panel input[data-tier="' + n.tier + '"]');
    if (tierCb && !tierCb.checked) return false;
    const q = (Shell.searchQuery || '').trim().toLowerCase();
    if (q && !(n.id.toLowerCase().includes(q) || (n.name || '').toLowerCase().includes(q))) return false;
    return true;
  }

  function render() {
    const tbl = document.getElementById('pivot');
    let html = '<thead><tr><th class="corner">type-in ↓ / type-out →</th>';
    COL_ORDER.forEach(co => { html += '<th class="col-head">' + escapeHtml(co) + '</th>'; });
    html += '</tr></thead><tbody>';
    let totalVisible = 0;
    let totalGaps = 0;
    ROW_ORDER.forEach(ri => {
      html += '<tr><th class="row-head">' + escapeHtml(ri) + '</th>';
      COL_ORDER.forEach(co => {
        const k = ri + '__' + co;
        const all = nodesByCell[k] || [];
        const visible = all.filter(passesFilters);
        totalVisible += visible.length;
        if (all.length === 0) totalGaps++;
        const isAbsoluteGap = all.length === 0;
        const isEmpty = visible.length === 0;
        const cellClass = isEmpty ? ' empty' : '';
        const gapAttrs = isAbsoluteGap ? ' data-gap="1" data-row="' + escapeHtml(ri) + '" data-col="' + escapeHtml(co) + '"' : '';
        html += '<td class="cell' + cellClass + '"' + gapAttrs + '>';
        if (!isEmpty) {
          html += '<span class="cell-count has">' + visible.length + (all.length !== visible.length ? ' / ' + all.length : '') + '</span>';
          html += '<div class="tiles">';
          visible.forEach(n => {
            const meta = (DATA.clusters.find(c => c.id === n.cluster) || {color: '#5a4f3a', label: n.cluster});
            const num = n.id.replace(/^lex-/, '');
            const sel = (n.id === selectedId) ? ' selected' : '';
            html += '<a class="tile' + (n.is_molecule ? ' molecule' : '') + sel + '" ';
            html += 'data-id="' + escapeHtml(n.id) + '" ';
            html += 'href="#' + encodeURIComponent(n.id) + '" ';
            html += 'style="background:' + meta.color + '">' + escapeHtml(num) + '</a>';
          });
          html += '</div>';
        } else if (all.length > 0) {
          html += '<span class="cell-count">0 visible / ' + all.length + ' filtered</span>';
        }
        html += '</td>';
      });
      html += '</tr>';
    });
    html += '</tbody>';
    tbl.innerHTML = html;
    document.getElementById('pivot-stats').textContent = totalVisible + ' atoms visible · ' + totalGaps + ' unfilled cells';

    // Wire tile clicks + fast custom tooltip. The anchor href stays so
    // deep-link bookmarking still works; preventDefault keeps hash-routing
    // managed centrally by the shell.
    tbl.querySelectorAll('a.tile').forEach(a => {
      const id = a.getAttribute('data-id');
      const n  = id ? nodesById[id] : null;
      a.addEventListener('click', (e) => {
        e.preventDefault();
        if (id) Shell.selectAtom(id);
      });
      if (n) {
        const cluster = DATA.clusters.find(c => c.id === n.cluster) || {label: n.cluster};
        const tipHTML = '<span class="id">' + escapeHtml(n.id) + '</span>'
          + '<span class="name">' + escapeHtml(n.name) + '</span>'
          + '<span class="tag">' + escapeHtml(cluster.label) + '</span>';
        a.addEventListener('mouseenter', (e) => Shell.showTip(tipHTML, e));
        a.addEventListener('mousemove', (e) => Shell.positionTip(e));
        a.addEventListener('mouseleave', Shell.hideTip);
      }
    });

    // Wire gap-cell clicks: open the gap-detail view in the detail pane.
    tbl.querySelectorAll('td.cell[data-gap="1"]').forEach(td => {
      const row = td.getAttribute('data-row');
      const col = td.getAttribute('data-col');
      td.addEventListener('click', () => {
        if (row && col) Shell.selectGapCell(row, col);
      });
      const tipHTML = '<span class="tag">gap</span><span class="name">'
        + escapeHtml(row) + ' → ' + escapeHtml(col) + ' — click for candidate suggestions</span>';
      td.addEventListener('mouseenter', (e) => Shell.showTip(tipHTML, e));
      td.addEventListener('mousemove', (e) => Shell.positionTip(e));
      td.addEventListener('mouseleave', Shell.hideTip);
    });
  }

  document.querySelectorAll('.pivot-panel #pivot-controls input').forEach(el => el.addEventListener('input', render));

  // Pivot intro now lives in the shell-detail placeholder slot.

  window.PivotPanel = {
    onSelectionChanged(id) {
      selectedId = id;
      render();
      if (id) {
        const tile = document.querySelector('.pivot-panel a.tile[data-id="' + id + '"]');
        if (tile) tile.scrollIntoView({behavior: 'smooth', block: 'center', inline: 'center'});
      }
    },
    onFiltersChanged() {
      render();
    },
  };

  render();
})();`

// pivotStandaloneTemplate wraps PivotPanelCSS/HTML/JS in a minimal
// page chrome so /pivot.html keeps working on its own.
const pivotStandaloneTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Lexicon — pivot table</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  :root { --bg: #16130f; --bg-panel: #1d1812; --bg-card: #211c16; --border: #322a20; --border-line: #4a3f30; --text: #ece3d4; --text-soft: #d7c9b1; --text-mute: #9c8f7a; --accent: #c9a45e; --accent-mute: #2a2118; --accent-strong: #e0b56b; --select: #d44a3a; --gap: #b87a1a; --gap-bg: #2a2418; --gap-border: #4a3f20; }
  * { box-sizing: border-box; }
  html, body { margin: 0; padding: 0; height: 100%; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; color: var(--text); background: var(--bg); font-size: 13px; }
  * { scrollbar-color: var(--border-line) var(--bg-panel); scrollbar-width: thin; }
  *::-webkit-scrollbar { width: 11px; height: 11px; }
  *::-webkit-scrollbar-track { background: var(--bg-panel); }
  *::-webkit-scrollbar-thumb { background: var(--border-line); border: 2px solid var(--bg-panel); border-radius: 6px; }
  *::-webkit-scrollbar-thumb:hover { background: var(--accent); }
  ::selection { background: var(--accent); color: var(--bg); }
  input, textarea, select, button { font-family: inherit; }
  input[type="checkbox"], input[type="radio"] { accent-color: var(--accent); }
  input[type="text"], input[type="search"] { background: var(--bg-card); color: var(--text); }
  input[type="text"]::placeholder, input[type="search"]::placeholder { color: var(--text-mute); }
  input[type="text"]:focus, input[type="search"]:focus { outline: 2px solid var(--accent); outline-offset: -1px; border-color: var(--accent); }
  #standalone { display: grid; grid-template-rows: auto 1fr; height: 100vh; }
  #standalone-topbar { padding: 6px 12px; background: var(--bg-panel); border-bottom: 1px solid var(--border); display: flex; gap: 14px; align-items: center; flex-wrap: wrap; }
  #standalone-topbar .group { display: flex; gap: 6px; align-items: center; }
  #standalone-topbar .group-label { font-weight: 600; color: var(--text-mute); }
  #standalone-topbar input[type="text"] { padding: 4px 8px; border: 1px solid var(--border); background: var(--bg-card); color: var(--text); border-radius: 3px; font-size: 13px; width: 220px; }
  #standalone-topbar label { display: inline-flex; align-items: center; gap: 3px; cursor: pointer; }
  #standalone-topbar a.navlink { color: var(--accent); text-decoration: none; padding: 4px 8px; border: 1px solid var(--border); border-radius: 3px; }
  #standalone-topbar a.navlink:hover { background: var(--accent-mute); border-color: var(--accent); }
{{ .PanelCSS }}
</style>
</head>
<body>
<div id="standalone">
  <div id="standalone-topbar">
    <a href="index.html" class="navlink">← shell</a>
    <a href="matrix.html" class="navlink">← matrix view</a>
    <div class="group">
      <span class="group-label">status:</span>
      <label><input type="checkbox" id="sa-filter-active" checked> active</label>
      <label><input type="checkbox" id="sa-filter-under-review"> under-review</label>
    </div>
    <div class="group">
      <input type="text" id="sa-search" placeholder="search by name or id…">
    </div>
  </div>
{{ .PanelHTML }}
</div>
<script>
window.DATA = {{ .DataJSON }};
window.LexiconPivot = { ROW_ORDER: {{ .RowOrderJSON }}, COL_ORDER: {{ .ColOrderJSON }} };
window.LexiconShell = {
  statusFilters: { active: true, underReview: false },
  searchQuery: '',
  selectAtom: function(_id) {},
};
document.getElementById('sa-filter-active').addEventListener('input', e => {
  window.LexiconShell.statusFilters.active = e.target.checked;
  if (window.PivotPanel) window.PivotPanel.onFiltersChanged();
});
document.getElementById('sa-filter-under-review').addEventListener('input', e => {
  window.LexiconShell.statusFilters.underReview = e.target.checked;
  if (window.PivotPanel) window.PivotPanel.onFiltersChanged();
});
document.getElementById('sa-search').addEventListener('input', e => {
  window.LexiconShell.searchQuery = e.target.value;
  if (window.PivotPanel) window.PivotPanel.onFiltersChanged();
});
</script>
<script>{{ .PanelJS }}</script>
</body>
</html>
`

type pivotData struct {
	PanelCSS     template.CSS
	PanelHTML    template.HTML
	PanelJS      template.JS
	DataJSON     template.JS
	RowOrderJSON template.JS
	ColOrderJSON template.JS
	RowCount     int
	ColCount     int
}

// PivotRowOrder + PivotColOrder are the row/col vocabularies used by
// the pivot view. Exported so the shell can render the panel HTML
// with the right axis counts when composing.
var (
	PivotRowOrder = []string{"state", "situation", "process", "question", "frame", "claim", "posture", "composition", "structure"}
	PivotColOrder = []string{"state", "process", "posture", "frame", "claim", "question", "composition", "structure", "typology", "warning"}
)

// renderPivotPanelHTML renders the PivotPanelHTML template with the
// row/col counts substituted into the intro line. Used by both
// RenderPivot (standalone) and the shell.
func renderPivotPanelHTML() (string, error) {
	t := template.Must(template.New("pivot-html").Parse(PivotPanelHTML))
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		RowCount int
		ColCount int
	}{
		RowCount: len(PivotRowOrder),
		ColCount: len(PivotColOrder),
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderPivot emits the standalone pivot-table page. Row/col axis
// match RenderMatrix so the two views share a visual vocabulary.
func RenderPivot(g Graph) ([]byte, error) {
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
	panelHTML, err := renderPivotPanelHTML()
	if err != nil {
		return nil, fmt.Errorf("render pivot panel html: %w", err)
	}

	t := template.Must(template.New("pivot").Parse(pivotStandaloneTemplate))
	var buf bytes.Buffer
	if err := t.Execute(&buf, pivotData{
		PanelCSS:     template.CSS(PivotPanelCSS),
		PanelHTML:    template.HTML(panelHTML),
		PanelJS:      template.JS(PivotPanelJS),
		DataJSON:     template.JS(jsonBytes),
		RowOrderJSON: template.JS(rowBytes),
		ColOrderJSON: template.JS(colBytes),
		RowCount:     len(PivotRowOrder),
		ColCount:     len(PivotColOrder),
	}); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
