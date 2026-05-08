const TransitAdmin = (() => {
  const API = '/api/v1/transit/agency-configs';
  let root = null;
  let agencies = [];
  let editingID = null;
  let previousFocus = null;
  let keyHandler = null;
  const pendingToggles = new Set();

  function init() {
    root = document.getElementById('transit-admin');
    const btn = document.getElementById('btn-transit-admin');
    if (!root || !btn || btn.dataset.bound) return;
    btn.dataset.bound = '1';
    btn.addEventListener('click', () => open());
  }

  async function open() {
    previousFocus = document.activeElement;
    root.classList.remove('hidden');
    root.setAttribute('aria-hidden', 'false');
    root.innerHTML = shellHTML('<p class="admin-loading">Loading feeds...</p>');
    bindShell();
    await load();
    focusFirst();
  }

  function close() {
    root.classList.add('hidden');
    root.setAttribute('aria-hidden', 'true');
    root.innerHTML = '';
    editingID = null;
    if (keyHandler) {
      document.removeEventListener('keydown', keyHandler);
      keyHandler = null;
    }
    if (previousFocus && document.contains(previousFocus)) {
      previousFocus.focus();
    }
    previousFocus = null;
  }

  async function load() {
    try {
      const res = await fetch(API);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      agencies = await res.json();
      render();
    } catch (err) {
      AppUI.reportError('Transit feeds failed to load', err);
      setBody('<p class="admin-empty">Transit feeds failed to load.</p>');
    }
  }

  function render() {
    const list = agencies.length
      ? `<div class="gtfs-agency-list">${agencies.map(agencyRow).join('')}</div>`
      : '<p class="admin-empty">No GTFS feeds configured.</p>';
    setBody(`
      <div class="admin-split">
        <section class="admin-list-pane">
          <div class="admin-list-toolbar">
            <h2 class="admin-section-title">Agencies</h2>
            <button type="button" class="btn-small btn-admin-new">+ Agency</button>
          </div>
          ${list}
        </section>
        <section class="admin-editor-pane">
          ${editorHTML(null)}
        </section>
      </div>
    `);
    bindList();
    bindEditor(null);
  }

  function shellHTML(body) {
    return `
      <div class="admin-backdrop"></div>
      <div class="admin-panel" role="dialog" aria-modal="true" aria-label="Transit GTFS feeds">
        <header class="admin-header">
          <h1 class="admin-title">GTFS Feeds</h1>
          <button type="button" class="btn-small btn-admin-close">Close</button>
        </header>
        <div class="admin-body">${body}</div>
      </div>
    `;
  }

  function setBody(html) {
    const body = root.querySelector('.admin-body');
    if (body) body.innerHTML = html;
  }

  function agencyRow(agency) {
    const enabled = agency.enabled !== false;
    const pending = pendingToggles.has(agency.id);
    return `
      <article class="gtfs-agency-row" data-id="${esc(agency.id)}">
        <div class="gtfs-agency-main">
          <div class="gtfs-agency-id">${esc(agency.id)}</div>
          <div class="gtfs-agency-meta">${urlCountLabel(agency)}</div>
        </div>
        <label class="switch" title="${enabled ? 'Disable agency' : 'Enable agency'}">
          <input type="checkbox" class="gtfs-enable-toggle"
                 aria-label="${esc(enabled ? `Disable agency ${agency.id}` : `Enable agency ${agency.id}`)}"
                 ${enabled ? 'checked' : ''}${pending ? ' disabled' : ''}>
          <span></span>
        </label>
        <button type="button" class="btn-small btn-admin-edit"${pending ? ' disabled' : ''}>Edit</button>
        <button type="button" class="btn-small btn-admin-delete"${pending ? ' disabled' : ''}>Delete</button>
      </article>
    `;
  }

  function urlCountLabel(agency) {
    const staticCount = agency.gtfs_static_urls?.length || 0;
    const tripCount = agency.gtfs_rt_trip_updates_urls?.length || 0;
    const vehicleCount = agency.gtfs_rt_vehicle_positions_urls?.length || 0;
    const alertCount = agency.gtfs_rt_alerts_url ? 1 : 0;
    return `${staticCount} static, ${tripCount} trips, ${vehicleCount} vehicles, ${alertCount} alerts`;
  }

  function editorHTML(agency) {
    const isEdit = !!agency;
    const enabled = agency?.enabled !== false;
    return `
      <form class="gtfs-editor">
        <div class="admin-editor-head">
          <h2 class="admin-section-title">${isEdit ? 'Edit Agency' : 'New Agency'}</h2>
        </div>
        <label class="admin-field">
          <span>Agency ID</span>
          <input name="id" type="text" value="${esc(agency?.id || '')}" ${isEdit ? 'readonly' : ''} required>
        </label>
        <label class="admin-check-row">
          <input name="enabled" type="checkbox" ${enabled ? 'checked' : ''}>
          <span>Enabled</span>
        </label>
        <label class="admin-field">
          <span>Static GTFS URLs</span>
          <textarea name="gtfs_static_urls" rows="4">${esc((agency?.gtfs_static_urls || []).join('\n'))}</textarea>
        </label>
        <label class="admin-field">
          <span>Trip updates URLs (optional)</span>
          <textarea name="gtfs_rt_trip_updates_urls" rows="4">${esc((agency?.gtfs_rt_trip_updates_urls || []).join('\n'))}</textarea>
        </label>
        <label class="admin-field">
          <span>Vehicle positions URLs</span>
          <textarea name="gtfs_rt_vehicle_positions_urls" rows="3">${esc((agency?.gtfs_rt_vehicle_positions_urls || []).join('\n'))}</textarea>
        </label>
        <label class="admin-field">
          <span>Alerts URL</span>
          <input name="gtfs_rt_alerts_url" type="url" value="${esc(agency?.gtfs_rt_alerts_url || '')}">
        </label>
        <div class="admin-form-error hidden"></div>
        <div class="admin-form-actions">
          <button type="submit" class="btn-primary">${isEdit ? 'Save' : 'Create'}</button>
          ${isEdit ? '<button type="button" class="btn-secondary btn-admin-cancel-edit">Cancel</button>' : ''}
        </div>
      </form>
    `;
  }

  function bindShell() {
    root.querySelector('.btn-admin-close')?.addEventListener('click', close);
    root.querySelector('.admin-backdrop')?.addEventListener('click', close);
    if (keyHandler) document.removeEventListener('keydown', keyHandler);
    keyHandler = e => {
      if (root.classList.contains('hidden')) return;
      if (e.key === 'Escape') {
        close();
        return;
      }
      if (e.key === 'Tab') trapFocus(e);
    };
    document.addEventListener('keydown', keyHandler);
  }

  function bindList() {
    root.querySelector('.btn-admin-new')?.addEventListener('click', () => {
      editingID = null;
      const pane = root.querySelector('.admin-editor-pane');
      pane.innerHTML = editorHTML(null);
      bindEditor(null);
      pane.querySelector('[name="id"]')?.focus();
    });

    root.querySelectorAll('.gtfs-agency-row').forEach(row => {
      const id = row.dataset.id;
      row.querySelector('.btn-admin-edit')?.addEventListener('click', () => edit(id));
      row.querySelector('.btn-admin-delete')?.addEventListener('click', () => remove(id));
      row.querySelector('.gtfs-enable-toggle')?.addEventListener('change', e => {
        setEnabled(id, e.target.checked);
      });
    });
  }

  function bindEditor(agency) {
    const form = root.querySelector('.gtfs-editor');
    if (!form) return;
    form.querySelector('.btn-admin-cancel-edit')?.addEventListener('click', () => {
      editingID = null;
      const pane = root.querySelector('.admin-editor-pane');
      pane.innerHTML = editorHTML(null);
      bindEditor(null);
    });
    form.addEventListener('submit', e => save(e, agency));
  }

  function edit(id) {
    const agency = agencies.find(a => a.id === id);
    if (!agency) return;
    editingID = id;
    const pane = root.querySelector('.admin-editor-pane');
    pane.innerHTML = editorHTML(agency);
    bindEditor(agency);
    pane.querySelector('[name="gtfs_static_urls"]')?.focus();
  }

  async function setEnabled(id, enabled) {
    if (pendingToggles.has(id)) return;
    pendingToggles.add(id);
    render();
    try {
      const res = await fetch(`${API}/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled }),
      });
      if (!res.ok) throw new Error(await responseError(res));
      AppUI.notify(enabled ? 'Transit agency enabled' : 'Transit agency disabled');
      await load();
    } catch (err) {
      AppUI.reportError('Transit agency update failed', err);
      await load();
    } finally {
      pendingToggles.delete(id);
      render();
    }
  }

  async function remove(id) {
    if (!window.confirm(`Delete GTFS agency ${id}?`)) return;
    try {
      const res = await fetch(`${API}/${encodeURIComponent(id)}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(await responseError(res));
      AppUI.notify('Transit agency deleted');
      editingID = null;
      await load();
    } catch (err) {
      AppUI.reportError('Transit agency delete failed', err);
    }
  }

  async function save(evt, current) {
    evt.preventDefault();
    const form = evt.currentTarget;
    if (form.dataset.pending === '1') return;
    const payload = formPayload(form);
    const error = validatePayload(payload);
    if (error) {
      showFormError(form, error);
      return;
    }

    const id = current?.id || payload.id;
    const isEdit = !!current;
    try {
      setFormPending(form, true);
      const res = await fetch(isEdit ? `${API}/${encodeURIComponent(id)}` : API, {
        method: isEdit ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error(await responseError(res));
      AppUI.notify(isEdit ? 'Transit agency saved' : 'Transit agency created');
      editingID = id;
      await load();
      if (editingID) edit(editingID);
    } catch (err) {
      showFormError(form, err.message || 'Save failed');
      AppUI.reportError('Transit agency save failed', err);
    } finally {
      if (document.contains(form)) setFormPending(form, false);
    }
  }

  function formPayload(form) {
    const data = new FormData(form);
    return {
      id: String(data.get('id') || '').trim(),
      enabled: data.get('enabled') === 'on',
      gtfs_static_urls: lines(data.get('gtfs_static_urls')),
      gtfs_rt_trip_updates_urls: lines(data.get('gtfs_rt_trip_updates_urls')),
      gtfs_rt_vehicle_positions_urls: lines(data.get('gtfs_rt_vehicle_positions_urls')),
      gtfs_rt_alerts_url: String(data.get('gtfs_rt_alerts_url') || '').trim(),
    };
  }

  function validatePayload(payload) {
    if (!/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(payload.id)) {
      return 'Invalid agency ID.';
    }
    if (payload.enabled && payload.gtfs_static_urls.length === 0) {
      return 'Enabled agencies require static GTFS URLs.';
    }
    const urls = [
      ...payload.gtfs_static_urls,
      ...payload.gtfs_rt_trip_updates_urls,
      ...payload.gtfs_rt_vehicle_positions_urls,
    ];
    if (payload.gtfs_rt_alerts_url) urls.push(payload.gtfs_rt_alerts_url);
    if (urls.some(url => !isHTTPURL(url))) {
      return 'Feed URLs must be absolute http or https URLs.';
    }
    return null;
  }

  function isHTTPURL(raw) {
    try {
      const url = new URL(raw);
      return !!url.host && (url.protocol === 'http:' || url.protocol === 'https:');
    } catch {
      return false;
    }
  }

  function lines(value) {
    return String(value || '')
      .split(/\r?\n/)
      .map(s => s.trim())
      .filter(Boolean);
  }

  async function responseError(res) {
    const text = await res.text();
    return text.trim() || `HTTP ${res.status}`;
  }

  function showFormError(form, message) {
    const el = form.querySelector('.admin-form-error');
    if (!el) return;
    el.textContent = message;
    el.classList.remove('hidden');
  }

  function setFormPending(form, pending) {
    form.dataset.pending = pending ? '1' : '0';
    form.querySelectorAll('button, input, textarea').forEach(el => {
      el.disabled = pending;
    });
  }

  function focusFirst() {
    const first = root.querySelector('.btn-admin-close, button, input, textarea, [tabindex]:not([tabindex="-1"])');
    first?.focus();
  }

  function trapFocus(e) {
    const focusable = [...root.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')]
      .filter(el => !el.disabled && el.offsetParent !== null);
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  function esc(s) {
    return String(s || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  document.addEventListener('DOMContentLoaded', init);

  return { open };
})();
