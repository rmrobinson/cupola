(function () {
  'use strict';
  window.CupolaWidgets = window.CupolaWidgets || [];

  function stopKey(config) {
    const agency  = (config?.agency  || '').trim();
    const route   = (config?.route   || '').trim();
    const stop_id = (config?.stop_id || '').trim();
    return agency && route && stop_id ? `${agency}:${route}:${stop_id}` : null;
  }

  // ── Route overlay helpers ─────────────────────────────────────────────────

  const SHAPE_RETRY_DELAY_MS = 60_000;

  function routeKey(config) {
    const agency = (config?.agency || '').trim();
    const route = (config?.route || '').trim();
    return agency && route ? `${agency}:${route}` : null;
  }

  function shapeStatus(container, key) {
    container._shapeStatusByRouteKey = container._shapeStatusByRouteKey || {};
    container._shapeStatusByRouteKey[key] = container._shapeStatusByRouteKey[key] || {
      data: null,
      unavailable: false,
      failedAt: null,
      notReadyAt: null,
    };
    return container._shapeStatusByRouteKey[key];
  }

  async function fetchShape(container, config) {
    const key = routeKey(config);
    if (!key) return false;

    const status = shapeStatus(container, key);
    if (status.unavailable) return false;
    if (status.data) return true;

    // Backoff: don't hammer the server after a transient failure.
    const lastFailure = status.failedAt || status.notReadyAt;
    if (lastFailure && Date.now() - lastFailure < SHAPE_RETRY_DELAY_MS) {
      return false;
    }

    try {
      const url = `/api/v1/transit/agencies/${encodeURIComponent(config.agency)}/routes/${encodeURIComponent(config.route)}/shape`;
      const r = await fetch(url);
      if (r.status === 404) {
        status.unavailable = true;
        status.failedAt = null;
        status.notReadyAt = null;
        return false;
      }
      if (r.status === 503) {
        status.notReadyAt = Date.now();
        return false;
      }
      if (!r.ok) {
        status.failedAt = Date.now();
        return false;
      }
      status.data = await r.json();
      status.failedAt = null;
      status.notReadyAt = null;
      return true;
    } catch {
      status.failedAt = Date.now();
      return false;
    }
  }

  async function syncOverlay(container, config) {
    const widgetId = container.dataset.widgetId;
    if (!widgetId) return;
    const ovl = window.CupolaOverlays;
    if (!ovl) return;

    if (!config.show_route || !ovl.hasMap()) {
      ovl.unregister(widgetId);
      return;
    }

    const ok = await fetchShape(container, config);
    if (!ok) {
      ovl.unregister(widgetId);
      return;
    }

    const shape = shapeStatus(container, routeKey(config)).data;
    ovl.register(widgetId, {
      type:        'polyline',
      color:       shape.color || '',
      coordinates: shape.geometry.coordinates,
      agencyId:    config.agency,
      routeId:     config.route,
    });
  }

  function setupAvailCb(container, config) {
    const ovl = window.CupolaOverlays;
    if (!ovl) return;

    if (container._mapAvailCb) ovl.offMapAvail(container._mapAvailCb);

    // Snapshot map availability before registering so the immediate-fire from
    // onMapAvail can distinguish "no map yet on page load" from "map was removed".
    let prevHasMap = ovl.hasMap();

    const cb = (hasMap) => {
      const wasAvail = prevHasMap;
      prevHasMap = hasMap;

      if (!hasMap && wasAvail && config.show_route) {
        ovl.unregister(container.dataset.widgetId || '');
      } else if (hasMap && config.show_route) {
        syncOverlay(container, config);
      }
    };

    container._mapAvailCb = cb;
    ovl.onMapAvail(cb);
  }

  // ── Render ────────────────────────────────────────────────────────────────

  function render(container, state, config) {
    const key  = stopKey(config);
    const maxN = maxTrips(config);

    if (!key) {
      container.innerHTML = `
        <div class="widget-transit">
          <p class="transit-no-config">Configure agency, route, and stop ID using &#9881;</p>
        </div>`;
      setupAvailCb(container, config);
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

    setupAvailCb(container, config);
    syncOverlay(container, config);
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
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function maxTrips(config) {
    const n = Number(config?.max_trips);
    if (!Number.isFinite(n) || n <= 0) return 4;
    return Math.min(20, Math.floor(n));
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

    // Determine whether "show route on map" can be enabled.
    const content      = panel.closest('.widget-inner')?.querySelector('.widget-content');
    const hasMap       = window.CupolaOverlays?.hasMap() || false;
    const selectedRouteKey = routeKey({ agency: selAg, route: selRt });
    const shapeStatusForRoute = selectedRouteKey && content?._shapeStatusByRouteKey?.[selectedRouteKey];
    const noShapes = !!shapeStatusForRoute?.unavailable;
    const canShowRoute = hasMap;
    const showRouteHint = !hasMap
      ? 'Add a map widget to enable this option'
      : noShapes
        ? 'This route has no shape data and will not be drawn'
        : '';

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
        <label class="config-row" title="${esc(showRouteHint)}">
          <span>Show route on map</span>
          <input type="checkbox" name="show_route"${cfg.show_route ? ' checked' : ''}${!canShowRoute ? ' disabled' : ''}>
        </label>
        <div class="config-actions">
          <button type="submit" class="btn-small btn-primary">Save</button>
          <button type="button" class="btn-small btn-secondary btn-config-cancel">Cancel</button>
        </div>
      </form>`;

    const form    = panel.querySelector('.config-form');
    const agSel   = form.querySelector('[name="agency"]');
    const rtSel   = form.querySelector('[name="route"]');
    const stopSel = form.querySelector('[name="stop_id"]');

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
        if (r.status === 503) {
          rtSel.innerHTML = '<option value="">Route data is still loading</option>';
          return;
        }
        if (r.ok) {
          routes = await r.json();
        } else {
          rtSel.innerHTML = '<option value="">Unable to load routes</option>';
          return;
        }
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
        if (r.status === 503) {
          stopSel.innerHTML = '<option value="">Stop data is still loading</option>';
          return;
        }
        if (r.ok) {
          stops = await r.json();
        } else {
          stopSel.innerHTML = '<option value="">Unable to load stops</option>';
          return;
        }
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

    await loadRoutes(agSel.value, selRt);

    form.addEventListener('submit', e => {
      e.preventDefault();
      const data      = new FormData(e.target);
      const newRoute  = data.get('route') || '';
      wc.config = {
        agency:     data.get('agency')    || '',
        route:      newRoute,
        stop_id:    data.get('stop_id')   || '',
        max_trips:  Number(data.get('max_trips')) || 4,
        show_route: data.get('show_route') === 'on',
      };
      onSave();
    });
  }

  // ── Widget definition ─────────────────────────────────────────────────────

  window.CupolaWidgets.push({
    type:        'transit',
    domain:      'transit.arrivals',
    defaultSize: { w: 6, h: 4 },
    buildConfig,
    subscriptionParams(config) {
      const agency  = (config?.agency  || '').trim();
      const route   = (config?.route   || '').trim();
      const stop_id = (config?.stop_id || '').trim();
      if (!agency || !route || !stop_id) return null;
      return { agency, route, stop_id, max_trips: maxTrips(config) };
    },
    render(container, state, config)   { render(container, state, config); },
    onUpdate(container, data, config)  { render(container, data,  config); },
    onRemove(container, _config) {
      const widgetId = container.dataset.widgetId;
      const ovl = window.CupolaOverlays;
      if (ovl) {
        if (widgetId) ovl.unregister(widgetId);
        if (container._mapAvailCb) ovl.offMapAvail(container._mapAvailCb);
      }
    },
  });
})();
