/**
 * Grid — CSS Grid canvas with drag-to-move, resize handles, widget chrome,
 * and auto-save to the active profile after each layout change.
 *
 * Depends on: Stream, Subscriptions, Widgets (from main.js), window.CupolaWidgets
 */
const Grid = (() => {
  let _profile = null;
  let _onSave = null;
  let _saveTimer = null;

  // ── Public API ────────────────────────────────────────────────────────

  async function init(profile, onSave) {
    destroy();
    _profile = profile;
    _onSave = onSave;

    const grid = document.getElementById('widget-grid');
    grid.innerHTML = '';
    grid.dataset.layout = profile.layout || 'landscape';

    for (const wc of (profile.widgets || [])) {
      const cell = await createCell(wc);
      grid.appendChild(cell);
    }
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
    const cols = (_profile.layout === 'portrait') ? 4 : 12;
    const free = nextFreePos(wc.pos.w, wc.pos.h, _profile.widgets, cols);
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

  // ── Cell creation ─────────────────────────────────────────────────────

  async function createCell(wc) {
    const def = getDefByType(wc.type);

    const cell = document.createElement('div');
    cell.className = 'widget-cell';
    cell.dataset.widgetId = wc.id;
    cell.dataset.widgetType = wc.type;
    setCellPos(cell, wc.pos);

    const inner = document.createElement('div');
    inner.className = 'widget-inner';

    const chrome = document.createElement('div');
    chrome.className = 'widget-chrome';
    chrome.innerHTML = `
      <span class="drag-handle" title="Drag to move">&#8942;&#8942;</span>
      <span class="widget-type-label">${esc(humanLabel(wc.type))}</span>
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

    initResize(resizeHandle, cell, wc);

    const dragHandle = chrome.querySelector('.drag-handle');
    dragHandle.addEventListener('pointerdown', e => {
      e.currentTarget.setPointerCapture(e.pointerId);
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

    // Fetch initial state for all domains.
    const stateMap = {};
    for (const d of domainList) {
      try {
        const r = await fetch(`/api/v1/state/${d}`);
        if (r.ok) stateMap[d] = await r.json();
      } catch {}
    }
    currentState = isMulti ? stateMap : (stateMap[domainList[0]] || null);

    // Create subscriptions for all domains.
    domainList.forEach(d => {
      const subId = isMulti ? `${wc.id}:${d}` : wc.id;
      const params = def.subscriptionParams ? def.subscriptionParams(wc.config) : null;
      Subscriptions.create(subId, d, params);
    });

    // Stamp widget identity and an inline-save hook onto the content element.
    // Widgets read these to access their own ID and trigger a profile save
    // without going through the config panel (e.g. "show route on map").
    // _saveConfig is a function property on the element, not a dataset string,
    // because dataset values are string-only and can't hold a function reference.
    content.dataset.widgetId = wc.id;
    content._saveConfig = scheduleSave;

    const hasState = isMulti ? Object.keys(stateMap).length > 0 : !!currentState;
    if (hasState) {
      def.render(content, currentState, wc.config);
    } else {
      renderUnavailable(content, domainList[0]);
    }

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
            if (!data) { renderUnavailable(content, d); return; }
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
    const rect = grid.getBoundingClientRect();
    const cols = (grid.dataset.layout === 'portrait') ? 4 : 12;
    const cs = getComputedStyle(grid);
    const rowH    = parseFloat(cs.gridAutoRows) || 60;
    const rowGap  = parseFloat(cs.rowGap)       || 0;
    const colGap  = parseFloat(cs.columnGap)    || 0;
    const rowPitch = rowH + rowGap;
    const colPitch = (rect.width + colGap) / cols;

    const overlay = document.createElement('div');
    overlay.id = 'drag-grid-overlay';
    overlay.style.cssText = [
      `position:fixed`,
      `left:${rect.left}px`,
      `top:${rect.top}px`,
      `width:${rect.width}px`,
      `height:${rect.height}px`,
      `pointer-events:none`,
      `z-index:50`,
      `background-image:` +
        `repeating-linear-gradient(to right,rgba(255,255,255,0.07) 0,rgba(255,255,255,0.07) 1px,transparent 1px,transparent ${colPitch}px),` +
        `repeating-linear-gradient(to bottom,rgba(255,255,255,0.07) 0,rgba(255,255,255,0.07) 1px,transparent 1px,transparent ${rowPitch}px)`,
    ].join(';');

    const ghost = document.createElement('div');
    ghost.id = 'drag-drop-ghost';
    ghost.className = 'drag-drop-ghost';
    ghost.style.width  = `${wc.pos.w * colPitch - colGap}px`;
    ghost.style.height = `${wc.pos.h * rowPitch - rowGap}px`;
    ghost.style.display = 'none';
    overlay.appendChild(ghost);
    document.body.appendChild(overlay);
  }

  function updateDragOverlay(e, grid) {
    const ghost = document.getElementById('drag-drop-ghost');
    if (!ghost) return;
    const rect = grid.getBoundingClientRect();
    const cols = (grid.dataset.layout === 'portrait') ? 4 : 12;
    const cs = getComputedStyle(grid);
    const rowH    = parseFloat(cs.gridAutoRows) || 60;
    const rowGap  = parseFloat(cs.rowGap)       || 0;
    const colGap  = parseFloat(cs.columnGap)    || 0;
    const rowPitch = rowH + rowGap;
    const colPitch = (rect.width + colGap) / cols;
    const col = Math.max(0, Math.min(cols - 1, Math.floor((e.clientX - rect.left) / colPitch)));
    const row = Math.max(0, Math.floor((e.clientY - rect.top) / rowPitch));
    ghost.style.left    = `${col * colPitch}px`;
    ghost.style.top     = `${row * rowPitch}px`;
    ghost.style.display = '';
  }

  function removeDragOverlay() {
    const overlay = document.getElementById('drag-grid-overlay');
    if (overlay) overlay.remove();
  }

  function posFromEvent(e, grid) {
    const rect = grid.getBoundingClientRect();
    const cols = (grid.dataset.layout === 'portrait') ? 4 : 12;
    const cs = getComputedStyle(grid);
    const rowH    = parseFloat(cs.gridAutoRows) || 60;
    const rowGap  = parseFloat(cs.rowGap)       || 0;
    const colGap  = parseFloat(cs.columnGap)    || 0;
    const rowPitch = rowH + rowGap;
    const colPitch = (rect.width + colGap) / cols;
    const col = Math.max(0, Math.min(cols - 1, Math.floor((e.clientX - rect.left) / colPitch)));
    const row = Math.max(0, Math.floor((e.clientY - rect.top) / rowPitch));
    return { col, row };
  }

  // ── Resize ────────────────────────────────────────────────────────────

  function initResize(handle, cell, wc) {
    handle.addEventListener('pointerdown', e => {
      e.preventDefault();
      e.stopPropagation();
      handle.setPointerCapture(e.pointerId);
      const grid = document.getElementById('widget-grid');
      const cols = (grid.dataset.layout === 'portrait') ? 4 : 12;
      const rect = grid.getBoundingClientRect();
      const colW = rect.width / cols;
      const rowH = parseFloat(getComputedStyle(grid).gridAutoRows) || 60;
      const startX = e.clientX, startY = e.clientY;
      const startW = wc.pos.w, startH = wc.pos.h;

      const onMove = e => {
        wc.pos.w = Math.max(1, Math.min(cols - wc.pos.col, startW + Math.round((e.clientX - startX) / colW)));
        wc.pos.h = Math.max(1, startH + Math.round((e.clientY - startY) / rowH));
        setCellPos(cell, wc.pos);
      };
      const onUp = () => {
        document.removeEventListener('pointermove', onMove);
        document.removeEventListener('pointerup', onUp);
        document.removeEventListener('pointercancel', onUp);
        scheduleSave();
      };
      document.addEventListener('pointermove', onMove);
      document.addEventListener('pointerup', onUp);
      document.addEventListener('pointercancel', onUp);
    });
  }

  // ── Helpers ───────────────────────────────────────────────────────────

  function nextFreePos(newW, newH, widgets, cols) {
    const occ = new Set();
    for (const wc of widgets) {
      for (let r = wc.pos.row; r < wc.pos.row + wc.pos.h; r++) {
        for (let c = wc.pos.col; c < wc.pos.col + wc.pos.w; c++) {
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
    for (const wc of widgets) bottom = Math.max(bottom, wc.pos.row + wc.pos.h);
    return { col: 0, row: bottom };
  }

  function setCellPos(cell, pos) {
    cell.style.gridColumn = `${(pos.col ?? 0) + 1} / span ${pos.w || 1}`;
    cell.style.gridRow    = `${(pos.row ?? 0) + 1} / span ${pos.h || 1}`;
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

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
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
  }

  return { init, addWidget, removeWidget, destroy };
})();
