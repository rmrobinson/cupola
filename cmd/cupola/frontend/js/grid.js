/**
 * Grid — CSS Grid canvas with drag-to-move, resize handles, widget chrome,
 * and auto-save to the active profile after each layout change.
 *
 * Depends on: Stream, Subscriptions, Widgets (from main.js), window.CupolaWidgets
 */
const Grid = (() => {
  let _profile = null;
  let _onSave = null;
  let _draggedId = null;
  let _saveTimer = null;

  // ── Public API ────────────────────────────────────────────────────────

  async function init(profile, onSave) {
    _profile = profile;
    _onSave = onSave;

    const grid = document.getElementById('widget-grid');
    grid.innerHTML = '';
    grid.dataset.layout = profile.layout || 'landscape';

    for (const wc of (profile.widgets || [])) {
      const cell = await createCell(wc);
      grid.appendChild(cell);
    }
    initDragDrop(grid);
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
    _profile.widgets = (_profile.widgets || []).filter(w => w.id !== widgetId);
    const grid = document.getElementById('widget-grid');
    const cell = grid.querySelector(`[data-widget-id="${widgetId}"]`);
    if (cell) {
      clearInterval(Number(cell.dataset.tickInterval));
      Subscriptions.remove(widgetId);
      if (def) Stream.off(def.domain, cell._streamHandler);
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
    cell.setAttribute('draggable', 'true');

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

    cell.addEventListener('dragstart', e => {
      _draggedId = wc.id;
      e.dataTransfer.effectAllowed = 'move';
      setTimeout(() => cell.classList.add('drag-source'), 0);
    });
    cell.addEventListener('dragend', () => {
      _draggedId = null;
      cell.classList.remove('drag-source');
    });

    if (!def) {
      content.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">Unknown widget type: ${esc(wc.type)}</span></div>`;
      return cell;
    }

    // currentState is kept current by the SSE handler so the config panel
    // can re-render the widget after a settings change.
    let currentState = null;

    const onConfigSave = () => {
      // Re-register subscription with updated params so parameterised collectors
      // (e.g. transit arrivals) start fetching the newly-selected stop immediately.
      if (def.subscriptionParams) {
        Subscriptions.create(wc.id, def.domain, def.subscriptionParams(wc.config));
      }
      if (currentState) def.render(content, currentState, wc.config);
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

    let state = null;
    try {
      const r = await fetch(`/api/v1/state/${def.domain}`);
      if (r.ok) state = await r.json();
    } catch {}
    currentState = state;

    const params = def.subscriptionParams ? def.subscriptionParams(wc.config) : null;
    Subscriptions.create(wc.id, def.domain, params);

    if (state) {
      def.render(content, state, wc.config);
    } else {
      renderUnavailable(content, def.domain);
    }

    const handler = (data) => {
      currentState = data || null;
      if (!data) { renderUnavailable(content, def.domain); return; }
      def.onUpdate(content, data, wc.config);
    };
    cell._streamHandler = handler;
    Stream.on(def.domain, handler);

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
      return `<label class="config-row"><span>${esc(f.label)}</span><input type="${inputType}" name="${esc(f.key)}" value="${esc(val)}"></label>`;
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
          wc.config[f.key] = Number(data.get(f.key));
        } else {
          wc.config[f.key] = data.get(f.key) ?? '';
        }
      });
      onSave();
    });
  }

  // ── Drag-and-drop ─────────────────────────────────────────────────────

  function initDragDrop(grid) {
    grid.addEventListener('dragover', e => {
      if (!_draggedId) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
    });

    grid.addEventListener('drop', e => {
      if (!_draggedId) return;
      e.preventDefault();
      const pos = posFromEvent(e, grid);
      const wc = _profile.widgets.find(w => w.id === _draggedId);
      if (!wc) return;
      wc.pos.col = pos.col;
      wc.pos.row = pos.row;
      const cell = grid.querySelector(`[data-widget-id="${_draggedId}"]`);
      if (cell) setCellPos(cell, wc.pos);
      scheduleSave();
    });
  }

  function posFromEvent(e, grid) {
    const rect = grid.getBoundingClientRect();
    const cols = (grid.dataset.layout === 'portrait') ? 4 : 12;
    const rowH = parseFloat(getComputedStyle(grid).gridAutoRows) || 60;
    const col = Math.max(0, Math.min(cols - 1, Math.floor((e.clientX - rect.left) / (rect.width / cols))));
    const row = Math.max(0, Math.floor((e.clientY - rect.top) / rowH));
    return { col, row };
  }

  // ── Resize ────────────────────────────────────────────────────────────

  function initResize(handle, cell, wc) {
    handle.addEventListener('mousedown', e => {
      e.preventDefault();
      e.stopPropagation();
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
        document.removeEventListener('mousemove', onMove);
        document.removeEventListener('mouseup', onUp);
        scheduleSave();
      };
      document.addEventListener('mousemove', onMove);
      document.addEventListener('mouseup', onUp);
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

  return { init, addWidget, removeWidget };
})();
