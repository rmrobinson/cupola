/**
 * Grid — CSS Grid canvas with drag-to-move, resize handles, widget chrome,
 * and auto-save to the active profile after each layout change.
 *
 * Depends on: Stream, Subscriptions, Widgets (from main.js), window.CupolaWidgets
 */
const Grid = (() => {
  const RESIZE_HIT_SIZE = 44;
  const GRID_VERSION = 2;
  const GRID_VERSION_SCALE = 2;
  let _profile = null;
  let _onSave = null;
  let _saveTimer = null;

  // ── Public API ────────────────────────────────────────────────────────

  async function init(profile, onSave) {
    destroy();
    _profile = profile;
    _onSave = onSave;
    const migrated = migrateProfileGrid(_profile);

    const grid = document.getElementById('widget-grid');
    grid.innerHTML = '';
    grid.dataset.layout = profile.layout || 'landscape';
    const displayPositions = layoutDisplayPositions(profile.widgets || [], gridCols(grid));

    for (const wc of (profile.widgets || [])) {
      const cell = await createCell(wc, displayPositions.get(wc.id));
      grid.appendChild(cell);
    }

    if (migrated) scheduleSave();
  }

  function destroy() {
    clearTimeout(_saveTimer);
    _saveTimer = null;
    removeDragOverlay();

    const grid = document.getElementById('widget-grid');
    if (!grid) return;
    grid.querySelectorAll('.widget-cell').forEach(cell => {
      const wc = (_profile?.widgets || []).find(w => w.id === cell.dataset.widgetId);
      const def = wc ? getDefByType(wc.type) : null;
      cleanupCell(cell, wc, def);
    });
    grid.innerHTML = '';
  }

  function addWidget(wc) {
    const grid = document.getElementById('widget-grid');
    const cols = gridCols(grid);
    wc.pos.w = Math.max(1, Math.min(wc.pos.w || 1, cols));
    const occupied = [...layoutDisplayPositions(_profile.widgets || [], cols).values()];
    const free = nextFreePos(wc.pos.w, wc.pos.h, occupied, cols);
    wc.pos.col = free.col;
    wc.pos.row = free.row;
    _profile.widgets.push(wc);
    createCell(wc).then(cell => {
      grid.appendChild(cell);
      scheduleSave();
    });
  }

  function removeWidget(widgetId) {
    const def = getWidgetDef(widgetId);  // capture before profile update
    const removedWc = (_profile.widgets || []).find(w => w.id === widgetId);
    _profile.widgets = (_profile.widgets || []).filter(w => w.id !== widgetId);
    const grid = document.getElementById('widget-grid');
    const cell = grid.querySelector(`[data-widget-id="${widgetId}"]`);
    if (cell) {
      cleanupCell(cell, removedWc, def);
      cell.remove();
    }
    scheduleSave();
  }

  async function refreshState() {
    const grid = document.getElementById('widget-grid');
    if (!grid) return;
    const stateCache = new Map();
    const refreshes = [...grid.querySelectorAll('.widget-cell')]
      .map(cell => cell._refreshState)
      .filter(Boolean)
      .map(fn => fn(stateCache));
    await Promise.allSettled(refreshes);
  }

  // ── Cell creation ─────────────────────────────────────────────────────

  async function createCell(wc, displayPos = null) {
    const def = getDefByType(wc.type);

    const cell = document.createElement('div');
    cell.className = 'widget-cell';
    cell.dataset.widgetId = wc.id;
    cell.dataset.widgetType = wc.type;
    setCellPos(cell, displayPos || wc.pos);

    const inner = document.createElement('div');
    inner.className = 'widget-inner';

    const chrome = document.createElement('div');
    chrome.className = 'widget-chrome';
    chrome.innerHTML = `
      <span class="drag-handle" title="Drag to move">&#8942;&#8942;</span>
      <span class="widget-type-label">${esc(widgetLabel(def, wc.type))}</span>
      <button class="btn-widget-config${(def?.configSchema?.length || def?.buildConfig) ? '' : ' hidden'}" title="Configure">&#9881;</button>
      <button class="btn-widget-remove" title="Remove">&times;</button>
    `;
    chrome.querySelector('.btn-widget-remove').addEventListener('click', () => removeWidget(wc.id));

    const configPanel = document.createElement('div');
    configPanel.className = 'widget-config-panel hidden';

    const content = document.createElement('div');
    content.className = 'widget-content';

    const resizeHandle = document.createElement('div');
    resizeHandle.className = 'widget-resize-handle';
    resizeHandle.title = 'Drag to resize';

    inner.appendChild(chrome);
    inner.appendChild(configPanel);
    inner.appendChild(content);
    inner.appendChild(resizeHandle);
    cell.appendChild(inner);

    initResize(inner, cell, wc, { hitTest: true });
    initResize(resizeHandle, cell, wc);

    chrome.addEventListener('pointerdown', e => {
      if (e.target.closest('button,input,select,textarea,a')) return;
      chrome.setPointerCapture(e.pointerId);
      startPointerDrag(e, cell, wc);
    });

    if (!def) {
      content.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">Unknown widget type: ${esc(wc.type)}</span></div>`;
      return cell;
    }

    // currentState is kept current by the SSE handler so the config panel
    // can re-render the widget after a settings change.
    // For multi-domain widgets (def.domains), currentState is a stateMap object.
    const domainList = def.domains ? def.domains : [def.domain];
    const isMulti = !!def.domains;
    let currentState = isMulti ? {} : null;

    const onConfigSave = () => {
      // Re-register subscription(s) with updated params so parameterised collectors
      // (e.g. transit arrivals) start fetching the newly-selected stop immediately.
      if (def.subscriptionParams) {
        domainList.forEach(d => {
          const subId = isMulti ? `${wc.id}:${d}` : wc.id;
          Subscriptions.create(subId, d, def.subscriptionParams(wc.config));
        });
      }
      const hasState = isMulti ? Object.keys(currentState).length > 0 : !!currentState;
      if (hasState) def.render(content, currentState, wc.config);
      configPanel.classList.add('hidden');
      scheduleSave();
    };
    if (def.buildConfig) {
      // Widget provides its own async config panel builder; rebuild on each open.
      chrome.querySelector('.btn-widget-config').addEventListener('click', () => {
        const wasHidden = configPanel.classList.contains('hidden');
        configPanel.classList.toggle('hidden');
        if (wasHidden) def.buildConfig(configPanel, wc, onConfigSave);
      });
    } else {
      buildConfigPanel(configPanel, def, wc, onConfigSave);
      chrome.querySelector('.btn-widget-config').addEventListener('click', () => {
        configPanel.classList.toggle('hidden');
      });
    }

    const stateMap = {};
    let rendered = false;

    async function refreshWidgetState(stateCache = new Map()) {
      await Promise.all(domainList.map(async d => {
        if (!stateCache.has(d)) {
          stateCache.set(d, fetch(`/api/v1/state/${d}`)
            .then(r => r.ok ? r.json() : undefined)
            .catch(() => undefined));
        }
        const state = await stateCache.get(d);
        if (state !== undefined) stateMap[d] = state;
      }));
      currentState = isMulti ? stateMap : (stateMap[domainList[0]] || null);
      renderCurrentState();
    }

    function renderCurrentState() {
      const hasState = isMulti ? Object.keys(stateMap).length > 0 : !!currentState;
      if (!hasState) {
        renderUnavailable(content, domainList[0]);
        rendered = false;
        return;
      }
      if (rendered) {
        def.onUpdate(content, currentState, wc.config);
      } else {
        def.render(content, currentState, wc.config);
        rendered = true;
      }
    }

    cell._refreshState = refreshWidgetState;

    // Stamp widget identity and an inline-save hook onto the content element.
    // Widgets read these to access their own ID and trigger a profile save
    // without going through the config panel (e.g. "show route on map").
    // _saveConfig is a function property on the element, not a dataset string,
    // because dataset values are string-only and can't hold a function reference.
    content.dataset.widgetId = wc.id;
    content._saveConfig = scheduleSave;

    // Fetch initial state for all domains.
    await refreshWidgetState();

    // Create subscriptions for all domains.
    domainList.forEach(d => {
      const subId = isMulti ? `${wc.id}:${d}` : wc.id;
      const params = def.subscriptionParams ? def.subscriptionParams(wc.config) : null;
      Subscriptions.create(subId, d, params);
    });

    // Set up stream handlers for all domains.
    const streamHandlers = [];
    domainList.forEach(d => {
      const handler = isMulti
        ? (data) => {
            stateMap[d] = data || null;
            currentState = stateMap;
            def.onUpdate(content, stateMap, wc.config);
          }
        : (data) => {
            currentState = data || null;
            if (!data) {
              renderUnavailable(content, d);
              rendered = false;
              return;
            }
            def.onUpdate(content, data, wc.config);
          };
      streamHandlers.push({ domain: d, handler });
      Stream.on(d, handler);
    });
    cell._streamHandlers = streamHandlers;

    return cell;
  }

  function renderUnavailable(container, domain) {
    container.innerHTML = `
      <div class="widget-unavailable">
        <span class="widget-unavailable-label">Source unavailable</span>
        <span style="font-size:10px;opacity:0.5">${esc(domain)}</span>
      </div>
    `;
  }

  // buildConfigPanel populates the config panel element.
  // configSchema entries: { key, label, type, default, options? }
  // Supported types: 'text', 'number', 'boolean', 'select'.
  function buildConfigPanel(panel, def, wc, onSave) {
    const schema = def?.configSchema;
    if (!schema || schema.length === 0) {
      panel.innerHTML = '<p class="config-empty">No settings available.</p>';
      return;
    }

    const fields = schema.map(f => {
      const val = wc.config?.[f.key] ?? f.default ?? '';
      if (f.type === 'boolean') {
        return `<label class="config-row"><span>${esc(f.label)}</span><input type="checkbox" name="${esc(f.key)}"${val ? ' checked' : ''}></label>`;
      }
      if (f.type === 'select') {
        const opts = (f.options || []).map(o =>
          `<option value="${esc(o.value)}"${val === o.value ? ' selected' : ''}>${esc(o.label)}</option>`
        ).join('');
        return `<label class="config-row"><span>${esc(f.label)}</span><select name="${esc(f.key)}">${opts}</select></label>`;
      }
      const inputType = f.type === 'number' ? 'number' : 'text';
      const ph = typeof f.placeholder === 'function' ? f.placeholder() : (f.placeholder ?? '');
      return `<label class="config-row"><span>${esc(f.label)}</span><input type="${inputType}" name="${esc(f.key)}" value="${esc(val)}"${ph !== '' ? ` placeholder="${esc(String(ph))}"` : ''}></label>`;
    }).join('');

    panel.innerHTML = `
      <form class="config-form">
        ${fields}
        <div class="config-actions">
          <button type="submit" class="btn-small btn-primary">Save</button>
          <button type="button" class="btn-small btn-secondary btn-config-cancel">Cancel</button>
        </div>
      </form>
    `;

    panel.querySelector('.btn-config-cancel').addEventListener('click', () => {
      panel.classList.add('hidden');
    });

    panel.querySelector('.config-form').addEventListener('submit', e => {
      e.preventDefault();
      const data = new FormData(e.target);
      wc.config = wc.config || {};
      schema.forEach(f => {
        if (f.type === 'boolean') {
          wc.config[f.key] = data.get(f.key) === 'on';
        } else if (f.type === 'number') {
          const raw = data.get(f.key);
          wc.config[f.key] = raw === '' ? null : Number(raw);
        } else {
          wc.config[f.key] = data.get(f.key) ?? '';
        }
      });
      onSave();
    });
  }

  // ── Drag-and-drop ─────────────────────────────────────────────────────

  function startPointerDrag(e, cell, wc) {
    if (e.button != null && e.button !== 0) return;
    e.preventDefault();
    const grid = document.getElementById('widget-grid');
    const current = effectiveGridPos(cell._gridPos || wc.pos, gridCols(grid));
    wc.pos.col = current.col;
    wc.pos.w = current.w;
    wc.pos.h = current.h;
    setCellPos(cell, current);
    const startX = e.clientX;
    const startY = e.clientY;
    let moved = false;

    const onMove = ev => {
      const dx = ev.clientX - startX;
      const dy = ev.clientY - startY;
      if (!moved && Math.hypot(dx, dy) < 4) return;
      if (!moved) {
        moved = true;
        cell.classList.add('drag-source');
        showDragOverlay(grid, wc);
      }
      updateDragOverlay(ev, grid);
    };
    const onUp = ev => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      document.removeEventListener('pointercancel', onCancel);
      if (moved) {
        const pos = posFromEvent(ev, grid);
        wc.pos.col = pos.col;
        wc.pos.row = pos.row;
        setCellPos(cell, wc.pos);
        scheduleSave();
      }
      cell.classList.remove('drag-source');
      removeDragOverlay();
    };
    const onCancel = () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      document.removeEventListener('pointercancel', onCancel);
      cell.classList.remove('drag-source');
      removeDragOverlay();
    };

    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
    document.addEventListener('pointercancel', onCancel);
  }

  function showDragOverlay(grid, wc) {
    removeDragOverlay();
    const metrics = gridMetrics(grid);

    const overlay = document.createElement('div');
    overlay.id = 'drag-grid-overlay';
    overlay.style.cssText = [
      `position:fixed`,
      `left:${metrics.rect.left}px`,
      `top:${metrics.rect.top}px`,
      `width:${metrics.rect.width}px`,
      `height:${metrics.rect.height}px`,
      `pointer-events:none`,
      `z-index:50`,
      `background-image:` +
        `repeating-linear-gradient(to right,rgba(255,255,255,0.07) 0,rgba(255,255,255,0.07) 1px,transparent 1px,transparent ${metrics.colPitch}px),` +
        `repeating-linear-gradient(to bottom,rgba(255,255,255,0.07) 0,rgba(255,255,255,0.07) 1px,transparent 1px,transparent ${metrics.rowPitch}px)`,
    ].join(';');

    const ghost = document.createElement('div');
    ghost.id = 'drag-drop-ghost';
    ghost.className = 'drag-drop-ghost';
    ghost.style.display = 'none';
    overlay.appendChild(ghost);
    document.body.appendChild(overlay);
    setOverlayGhost(effectiveGridPos(wc.pos, metrics.cols), metrics);
  }

  function updateDragOverlay(e, grid) {
    const metrics = gridMetrics(grid);
    setOverlayGhost({ ...posFromEvent(e, grid), w: 1, h: 1 }, metrics, { keepSize: true });
  }

  function removeDragOverlay() {
    const overlay = document.getElementById('drag-grid-overlay');
    if (overlay) overlay.remove();
  }

  function posFromEvent(e, grid) {
    const metrics = gridMetrics(grid);
    const col = Math.max(0, Math.min(metrics.cols - 1, Math.floor((e.clientX - metrics.rect.left) / metrics.colPitch)));
    const row = Math.max(0, Math.floor((e.clientY - metrics.rect.top) / metrics.rowPitch));
    return { col, row };
  }

  function gridMetrics(grid) {
    const rect = grid.getBoundingClientRect();
    const cols = gridCols(grid);
    const cs = getComputedStyle(grid);
    const rowH    = parseFloat(cs.gridAutoRows) || 60;
    const rowGap  = parseFloat(cs.rowGap)       || 0;
    const colGap  = parseFloat(cs.columnGap)    || 0;
    const rowPitch = rowH + rowGap;
    const colPitch = (rect.width + colGap) / cols;
    return { rect, cols, rowGap, colGap, rowPitch, colPitch };
  }

  function setOverlayGhost(pos, metrics, opts = {}) {
    const ghost = document.getElementById('drag-drop-ghost');
    if (!ghost) return;
    ghost.style.left = `${pos.col * metrics.colPitch}px`;
    ghost.style.top = `${pos.row * metrics.rowPitch}px`;
    if (!opts.keepSize) {
      ghost.style.width = `${pos.w * metrics.colPitch - metrics.colGap}px`;
      ghost.style.height = `${pos.h * metrics.rowPitch - metrics.rowGap}px`;
    }
    ghost.style.display = '';
  }

  // ── Resize ────────────────────────────────────────────────────────────

  function initResize(hitTarget, cell, wc, opts = {}) {
    hitTarget.addEventListener('pointerdown', e => {
      if (opts.hitTest && !isResizeHit(e, hitTarget)) return;
      e.preventDefault();
      e.stopPropagation();
      hitTarget.setPointerCapture(e.pointerId);
      cell.classList.add('widget-resizing');
      const grid = document.getElementById('widget-grid');
      const cols = gridCols(grid);
      const current = effectiveGridPos(cell._gridPos || wc.pos, cols);
      wc.pos.col = current.col;
      wc.pos.w = current.w;
      wc.pos.h = current.h;
      setCellPos(cell, current);
      showDragOverlay(grid, wc);
      const ghost = document.getElementById('drag-drop-ghost');
      if (ghost) ghost.classList.add('drag-drop-ghost-resize');
      const metrics = gridMetrics(grid);
      const startX = e.clientX, startY = e.clientY;
      const startW = current.w, startH = current.h;

      const onMove = e => {
        e.preventDefault();
        const maxW = Math.max(1, cols - Math.min(wc.pos.col, cols - 1));
        wc.pos.w = Math.max(1, Math.min(maxW, startW + Math.round((e.clientX - startX) / metrics.colPitch)));
        wc.pos.h = Math.max(1, startH + Math.round((e.clientY - startY) / metrics.rowPitch));
        setCellPos(cell, wc.pos);
        setOverlayGhost(wc.pos, metrics);
      };
      const onUp = () => {
        document.removeEventListener('pointermove', onMove);
        document.removeEventListener('pointerup', onUp);
        document.removeEventListener('pointercancel', onUp);
        cell.classList.remove('widget-resizing');
        removeDragOverlay();
        scheduleSave();
      };
      document.addEventListener('pointermove', onMove);
      document.addEventListener('pointerup', onUp);
      document.addEventListener('pointercancel', onUp);
    });
  }

  function isResizeHit(e, el) {
    if (e.target.closest('.widget-chrome,.widget-config-panel,button,input,select,textarea,a')) return false;
    const rect = el.getBoundingClientRect();
    return e.clientX >= rect.right - RESIZE_HIT_SIZE &&
      e.clientX <= rect.right &&
      e.clientY >= rect.bottom - RESIZE_HIT_SIZE &&
      e.clientY <= rect.bottom;
  }

  // ── Helpers ───────────────────────────────────────────────────────────

  function nextFreePos(newW, newH, occupiedPositions, cols) {
    const occ = new Set();
    for (const rawPos of occupiedPositions) {
      const pos = effectiveGridPos(rawPos, cols);
      for (let r = pos.row; r < pos.row + pos.h; r++) {
        for (let c = pos.col; c < pos.col + pos.w; c++) {
          occ.add(r + ',' + c);
        }
      }
    }
    const maxCols = Math.max(1, cols - newW + 1);
    for (let row = 0; row < 200; row++) {
      for (let col = 0; col < maxCols; col++) {
        let fits = true;
        outer: for (let r = row; r < row + newH; r++) {
          for (let c = col; c < col + newW; c++) {
            if (occ.has(r + ',' + c)) { fits = false; break outer; }
          }
        }
        if (fits) return { col, row };
      }
    }
    let bottom = 0;
    for (const rawPos of occupiedPositions) {
      const pos = effectiveGridPos(rawPos, cols);
      bottom = Math.max(bottom, pos.row + pos.h);
    }
    return { col: 0, row: bottom };
  }

  function layoutDisplayPositions(widgets, cols) {
    const placed = new Map();
    const occupied = [];
    const ordered = [...widgets].sort((a, b) => {
      const ar = a.pos?.row ?? 0, br = b.pos?.row ?? 0;
      if (ar !== br) return ar - br;
      return (a.pos?.col ?? 0) - (b.pos?.col ?? 0);
    });

    for (const wc of ordered) {
      const desired = effectiveGridPos(wc.pos || {}, cols);
      const pos = firstAvailableAtOrAfter(desired, occupied, cols);
      placed.set(wc.id, pos);
      occupied.push(pos);
    }
    return placed;
  }

  function firstAvailableAtOrAfter(desired, occupied, cols) {
    let row = desired.row;
    const maxCol = Math.max(0, cols - desired.w);
    while (row < 2000) {
      const startCol = row === desired.row ? Math.min(desired.col, maxCol) : 0;
      for (let col = startCol; col <= maxCol; col++) {
        const candidate = { ...desired, row, col };
        if (!overlapsAny(candidate, occupied)) return candidate;
      }
      row++;
    }
    return nextFreePos(desired.w, desired.h, occupied, cols);
  }

  function overlapsAny(pos, occupied) {
    return occupied.some(other =>
      pos.col < other.col + other.w &&
      pos.col + pos.w > other.col &&
      pos.row < other.row + other.h &&
      pos.row + pos.h > other.row
    );
  }

  function effectiveGridPos(pos, cols) {
    const col = Math.max(0, Math.min(pos.col ?? 0, cols - 1));
    const w = Math.max(1, Math.min(pos.w || 1, cols - col));
    return {
      col,
      row: Math.max(0, pos.row ?? 0),
      w,
      h: Math.max(1, pos.h || 1),
    };
  }

  function migrateProfileGrid(profile) {
    if (!profile || Number(profile.grid_version || 1) >= GRID_VERSION) return false;
    (profile.widgets || []).forEach(wc => {
      if (!wc.pos) return;
      wc.pos.col = Math.max(0, (wc.pos.col || 0) * GRID_VERSION_SCALE);
      wc.pos.w = Math.max(1, (wc.pos.w || 1) * GRID_VERSION_SCALE);
    });
    profile.grid_version = GRID_VERSION;
    return true;
  }

  function setCellPos(cell, pos) {
    const next = {
      col: pos.col ?? 0,
      row: pos.row ?? 0,
      w: pos.w || 1,
      h: pos.h || 1,
    };
    cell._gridPos = next;
    cell.style.gridColumn = `${next.col + 1} / span ${next.w}`;
    cell.style.gridRow    = `${next.row + 1} / span ${next.h}`;
  }

  function gridCols(grid) {
    const tracks = getComputedStyle(grid).gridTemplateColumns
      .split(' ')
      .filter(Boolean).length;
    return tracks || layoutCols(grid.dataset.layout);
  }

  function layoutCols(layout) {
    return layout === 'portrait' ? 8 : 24;
  }

  function getDefByType(type) {
    return (window.CupolaWidgets || []).find(w => w.type === type) || null;
  }

  function getWidgetDef(widgetId) {
    const wc = (_profile?.widgets || []).find(w => w.id === widgetId);
    return wc ? getDefByType(wc.type) : null;
  }

  function humanLabel(type) {
    return type.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
  }

  function widgetLabel(def, type) {
    return def?.label || humanLabel(type);
  }

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function scheduleSave() {
    clearTimeout(_saveTimer);
    _saveTimer = setTimeout(() => {
      if (_onSave) _onSave(_profile);
    }, 400);
  }

  function cleanupCell(cell, wc, def) {
    clearInterval(Number(cell.dataset.tickInterval));
    const widgetId = cell.dataset.widgetId;
    if (def) {
      const domainList = def.domains ? def.domains : [def.domain];
      const isMulti = !!def.domains;
      domainList.forEach(d => Subscriptions.remove(isMulti ? `${widgetId}:${d}` : widgetId));
      if (def.onRemove) {
        const content = cell.querySelector('.widget-content');
        if (content) def.onRemove(content, wc?.config);
      }
    } else if (widgetId) {
      Subscriptions.remove(widgetId);
    }
    (cell._streamHandlers || []).forEach(({ domain, handler }) => Stream.off(domain, handler));
    cell._streamHandlers = [];
    cell._refreshState = null;
  }

  return { init, addWidget, removeWidget, destroy, refreshState };
})();
