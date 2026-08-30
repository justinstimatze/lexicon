package viz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
)

// MatrixPanelCSS, MatrixPanelHTML, MatrixPanelJS are the three pieces
// of the cluster-sorted adjacency-matrix view, factored out so the
// unified shell can compose them alongside the pivot panel. Every CSS
// rule is scoped under a `.matrix-panel` ancestor, every JS global is
// IIFE-wrapped and the only entry point is `window.MatrixPanel`, and
// every ID that could collide with the pivot panel or the shell is
// prefixed with `matrix-`. The standalone `RenderMatrix` wraps the
// three with a minimal top-bar so the legacy `matrix.html` URL still
// works on its own.
const MatrixPanelCSS = `
.matrix-panel { display: grid; grid-template-rows: auto 1fr; height: 100%; min-height: 0; background: var(--bg, #16130f); }
.matrix-panel * { box-sizing: border-box; }
.matrix-panel #matrix-controls { padding: 6px 12px; background: var(--bg-panel, #1d1812); border-bottom: 1px solid var(--border, #322a20); display: flex; gap: 14px; align-items: center; flex-wrap: wrap; }
.matrix-panel #matrix-controls .group { display: flex; gap: 6px; align-items: center; }
.matrix-panel .zoom-btn { padding: 2px 8px; border: 1px solid var(--border, #322a20); background: var(--bg-card, #211c16); color: var(--text, #ece3d4); cursor: pointer; font-family: monospace; font-size: 14px; line-height: 1; border-radius: 3px; }
.matrix-panel .zoom-btn:hover { background: var(--accent-mute, #2a2118); border-color: var(--accent, #c9a45e); }
.matrix-panel #matrix-zoom-readout { color: var(--text-mute, #9c8f7a); font-size: 11px; min-width: 44px; text-align: center; font-family: monospace; }
.matrix-panel #matrix-stats { color: var(--text-mute, #9c8f7a); font-size: 12px; margin-left: auto; }
.matrix-panel #matrix-wrap { overflow: auto; position: relative; background: var(--bg, #16130f); min-height: 0; }
.matrix-panel #matrix-stage { position: relative; }
.matrix-panel #matrix { display: block; cursor: grab; position: absolute; top: 0; left: 0; transform-origin: 0 0; image-rendering: pixelated; image-rendering: crisp-edges; }
.matrix-panel #matrix.grabbing { cursor: grabbing; }
.matrix-panel #matrix-tooltip { position: absolute; pointer-events: none; background: var(--bg-card, #211c16); border: 1px solid var(--border-line, #4a3f30); color: var(--text-soft, #d7c9b1); border-radius: 3px; padding: 4px 8px; font-size: 11px; box-shadow: 0 4px 14px rgba(0,0,0,0.45); opacity: 0; transition: opacity 0.12s; white-space: nowrap; max-width: 360px; z-index: 5; }
.matrix-panel #matrix-tooltip.visible { opacity: 1; }
.matrix-panel #matrix-tooltip .id { font-family: monospace; color: var(--text-mute, #9c8f7a); }
.matrix-panel #matrix-tooltip .name { display: block; color: var(--text, #ece3d4); font-size: 11px; margin: 1px 0; line-height: 1.3; white-space: normal; }
/* Matrix intro now lives in the shell-detail placeholder (no real estate cost
 * in the panel; full content in the wider detail sidebar; no truncation). */
`

// MatrixPanelHTML is the panel's outer DOM. IDs that could collide
// with the pivot panel or the shell are prefixed with `matrix-`; the
// canvas itself stays `#matrix` since pivot has no canvas.
const MatrixPanelHTML = `<div class="matrix-panel">
  <div id="matrix-controls">
    <div class="group">
      <button class="zoom-btn" id="matrix-zoom-out" title="zoom out (or Ctrl+scroll)">−</button>
      <span id="matrix-zoom-readout">fit</span>
      <button class="zoom-btn" id="matrix-zoom-in" title="zoom in (or Ctrl+scroll)">+</button>
      <button class="zoom-btn" id="matrix-zoom-fit" title="fit to viewport">⤢</button>
    </div>
    <div id="matrix-stats"></div>
  </div>
  <div id="matrix-wrap">
    <div id="matrix-stage">
      <canvas id="matrix"></canvas>
    </div>
    <div id="matrix-tooltip"></div>
  </div>
</div>`

// MatrixPanelJS is the panel's JS, IIFE-wrapped so its locals don't
// leak. It reads global state from `window.LexiconShell` (provided by
// the shell, or by the standalone wrapper below) and exposes a small
// `window.MatrixPanel` API the shell calls when selection or filters
// change. `window.DATA` is shared with the pivot panel.
const MatrixPanelJS = `(function() {
  const DATA = window.DATA;
  const Shell = window.LexiconShell;

  const clusterById = {};
  DATA.clusters.forEach(c => { clusterById[c.id] = c; });

  const CELL_SIZE   = 4;
  const LEFT_MARGIN = 180;
  const TOP_MARGIN  = 180;
  const STRIPE_W    = 10;

  const canvas = document.getElementById('matrix');
  const ctx    = canvas.getContext('2d');
  const stage  = document.getElementById('matrix-stage');
  const wrap   = document.getElementById('matrix-wrap');
  const tooltip = document.getElementById('matrix-tooltip');

  let viewScale = 1;
  const SCALE_MIN = 0.25;
  const SCALE_MAX = 4;
  function applyScale() {
    canvas.style.transform = 'scale(' + viewScale + ')';
    stage.style.width  = (canvas.width  * viewScale) + 'px';
    stage.style.height = (canvas.height * viewScale) + 'px';
    document.getElementById('matrix-zoom-readout').textContent = (viewScale * 100).toFixed(0) + '%';
  }
  function setScale(s, anchor) {
    const old = viewScale;
    viewScale = Math.min(SCALE_MAX, Math.max(SCALE_MIN, s));
    const before = anchor ? {
      sl: wrap.scrollLeft, st: wrap.scrollTop,
      rx: anchor.x - wrap.getBoundingClientRect().left + wrap.scrollLeft,
      ry: anchor.y - wrap.getBoundingClientRect().top + wrap.scrollTop,
    } : null;
    applyScale();
    if (before) {
      const factor = viewScale / old;
      wrap.scrollLeft = before.rx * factor - (anchor.x - wrap.getBoundingClientRect().left);
      wrap.scrollTop  = before.ry * factor - (anchor.y - wrap.getBoundingClientRect().top);
    }
  }
  function fitToWrap() {
    const rect = wrap.getBoundingClientRect();
    if (rect.width < 10 || rect.height < 10) return;
    const pad = 8;
    const sx = (rect.width  - pad) / canvas.width;
    const sy = (rect.height - pad) / canvas.height;
    setScale(Math.min(sx, sy));
  }

  function visibleNodes() {
    const f = Shell.statusFilters;
    const ns = DATA.nodes.filter(n =>
      (n.status === 'active' && f.active) ||
      (n.status === 'under-review' && f.underReview)
    );
    ns.sort((a, b) => {
      if (a.cluster !== b.cluster) return a.cluster < b.cluster ? -1 : 1;
      if (b.in_degree !== a.in_degree) return b.in_degree - a.in_degree;
      return a.id < b.id ? -1 : 1;
    });
    return ns;
  }

  let nodes = [];
  let indexById = {};
  let edgesBySource = {};
  let clusterRuns = [];
  let highlightId = null;
  let pairedHighlightId = null;

  function buildAdjacency() {
    indexById = {};
    nodes.forEach((n, i) => indexById[n.id] = i);
    edgesBySource = {};
    DATA.edges.forEach(e => {
      if (indexById[e.source] === undefined || indexById[e.target] === undefined) return;
      (edgesBySource[e.source] = edgesBySource[e.source] || {})[e.target] = e.type;
    });
    clusterRuns = [];
    let runStart = 0;
    let runCluster = nodes[0] ? nodes[0].cluster : null;
    nodes.forEach((n, i) => {
      if (n.cluster !== runCluster) {
        clusterRuns.push({cluster: runCluster, start: runStart, end: i});
        runStart = i;
        runCluster = n.cluster;
      }
    });
    if (nodes.length > 0) clusterRuns.push({cluster: runCluster, start: runStart, end: nodes.length});
  }

  function searchSet() {
    const q = (Shell.searchQuery || '').trim().toLowerCase();
    if (!q) return null;
    const out = new Set();
    nodes.forEach(n => {
      if (n.id.toLowerCase().includes(q) || (n.name || '').toLowerCase().includes(q)) {
        out.add(n.id);
      }
    });
    return out;
  }

  function render() {
    nodes = visibleNodes();
    buildAdjacency();
    const N = nodes.length;
    const gridSize = N * CELL_SIZE;
    canvas.width  = LEFT_MARGIN + gridSize;
    canvas.height = TOP_MARGIN  + gridSize;
    applyScale();

    ctx.fillStyle = '#16130f';
    ctx.fillRect(0, 0, canvas.width, canvas.height);

    clusterRuns.forEach((run, i) => {
      const tint = i % 2 === 0 ? '#1d1812' : '#211c16';
      ctx.fillStyle = tint;
      ctx.fillRect(LEFT_MARGIN, TOP_MARGIN + run.start * CELL_SIZE, gridSize, (run.end - run.start) * CELL_SIZE);
    });

    const sset = searchSet();
    if (sset) {
      nodes.forEach((n, i) => {
        if (sset.has(n.id)) {
          ctx.fillStyle = 'rgba(201, 164, 94, 0.18)';
          ctx.fillRect(LEFT_MARGIN, TOP_MARGIN + i * CELL_SIZE, gridSize, CELL_SIZE);
          ctx.fillRect(LEFT_MARGIN + i * CELL_SIZE, TOP_MARGIN, CELL_SIZE, gridSize);
        }
      });
    }

    ctx.fillStyle = '#ece3d4';
    DATA.edges.forEach(e => {
      const si = indexById[e.source];
      const ti = indexById[e.target];
      if (si === undefined || ti === undefined) return;
      if (e.type === 'related') {
        ctx.fillStyle = '#ece3d4';
        ctx.fillRect(LEFT_MARGIN + ti * CELL_SIZE, TOP_MARGIN + si * CELL_SIZE, CELL_SIZE, CELL_SIZE);
        ctx.fillRect(LEFT_MARGIN + si * CELL_SIZE, TOP_MARGIN + ti * CELL_SIZE, CELL_SIZE, CELL_SIZE);
      } else if (e.type === 'decomposes-into') {
        ctx.fillStyle = '#c9a45e';
        ctx.fillRect(LEFT_MARGIN + ti * CELL_SIZE, TOP_MARGIN + si * CELL_SIZE, CELL_SIZE, CELL_SIZE);
      }
    });

    ctx.strokeStyle = '#322a20';
    ctx.lineWidth = 0.5;
    ctx.beginPath();
    ctx.moveTo(LEFT_MARGIN, TOP_MARGIN);
    ctx.lineTo(LEFT_MARGIN + gridSize, TOP_MARGIN + gridSize);
    ctx.stroke();

    clusterRuns.forEach(run => {
      const meta = clusterById[run.cluster] || {color: '#888', label: run.cluster};
      ctx.fillStyle = meta.color;
      ctx.fillRect(LEFT_MARGIN - STRIPE_W, TOP_MARGIN + run.start * CELL_SIZE, STRIPE_W, (run.end - run.start) * CELL_SIZE);
      ctx.fillRect(LEFT_MARGIN + run.start * CELL_SIZE, TOP_MARGIN - STRIPE_W, (run.end - run.start) * CELL_SIZE, STRIPE_W);

      const labelStr = meta.label || run.cluster;
      const dashIdx = labelStr.indexOf(' — ');
      const line1 = dashIdx > 0 ? labelStr.substring(0, dashIdx) : labelStr;
      const line2 = dashIdx > 0 ? labelStr.substring(dashIdx + 3) : '';

      const runHeight = (run.end - run.start) * CELL_SIZE;
      if (runHeight >= 32) {
        ctx.save();
        ctx.fillStyle = '#d7c9b1';
        ctx.font = '13px -apple-system, sans-serif';
        ctx.textBaseline = 'middle';
        const yCenter = TOP_MARGIN + run.start * CELL_SIZE + runHeight / 2;
        if (line2) {
          ctx.fillText(line1, 8, yCenter - 9);
          ctx.fillText(line2, 8, yCenter + 9);
        } else {
          ctx.fillText(line1, 8, yCenter);
        }
        ctx.restore();
      } else if (runHeight >= 14) {
        ctx.save();
        ctx.fillStyle = '#d7c9b1';
        ctx.font = '13px -apple-system, sans-serif';
        ctx.textBaseline = 'middle';
        const ty = TOP_MARGIN + run.start * CELL_SIZE + runHeight / 2;
        ctx.fillText(line1, 8, ty);
        ctx.restore();
      }
      const runWidth = (run.end - run.start) * CELL_SIZE;
      if (runWidth >= 32) {
        ctx.save();
        ctx.fillStyle = '#d7c9b1';
        ctx.font = '13px -apple-system, sans-serif';
        ctx.textBaseline = 'middle';
        const xCenter = LEFT_MARGIN + run.start * CELL_SIZE + runWidth / 2;
        ctx.translate(xCenter, TOP_MARGIN - STRIPE_W - 6);
        ctx.rotate(-Math.PI / 2);
        if (line2) {
          ctx.fillText(line1, 0, -9);
          ctx.fillText(line2, 0, 9);
        } else {
          ctx.fillText(line1, 0, 0);
        }
        ctx.restore();
      } else if (runWidth >= 14) {
        ctx.save();
        ctx.fillStyle = '#d7c9b1';
        ctx.font = '13px -apple-system, sans-serif';
        ctx.textBaseline = 'middle';
        const tx = LEFT_MARGIN + run.start * CELL_SIZE + runWidth / 2;
        ctx.translate(tx, TOP_MARGIN - STRIPE_W - 6);
        ctx.rotate(-Math.PI / 2);
        ctx.fillText(line1, 0, 0);
        ctx.restore();
      }
    });

    function strokeRowBand(i) {
      ctx.strokeRect(LEFT_MARGIN, TOP_MARGIN + i * CELL_SIZE - 0.5, gridSize, CELL_SIZE + 1);
    }
    function strokeColBand(i) {
      ctx.strokeRect(LEFT_MARGIN + i * CELL_SIZE - 0.5, TOP_MARGIN, CELL_SIZE + 1, gridSize);
    }
    function strokeCross(i) { strokeRowBand(i); strokeColBand(i); }
    ctx.strokeStyle = '#d44a3a';
    ctx.lineWidth = 1;
    // Single-select mode (header click → one atom): full cross at the
    // diagonal cell (atom × itself). Pair mode (cell click → two atoms):
    // single intersection at the actually-clicked cell — horizontal band
    // at the row atom + vertical band at the col atom. selectPair sets
    // highlightId = colId, pairedHighlightId = rowId, so:
    //   row band → indexById[pairedHighlightId]
    //   col band → indexById[highlightId]
    if (pairedHighlightId && indexById[pairedHighlightId] !== undefined && pairedHighlightId !== highlightId) {
      if (indexById[pairedHighlightId] !== undefined) strokeRowBand(indexById[pairedHighlightId]);
      if (highlightId && indexById[highlightId] !== undefined) strokeColBand(indexById[highlightId]);
    } else if (highlightId && indexById[highlightId] !== undefined) {
      strokeCross(indexById[highlightId]);
    }

    // Zoom-aware per-atom id labels in the margins. CSS-transform zoom
    // multiplies font size on screen, so 5px text at viewScale=2 reads
    // as 10px. Threshold tuned so labels appear once the screen-space
    // text is large enough to skim.
    if (viewScale >= 1.5) {
      ctx.save();
      const px = Math.max(3, Math.round(CELL_SIZE * 0.95));
      ctx.font = px + 'px ui-monospace, "SF Mono", Menlo, monospace';
      ctx.fillStyle = '#9c8f7a';
      ctx.textBaseline = 'middle';
      nodes.forEach((n, i) => {
        const num = n.id.replace(/^lex-/, '');
        const cy = TOP_MARGIN + i * CELL_SIZE + CELL_SIZE / 2;
        ctx.textAlign = 'right';
        ctx.fillText(num, LEFT_MARGIN - STRIPE_W - 2, cy);
        const cx = LEFT_MARGIN + i * CELL_SIZE + CELL_SIZE / 2;
        ctx.save();
        ctx.translate(cx, TOP_MARGIN - STRIPE_W - 2);
        ctx.rotate(-Math.PI / 2);
        ctx.textAlign = 'left';
        ctx.fillText(num, 0, 0);
        ctx.restore();
      });
      ctx.restore();
    }

    const edgeCount = DATA.edges.filter(e => indexById[e.source] !== undefined && indexById[e.target] !== undefined).length;
    document.getElementById('matrix-stats').textContent = N + ' atoms · ' + edgeCount + ' edges · ' + clusterRuns.length + ' clusters';
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c]);
  }

  canvas.addEventListener('mousemove', (e) => {
    if (panState.active) return;
    const r = canvas.getBoundingClientRect();
    const x = (e.clientX - r.left) / viewScale;
    const y = (e.clientY - r.top) / viewScale;
    if (x >= 0 && x < LEFT_MARGIN && y >= TOP_MARGIN && y < TOP_MARGIN + nodes.length * CELL_SIZE) {
      const row = Math.floor((y - TOP_MARGIN) / CELL_SIZE);
      showHeaderTip(e, nodes[row]);
      return;
    }
    if (y >= 0 && y < TOP_MARGIN && x >= LEFT_MARGIN && x < LEFT_MARGIN + nodes.length * CELL_SIZE) {
      const col = Math.floor((x - LEFT_MARGIN) / CELL_SIZE);
      showHeaderTip(e, nodes[col]);
      return;
    }
    if (x >= LEFT_MARGIN && y >= TOP_MARGIN) {
      const row = Math.floor((y - TOP_MARGIN) / CELL_SIZE);
      const col = Math.floor((x - LEFT_MARGIN) / CELL_SIZE);
      if (row >= 0 && row < nodes.length && col >= 0 && col < nodes.length) {
        showCellTip(e, nodes[row], nodes[col]);
        return;
      }
    }
    hideTooltip();
  });
  canvas.addEventListener('mouseleave', hideTooltip);

  function showHeaderTip(e, n) {
    if (!n) return hideTooltip();
    tooltip.innerHTML =
      '<div><span class="id">' + escapeHtml(n.id) + '</span></div>' +
      '<span class="name">' + escapeHtml(n.name) + '</span>';
    positionTooltip(e);
  }
  function showCellTip(e, rowN, colN) {
    if (!rowN || !colN) return hideTooltip();
    const edgeType = (edgesBySource[rowN.id] || {})[colN.id] || (edgesBySource[colN.id] || {})[rowN.id];
    let body = '<div><span class="id">' + escapeHtml(rowN.id) + '</span> × <span class="id">' + escapeHtml(colN.id) + '</span></div>';
    body += '<span class="name">' + escapeHtml(rowN.name) + '</span>';
    body += '<span class="name">' + escapeHtml(colN.name) + '</span>';
    if (edgeType) body += '<div style="font-style:italic;color:#c9a45e">' + escapeHtml(edgeType) + '</div>';
    tooltip.innerHTML = body;
    positionTooltip(e);
  }
  function positionTooltip(e) {
    const wrapR = wrap.getBoundingClientRect();
    const x = e.clientX - wrapR.left + wrap.scrollLeft + 12;
    const y = e.clientY - wrapR.top + wrap.scrollTop + 12;
    tooltip.style.left = x + 'px';
    tooltip.style.top  = y + 'px';
    tooltip.classList.add('visible');
  }
  function hideTooltip() { tooltip.classList.remove('visible'); }

  canvas.addEventListener('click', (e) => {
    if (panState.moved) return;
    const r = canvas.getBoundingClientRect();
    const x = (e.clientX - r.left) / viewScale;
    const y = (e.clientY - r.top) / viewScale;
    if (x >= 0 && x < LEFT_MARGIN && y >= TOP_MARGIN && y < TOP_MARGIN + nodes.length * CELL_SIZE) {
      const target = nodes[Math.floor((y - TOP_MARGIN) / CELL_SIZE)];
      if (target) { highlightId = target.id; Shell.selectAtom(target.id); render(); }
    } else if (y >= 0 && y < TOP_MARGIN && x >= LEFT_MARGIN && x < LEFT_MARGIN + nodes.length * CELL_SIZE) {
      const target = nodes[Math.floor((x - LEFT_MARGIN) / CELL_SIZE)];
      if (target) { highlightId = target.id; Shell.selectAtom(target.id); render(); }
    } else if (x >= LEFT_MARGIN && y >= TOP_MARGIN) {
      // Cell click inside the grid. Diagonal → single atom; off-diagonal
      // → pair-detail view (row × col + edge type). The col atom is the
      // "primary" in the detail pane (right-hand far end of the
      // relationship); the row atom is the "paired" atom.
      const row = Math.floor((y - TOP_MARGIN) / CELL_SIZE);
      const col = Math.floor((x - LEFT_MARGIN) / CELL_SIZE);
      if (row >= 0 && row < nodes.length && col >= 0 && col < nodes.length) {
        const rowN = nodes[row];
        const colN = nodes[col];
        if (row === col) {
          highlightId = rowN.id;
          Shell.selectAtom(rowN.id);
          render();
        } else {
          const edgeType = (edgesBySource[rowN.id] || {})[colN.id] || (edgesBySource[colN.id] || {})[rowN.id] || null;
          highlightId = colN.id;
          Shell.selectPair(rowN.id, colN.id, edgeType);
          render();
        }
      }
    }
  });

  document.getElementById('matrix-zoom-in').addEventListener('click', () => setScale(viewScale * 1.4));
  document.getElementById('matrix-zoom-out').addEventListener('click', () => setScale(viewScale / 1.4));
  document.getElementById('matrix-zoom-fit').addEventListener('click', fitToWrap);

  // Intro banner dismiss — persist in localStorage so it stays
  // Matrix intro now lives in the shell-detail placeholder slot.
  wrap.addEventListener('wheel', (e) => {
    if (!(e.ctrlKey || e.metaKey)) return;
    e.preventDefault();
    const factor = e.deltaY < 0 ? 1.15 : (1 / 1.15);
    setScale(viewScale * factor, {x: e.clientX, y: e.clientY});
  }, {passive: false});

  const panState = {active: false, moved: false, sx: 0, sy: 0, ox: 0, oy: 0};
  canvas.addEventListener('mousedown', (e) => {
    if (e.button !== 0) return;
    panState.active = true;
    panState.moved = false;
    panState.sx = e.clientX;
    panState.sy = e.clientY;
    panState.ox = wrap.scrollLeft;
    panState.oy = wrap.scrollTop;
    canvas.classList.add('grabbing');
    hideTooltip();
  });
  window.addEventListener('mousemove', (e) => {
    if (!panState.active) return;
    const dx = e.clientX - panState.sx;
    const dy = e.clientY - panState.sy;
    if (Math.abs(dx) + Math.abs(dy) > 3) panState.moved = true;
    if (panState.moved) {
      wrap.scrollLeft = panState.ox - dx;
      wrap.scrollTop  = panState.oy - dy;
    }
  });
  window.addEventListener('mouseup', () => {
    if (!panState.active) return;
    panState.active = false;
    canvas.classList.remove('grabbing');
    setTimeout(() => { panState.moved = false; }, 0);
  });

  // Two-finger touch: pinch-zoom on the canvas, single-finger pan.
  // touchstart preventDefault suppresses the synthetic mouse events
  // so the mouse path above doesn't double-fire on touch devices.
  let pinchState = null;
  canvas.addEventListener('touchstart', (e) => {
    if (e.touches.length === 2) {
      e.preventDefault();
      const t1 = e.touches[0], t2 = e.touches[1];
      pinchState = {
        startDist: Math.hypot(t2.clientX - t1.clientX, t2.clientY - t1.clientY),
        startScale: viewScale,
        cx: (t1.clientX + t2.clientX) / 2,
        cy: (t1.clientY + t2.clientY) / 2,
      };
    } else if (e.touches.length === 1) {
      e.preventDefault();
      const t = e.touches[0];
      panState.active = true;
      panState.moved = false;
      panState.sx = t.clientX;
      panState.sy = t.clientY;
      panState.ox = wrap.scrollLeft;
      panState.oy = wrap.scrollTop;
    }
  }, {passive: false});
  canvas.addEventListener('touchmove', (e) => {
    if (pinchState && e.touches.length === 2) {
      e.preventDefault();
      const t1 = e.touches[0], t2 = e.touches[1];
      const dist = Math.hypot(t2.clientX - t1.clientX, t2.clientY - t1.clientY);
      setScale(pinchState.startScale * (dist / pinchState.startDist), {x: pinchState.cx, y: pinchState.cy});
    } else if (panState.active && e.touches.length === 1) {
      e.preventDefault();
      const t = e.touches[0];
      const dx = t.clientX - panState.sx;
      const dy = t.clientY - panState.sy;
      if (Math.abs(dx) + Math.abs(dy) > 3) panState.moved = true;
      if (panState.moved) {
        wrap.scrollLeft = panState.ox - dx;
        wrap.scrollTop  = panState.oy - dy;
      }
    }
  }, {passive: false});
  canvas.addEventListener('touchend', (e) => {
    if (e.touches.length === 0) {
      pinchState = null;
      panState.active = false;
      setTimeout(() => { panState.moved = false; }, 0);
    }
  });

  window.MatrixPanel = {
    onSelectionChanged(id, pairedId) {
      highlightId = id;
      pairedHighlightId = pairedId || null;
      render();
      if (id && indexById[id] !== undefined) {
        const i = indexById[id];
        const rowY = (TOP_MARGIN + i * CELL_SIZE) * viewScale;
        const colX = (LEFT_MARGIN + i * CELL_SIZE) * viewScale;
        if (rowY < wrap.scrollTop || rowY > wrap.scrollTop + wrap.clientHeight) {
          wrap.scrollTop = rowY - wrap.clientHeight / 2;
        }
        if (colX < wrap.scrollLeft || colX > wrap.scrollLeft + wrap.clientWidth) {
          wrap.scrollLeft = colX - wrap.clientWidth / 2;
        }
      }
    },
    onFiltersChanged() {
      render();
    },
    fit() { fitToWrap(); },
  };

  render();
  requestAnimationFrame(fitToWrap);
})();`

// matrixStandaloneTemplate wraps MatrixPanelCSS/HTML/JS in a minimal
// page chrome (top-bar with status/search/pivot link) so the legacy
// /matrix.html URL still works for direct access. The standalone
// chrome plays the role of the shell: it owns the status/search
// filters and exposes them through a tiny window.LexiconShell stub.
const matrixStandaloneTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Lexicon — elements (matrix)</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  :root { --bg: #16130f; --bg-panel: #1d1812; --bg-card: #211c16; --border: #322a20; --border-line: #4a3f30; --text: #ece3d4; --text-soft: #d7c9b1; --text-mute: #9c8f7a; --accent: #c9a45e; --accent-mute: #2a2118; --accent-strong: #e0b56b; --gap: #b87a1a; --gap-bg: #2a2418; --gap-border: #4a3f20; --mark-bg: #c9a45e; --mark-fg: #16130f; }
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
    <a href="pivot.html" class="navlink">pivot view →</a>
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
window.LexiconShell = {
  statusFilters: { active: true, underReview: false },
  searchQuery: '',
  selectAtom: function(_id) {},
};
document.getElementById('sa-filter-active').addEventListener('input', e => {
  window.LexiconShell.statusFilters.active = e.target.checked;
  if (window.MatrixPanel) window.MatrixPanel.onFiltersChanged();
});
document.getElementById('sa-filter-under-review').addEventListener('input', e => {
  window.LexiconShell.statusFilters.underReview = e.target.checked;
  if (window.MatrixPanel) window.MatrixPanel.onFiltersChanged();
});
document.getElementById('sa-search').addEventListener('input', e => {
  window.LexiconShell.searchQuery = e.target.value;
  if (window.MatrixPanel) window.MatrixPanel.onFiltersChanged();
});
</script>
<script>{{ .PanelJS }}</script>
</body>
</html>
`

// RenderMatrix emits the standalone adjacency-matrix landing page.
// Same Graph shape used by RenderPivot and RenderShell, so a single
// elements JSON powers all three.
func RenderMatrix(g Graph) ([]byte, error) {
	jsonBytes, err := json.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("marshal graph: %w", err)
	}
	t := template.Must(template.New("matrix").Parse(matrixStandaloneTemplate))
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		PanelCSS  template.CSS
		PanelHTML template.HTML
		PanelJS   template.JS
		DataJSON  template.JS
	}{
		PanelCSS:  template.CSS(MatrixPanelCSS),
		PanelHTML: template.HTML(MatrixPanelHTML),
		PanelJS:   template.JS(MatrixPanelJS),
		DataJSON:  template.JS(jsonBytes),
	}); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
