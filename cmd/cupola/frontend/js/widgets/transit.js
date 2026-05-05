(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function stopKey(config) {
    const agency  = (config?.agency  || '').trim();
    const route   = (config?.route   || '').trim();
    const stop_id = (config?.stop_id || '').trim();
    return agency && route && stop_id ? `${agency}:${route}:${stop_id}` : null;
  }

  function render(container, state, config) {
    const key  = stopKey(config);
    const maxN = config?.max_trips > 0 ? Number(config.max_trips) : 4;

    if (!key) {
      container.innerHTML = `
        <div class="widget-transit">
          <p class="transit-no-config">Configure agency, route, and stop ID using &#9881;</p>
        </div>`;
      return;
    }

    const sa       = state?.stops?.[key];
    const arrivals = (sa?.arrivals || []).slice(0, maxN);

    container.innerHTML = `
      <div class="widget-transit">
        <div class="transit-header">
          <span class="transit-route">${esc(sa?.route_name || config.route)}</span>
          <span class="transit-stop">${esc(sa?.stop_name  || config.stop_id)}</span>
        </div>
        ${arrivals.length === 0
          ? `<p class="transit-empty">${sa ? 'No upcoming arrivals' : 'Waiting for data…'}</p>`
          : `<div class="transit-list">${arrivals.map(arrivalRow).join('')}</div>`
        }
      </div>`;
  }

  function arrivalRow(a) {
    const t    = a.predicted || a.scheduled;
    const mins = Math.floor((new Date(t) - Date.now()) / 60000);
    const when = mins <= 0 ? 'Now' : mins === 1 ? '1 min' : `${mins} min`;

    const isRealtime = a.predicted != null;
    let minsCls = '';
    let timeCls = '';
    if (isRealtime) {
      const delaySecs = a.delay != null
        ? a.delay
        : (new Date(a.predicted) - new Date(a.scheduled)) / 1000;
      if (delaySecs <= 0)  minsCls = ' rt-ontime';
      else if (delaySecs < 60) minsCls = ' rt-slight-delay';
      else                 minsCls = ' rt-late';
      timeCls = ' rt';
    }

    let delayHtml = '';
    if (a.delay != null && a.delay !== 0) {
      const sign = a.delay > 0 ? '+' : '';
      const dm   = Math.round(a.delay / 60);
      const cls  = a.delay > 60 ? ' delay-late' : (a.delay < -30 ? ' delay-early' : '');
      delayHtml  = `<span class="arrival-delay${cls}">${sign}${dm}m</span>`;
    }

    return `
      <div class="arrival-row">
        <span class="arrival-mins${minsCls}">${esc(when)}</span>
        <span class="arrival-headsign">${esc(a.headsign || '—')}</span>
        <span class="arrival-time${timeCls}">${fmtTime(t)}</span>
        ${delayHtml}
      </div>`;
  }

  function fmtTime(iso) {
    if (!iso) return '—';
    return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // ── Custom config panel ───────────────────────────────────────────────────

  async function buildConfig(panel, wc, onSave) {
    panel.innerHTML = '<p class="config-loading">Loading agencies…</p>';

    let agencies = [];
    try {
      const r = await fetch('/api/v1/transit/agencies');
      if (r.ok) agencies = await r.json();
    } catch {}

    if (agencies.length === 0) {
      panel.innerHTML = '<p class="config-empty">No transit agencies configured.</p>';
      return;
    }

    const cfg     = wc.config || {};
    const selAg   = cfg.agency  || '';
    const selRt   = cfg.route   || '';
    const selStop = cfg.stop_id || '';
    const maxT    = cfg.max_trips != null ? cfg.max_trips : 4;

    // Agency options
    const agOpts = agencies.map(a =>
      `<option value="${esc(a.id)}"${a.id === selAg ? ' selected' : ''}>${esc(a.id)}</option>`
    ).join('');

    panel.innerHTML = `
      <form class="config-form">
        <label class="config-row">
          <span>Agency</span>
          <select name="agency">${agOpts}</select>
        </label>
        <label class="config-row">
          <span>Route</span>
          <select name="route"><option value="">— select agency first —</option></select>
        </label>
        <label class="config-row">
          <span>Stop</span>
          <select name="stop_id"><option value="">— select route first —</option></select>
        </label>
        <label class="config-row">
          <span>Max trips</span>
          <input type="number" name="max_trips" min="1" max="20" value="${esc(maxT)}">
        </label>
        <div class="config-actions">
          <button type="submit" class="btn-small btn-primary">Save</button>
          <button type="button" class="btn-small btn-secondary btn-config-cancel">Cancel</button>
        </div>
      </form>`;

    const form     = panel.querySelector('.config-form');
    const agSel    = form.querySelector('[name="agency"]');
    const rtSel    = form.querySelector('[name="route"]');
    const stopSel  = form.querySelector('[name="stop_id"]');

    panel.querySelector('.btn-config-cancel').addEventListener('click', () => {
      panel.classList.add('hidden');
    });

    async function loadRoutes(agencyID, currentRoute) {
      rtSel.innerHTML = '<option value="">Loading…</option>';
      stopSel.innerHTML = '<option value="">— select route first —</option>';
      if (!agencyID) return;
      let routes = [];
      try {
        const r = await fetch(`/api/v1/transit/agencies/${encodeURIComponent(agencyID)}/routes`);
        if (r.ok) routes = await r.json();
      } catch {}
      if (routes.length === 0) {
        rtSel.innerHTML = '<option value="">No routes found</option>';
        return;
      }
      rtSel.innerHTML = routes.map(rt => {
        const label = rt.short_name ? `${rt.short_name} — ${rt.long_name}` : rt.long_name;
        return `<option value="${esc(rt.id)}"${rt.id === currentRoute ? ' selected' : ''}>${esc(label)}</option>`;
      }).join('');
      if (currentRoute) await loadStops(agencyID, currentRoute, selStop);
    }

    async function loadStops(agencyID, routeID, currentStop) {
      stopSel.innerHTML = '<option value="">Loading…</option>';
      if (!agencyID || !routeID) return;
      let stops = [];
      try {
        const r = await fetch(`/api/v1/transit/agencies/${encodeURIComponent(agencyID)}/routes/${encodeURIComponent(routeID)}/stops`);
        if (r.ok) stops = await r.json();
      } catch {}
      if (stops.length === 0) {
        stopSel.innerHTML = '<option value="">No stops found</option>';
        return;
      }
      stopSel.innerHTML = stops.map(st => {
        const dist = st.distance_km < 1
          ? `${Math.round(st.distance_km * 1000)} m`
          : `${st.distance_km.toFixed(1)} km`;
        const label = st.code ? `${st.name} (${st.code}) — ${dist}` : `${st.name} — ${dist}`;
        return `<option value="${esc(st.id)}"${st.id === currentStop ? ' selected' : ''}>${esc(label)}</option>`;
      }).join('');
    }

    agSel.addEventListener('change', () => loadRoutes(agSel.value, ''));
    rtSel.addEventListener('change', () => loadStops(agSel.value, rtSel.value, ''));

    // Always load routes for the currently-shown agency (handles both saved config
    // and new widgets where the select defaults to the first agency).
    await loadRoutes(agSel.value, selRt);

    form.addEventListener('submit', e => {
      e.preventDefault();
      const data = new FormData(e.target);
      wc.config = {
        agency:    data.get('agency')    || '',
        route:     data.get('route')     || '',
        stop_id:   data.get('stop_id')   || '',
        max_trips: Number(data.get('max_trips')) || 4,
      };
      onSave();
    });
  }

  window.CupolaWidgets.push({
    type:        'transit',
    domain:      'transit.arrivals',
    defaultSize: { w: 3, h: 4 },
    buildConfig,
    subscriptionParams(config) {
      const agency  = (config?.agency  || '').trim();
      const route   = (config?.route   || '').trim();
      const stop_id = (config?.stop_id || '').trim();
      if (!agency || !route || !stop_id) return null;
      return { agency, route, stop_id };
    },
    render(container, state, config)       { render(container, state, config); },
    onUpdate(container, data,  config)     { render(container, data,  config); },
  });
})();
