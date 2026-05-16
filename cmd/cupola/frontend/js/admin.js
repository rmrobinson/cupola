const Admin = (() => {
  const API = {
    gtfs: '/api/v1/transit/agency-configs',
    collectors: '/api/v1/admin/collectors',
  };

  let panel = null;
  let activeTab = 'gtfs';
  let agencies = [];
  let editingID = null;
  let mutationPending = false;
  const pendingToggles = new Set();

  function init() {
    panel = document.getElementById('admin-panel');
    document.querySelectorAll('.admin-nav-item').forEach(btn => {
      btn.addEventListener('click', () => selectTab(btn.dataset.tab));
    });
    selectTab(activeTab);
  }

  function selectTab(tab) {
    activeTab = tab || 'gtfs';
    document.querySelectorAll('.admin-nav-item').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.tab === activeTab);
    });

    if (activeTab === 'dashboards') {
      loadDashboards();
    } else if (activeTab === 'collectors') {
      loadCollectors();
    } else {
      loadGTFS();
    }
  }

  async function loadGTFS() {
    panel.innerHTML = loadingHTML('Loading feeds...');
    try {
      const res = await fetch(API.gtfs);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      agencies = await res.json();
      renderGTFS();
    } catch (err) {
      AppUI.reportError('Transit feeds failed to load', err);
      panel.innerHTML = emptyHTML('Transit feeds failed to load.');
    }
  }

  function renderGTFS() {
    const list = agencies.length
      ? `<div class="gtfs-agency-list">${agencies.map(agencyRow).join('')}</div>`
      : '<p class="admin-empty">No GTFS feeds configured.</p>';
    panel.innerHTML = `
      <div class="admin-page-head">
        <h2 class="admin-page-title">GTFS Feeds</h2>
      </div>
      <div class="admin-split">
        <section class="admin-list-pane">
          <div class="admin-list-toolbar">
            <h3 class="admin-section-title">Agencies</h3>
            <button type="button" class="btn-small btn-admin-new">+ Agency</button>
          </div>
          ${list}
        </section>
        <section class="admin-editor-pane">
          ${editorHTML(null)}
        </section>
      </div>
    `;
    bindGTFSList();
    bindEditor(null);
    applyMutationState();
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
          <h3 class="admin-section-title">${isEdit ? 'Edit Agency' : 'New Agency'}</h3>
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
          <span>Trip updates URLs</span>
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

  function bindGTFSList() {
    panel.querySelector('.btn-admin-new')?.addEventListener('click', () => {
      editingID = null;
      const pane = panel.querySelector('.admin-editor-pane');
      pane.innerHTML = editorHTML(null);
      bindEditor(null);
      pane.querySelector('[name="id"]')?.focus();
    });

    panel.querySelectorAll('.gtfs-agency-row').forEach(row => {
      const id = row.dataset.id;
      row.querySelector('.btn-admin-edit')?.addEventListener('click', () => editAgency(id));
      row.querySelector('.btn-admin-delete')?.addEventListener('click', () => removeAgency(id));
      row.querySelector('.gtfs-enable-toggle')?.addEventListener('change', e => {
        setAgencyEnabled(id, e.target.checked);
      });
    });
  }

  function bindEditor(agency) {
    const form = panel.querySelector('.gtfs-editor');
    if (!form) return;
    form.querySelector('.btn-admin-cancel-edit')?.addEventListener('click', () => {
      editingID = null;
      const pane = panel.querySelector('.admin-editor-pane');
      pane.innerHTML = editorHTML(null);
      bindEditor(null);
    });
    form.addEventListener('submit', e => saveAgency(e, agency));
  }

  function editAgency(id) {
    const agency = agencies.find(a => a.id === id);
    if (!agency) return;
    editingID = id;
    const pane = panel.querySelector('.admin-editor-pane');
    pane.innerHTML = editorHTML(agency);
    bindEditor(agency);
    pane.querySelector('[name="gtfs_static_urls"]')?.focus();
  }

  async function setAgencyEnabled(id, enabled) {
    if (pendingToggles.has(id) || !beginMutation()) return;
    pendingToggles.add(id);
    renderGTFS();
    try {
      const res = await fetch(`${API.gtfs}/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled }),
      });
      if (!res.ok) throw new Error(await responseError(res));
      AppUI.notify(enabled ? 'Transit agency enabled' : 'Transit agency disabled');
      await loadGTFS();
    } catch (err) {
      AppUI.reportError('Transit agency update failed', err);
      await loadGTFS();
    } finally {
      pendingToggles.delete(id);
      endMutation();
    }
  }

  async function removeAgency(id) {
    if (!window.confirm(`Delete GTFS agency ${id}?`)) return;
    if (!beginMutation()) return;
    try {
      const res = await fetch(`${API.gtfs}/${encodeURIComponent(id)}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(await responseError(res));
      AppUI.notify('Transit agency deleted');
      editingID = null;
      await loadGTFS();
    } catch (err) {
      AppUI.reportError('Transit agency delete failed', err);
    } finally {
      endMutation();
    }
  }

  async function saveAgency(evt, current) {
    evt.preventDefault();
    const form = evt.currentTarget;
    if (form.dataset.pending === '1' || mutationPending) return;
    const payload = formPayload(form);
    const error = validatePayload(payload);
    if (error) {
      showFormError(form, error);
      return;
    }

    const id = current?.id || payload.id;
    const isEdit = !!current;
    if (!beginMutation()) return;
    try {
      setFormPending(form, true);
      const res = await fetch(isEdit ? `${API.gtfs}/${encodeURIComponent(id)}` : API.gtfs, {
        method: isEdit ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error(await responseError(res));
      AppUI.notify(isEdit ? 'Transit agency saved' : 'Transit agency created');
      editingID = id;
      await loadGTFS();
      if (editingID) editAgency(editingID);
    } catch (err) {
      showFormError(form, err.message || 'Save failed');
      AppUI.reportError('Transit agency save failed', err);
    } finally {
      if (document.contains(form)) setFormPending(form, false);
      endMutation();
    }
  }

  async function loadDashboards() {
    panel.innerHTML = loadingHTML('Loading dashboards...');
    try {
      const dashboards = await DashboardAPI.listProfiles();
      renderDashboards(dashboards || []);
    } catch (err) {
      AppUI.reportError('Dashboards failed to load', err);
      panel.innerHTML = emptyHTML('Dashboards failed to load.');
    }
  }

  function renderDashboards(dashboards) {
    const rows = dashboards.length
      ? dashboards.map(dashboardRow).join('')
      : '<p class="admin-empty">No dashboards saved.</p>';
    panel.innerHTML = `
      <div class="admin-page-head">
        <h2 class="admin-page-title">Dashboards</h2>
        <button type="button" class="btn-small btn-refresh">Refresh</button>
      </div>
      <div class="admin-table-list">${rows}</div>
    `;
    panel.querySelector('.btn-refresh')?.addEventListener('click', loadDashboards);
    panel.querySelectorAll('.btn-dashboard-delete').forEach(btn => {
      btn.addEventListener('click', () => deleteDashboard(btn.dataset.id, btn.dataset.name));
    });
    applyMutationState();
  }

  function dashboardRow(d) {
    return `
      <article class="admin-row">
        <div>
          <div class="admin-row-title">${esc(d.name || d.id)}</div>
          <div class="admin-row-meta">${esc(d.id)} · ${esc(d.layout || 'landscape')}${d.description ? ` · ${esc(d.description)}` : ''}</div>
        </div>
        <button type="button" class="btn-small btn-dashboard-delete" data-id="${esc(d.id)}" data-name="${esc(d.name || d.id)}">Delete</button>
      </article>
    `;
  }

  async function deleteDashboard(id, name) {
    if (!window.confirm(`Delete dashboard ${name}?`)) return;
    if (!beginMutation()) return;
    try {
      await DashboardAPI.deleteProfile(id);
      AppUI.notify('Dashboard deleted');
      await loadDashboards();
    } catch (err) {
      AppUI.reportError('Dashboard delete failed', err);
    } finally {
      endMutation();
    }
  }

  async function loadCollectors() {
    panel.innerHTML = loadingHTML('Loading collectors...');
    try {
      const res = await fetch(API.collectors);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const collectors = await res.json();
      renderCollectors(collectors || []);
    } catch (err) {
      AppUI.reportError('Collectors failed to load', err);
      panel.innerHTML = emptyHTML('Collectors failed to load.');
    }
  }

  function renderCollectors(collectors) {
    const rows = collectors.length
      ? collectors.map(collectorRow).join('')
      : '<p class="admin-empty">No collectors registered.</p>';
    panel.innerHTML = `
      <div class="admin-page-head">
        <h2 class="admin-page-title">Collectors</h2>
        <button type="button" class="btn-small btn-refresh">Refresh</button>
      </div>
      <div class="collector-list">${rows}</div>
    `;
    panel.querySelector('.btn-refresh')?.addEventListener('click', loadCollectors);
    panel.querySelectorAll('.btn-connectivity-toggle').forEach(btn => {
      btn.addEventListener('click', () => toggleConnectivityForce(btn.dataset.forced === 'true'));
    });
    applyMutationState();
  }

  function collectorRow(c) {
    const forceToggle = c.forced != null ? `
      <div class="collector-force-row">
        <button type="button" class="btn-small btn-connectivity-toggle" data-forced="${esc(String(c.forced))}">
          ${c.forced ? 'Clear forced offline' : 'Force offline'}
        </button>
        ${c.forced ? '<span class="collector-force-label">Test mode active</span>' : ''}
      </div>` : '';
    return `
      <article class="collector-row">
        <div class="collector-main">
          <div class="collector-id-line">
            <span class="collector-id">${esc(c.id)}</span>
            <span class="status-pill status-${esc(c.status || 'unknown')}">${esc(c.status || 'unknown')}</span>
          </div>
          <div class="collector-domain">${esc(c.domain || '')}</div>
          <div class="collector-times">${collectorTimes(c)}</div>
          ${c.message ? `<div class="collector-message">${esc(c.message)}</div>` : ''}
          ${forceToggle}
        </div>
        <div class="collector-metadata">${metadataHTML(c.metadata)}</div>
      </article>
    `;
  }

  async function toggleConnectivityForce(currentlyForced) {
    if (!beginMutation()) return;
    try {
      const res = await fetch('/api/v1/admin/connectivity', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ forced_down: !currentlyForced }),
      });
      if (!res.ok) throw new Error(await responseError(res));
      AppUI.notify(currentlyForced ? 'Connectivity override cleared' : 'Connectivity forced offline');
      await loadCollectors();
    } catch (err) {
      AppUI.reportError('Connectivity toggle failed', err);
    } finally {
      endMutation();
    }
  }

  function collectorTimes(c) {
    const rows = [];
    if (c.last_updated_at) rows.push(timeRow('Last data update', c.last_updated_at));
    if (c.last_event_at) rows.push(timeRow('Last health event', c.last_event_at));
    if (c.last_success_at) rows.push(timeRow('Last successful health event', c.last_success_at));
    return rows.length ? rows.join('') : '<div>No updates observed yet</div>';
  }

  function timeRow(label, raw) {
    return `<div><span class="collector-time-label">${esc(label)}</span> ${esc(formatTime(raw))}</div>`;
  }

  function metadataHTML(metadata) {
    const agencies = metadata?.agencies || [];
    if (!agencies.length) return '<span>No collector metadata.</span>';
    return `
      <table class="metadata-table">
        <thead>
          <tr>
            <th>Agency</th>
            <th>Routes</th>
            <th>Stops</th>
            <th>Trips</th>
            <th>Shapes</th>
            <th>Links</th>
          </tr>
        </thead>
        <tbody>
          ${agencies.map(ag => `
            <tr>
              <td>${esc(ag.id)}</td>
              <td>${num(ag.schedule?.routes)}</td>
              <td>${num(ag.schedule?.stops)}</td>
              <td>${num(ag.schedule?.trips)}</td>
              <td>${num(ag.schedule?.shapes)}</td>
              <td>${num(ag.schedule?.route_stops)}</td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    `;
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

  function beginMutation() {
    if (mutationPending) return false;
    mutationPending = true;
    applyMutationState();
    return true;
  }

  function endMutation() {
    mutationPending = false;
    applyMutationState();
  }

  function applyMutationState() {
    const app = document.querySelector('.admin-app');
    if (!app) return;
    app.classList.toggle('admin-busy', mutationPending);
    app.querySelectorAll('button, input, textarea, select').forEach(el => {
      if (mutationPending) {
        if (!el.disabled) el.dataset.adminWasEnabled = '1';
        el.disabled = true;
      } else if (el.dataset.adminWasEnabled === '1') {
        el.disabled = false;
        delete el.dataset.adminWasEnabled;
      }
    });
  }

  function formatTime(raw) {
    const date = new Date(raw);
    if (Number.isNaN(date.getTime())) return 'unknown';
    return date.toLocaleString([], {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }

  function num(value) {
    return Number(value || 0).toLocaleString();
  }

  function loadingHTML(message) {
    return `<p class="admin-loading">${esc(message)}</p>`;
  }

  function emptyHTML(message) {
    return `<p class="admin-empty">${esc(message)}</p>`;
  }

  function esc(s) {
    return String(s || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  document.addEventListener('DOMContentLoaded', init);

  return { selectTab };
})();
