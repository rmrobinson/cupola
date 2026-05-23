(function () {
  'use strict';
  window.CupolaWidgets = window.CupolaWidgets || [];

  // ── Helpers ───────────────────────────────────────────────────────────────

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function fmtTime(iso) {
    if (!iso) return '—';
    return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  // Stable hue from agency ID string so each agency gets a distinct badge color.
  function agencyHue(agencyID) {
    let h = 0;
    for (let i = 0; i < agencyID.length; i++) h = (h * 31 + agencyID.charCodeAt(i)) & 0xffff;
    return h % 360;
  }

  function agencyBadgeStyle(agencyID) {
    const hue = agencyHue(agencyID);
    return `background:hsl(${hue},55%,38%)`;
  }

  // ── Subscription management ───────────────────────────────────────────────

  function subKeyPrefix(widgetId) {
    return `${widgetId}:station:`;
  }

  function subKey(widgetId, agency, route, stopId) {
    return `${subKeyPrefix(widgetId)}${agency}:${route}:${stopId}`;
  }

  function arrivalStateKey(agency, route, stopId) {
    return `${agency}:${route}:${stopId}`;
  }

  function cleanupSubscriptions(container) {
    if (container._stationStreamHandler) {
      Stream.off('transit.arrivals', container._stationStreamHandler);
      container._stationStreamHandler = null;
    }
    const subKeys = container._stationSubKeys || [];
    subKeys.forEach(k => Subscriptions.remove(k));
    container._stationSubKeys = [];
    container._stationArrivals = {};
    container._stationArrivalKeys = new Set();
  }

  // Called on SSE update — pull relevant keys out of the full arrivals state.
  function mergeArrivals(container, fullState) {
    const tracked = container._stationArrivalKeys || new Set();
    const stops = fullState?.stops || {};
    tracked.forEach(k => {
      if (stops[k] !== undefined) {
        container._stationArrivals[k] = stops[k];
      }
    });
  }

  // Discover routes for each configured stop, then register subscriptions.
  // Returns a promise that resolves when all subscriptions are posted.
  async function setupSubscriptions(container, config) {
    cleanupSubscriptions(container);

    const widgetId = container.dataset.widgetId;
    if (!widgetId) return;

    const stops = (config?.stops || []).filter(s => s.agency_id && s.stop_id);
    if (stops.length === 0) return;

    container._stationSubKeys = [];
    container._stationArrivalKeys = new Set();
    container._stationArrivals = {};

    const promises = stops.map(async ({ agency_id, stop_id }) => {
      let routes = [];
      try {
        const r = await fetch(
          `/api/v1/transit/agencies/${encodeURIComponent(agency_id)}/stops/${encodeURIComponent(stop_id)}/routes`
        );
        if (r.ok) routes = await r.json();
      } catch {}

      for (const rt of routes) {
        const sk = subKey(widgetId, agency_id, rt.id, stop_id);
        const ak = arrivalStateKey(agency_id, rt.id, stop_id);
        container._stationSubKeys.push(sk);
        container._stationArrivalKeys.add(ak);
        Subscriptions.create(sk, 'transit.arrivals', {
          agency: agency_id,
          route: rt.id,
          stop_id,
          max_trips: maxDepartures(config),
        });
      }
    });

    await Promise.all(promises);

    // Seed from current full state.
    try {
      const r = await fetch('/api/v1/state/transit.arrivals');
      if (r.ok) mergeArrivals(container, await r.json());
    } catch {}

    // Register stream handler.
    container._stationStreamHandler = (data) => {
      mergeArrivals(container, data);
      renderDepartures(container, config);
    };
    Stream.on('transit.arrivals', container._stationStreamHandler);
  }

  // ── Render ────────────────────────────────────────────────────────────────

  function renderDepartures(container, config) {
    const stationName = config?.station_name || 'Station';
    const maxDep = maxDepartures(config);
    const windowMin = config?.time_window_minutes >= 0 ? Number(config.time_window_minutes) : 60;
    const now = Date.now();

    // Gather and annotate all arrivals.
    const allArrivals = [];
    const arrivalsMap = container._stationArrivals || {};
    for (const [, sa] of Object.entries(arrivalsMap)) {
      for (const a of (sa.arrivals || [])) {
        allArrivals.push({ ...a, _agency: sa.agency_id, _route: sa.route_name || sa.route_id, _stop: sa.stop_name });
      }
    }

    // Sort by effective departure time.
    allArrivals.sort((a, b) => {
      const ta = new Date(a.predicted || a.scheduled).getTime();
      const tb = new Date(b.predicted || b.scheduled).getTime();
      return ta - tb;
    });

    // Filter: remove past departures; apply time window.
    const filtered = allArrivals.filter(a => {
      const t = new Date(a.predicted || a.scheduled).getTime();
      if (t < now) return false;
      if (windowMin > 0 && t > now + windowMin * 60000) return false;
      return true;
    }).slice(0, maxDep);

    const hasConfig = (config?.stops || []).some(s => s.agency_id && s.stop_id);

    let rows = '';
    if (!hasConfig) {
      rows = `<p class="station-no-config">Configure station stops using &#9881;</p>`;
    } else if (filtered.length === 0) {
      const hasData = Object.keys(arrivalsMap).length > 0;
      rows = `<p class="station-empty">${hasData ? 'No upcoming departures' : 'Waiting for data…'}</p>`;
    } else {
      rows = `<div class="station-list">${filtered.map(departureRow).join('')}</div>`;
    }

    container.innerHTML = `
      <div class="widget-transit-station">
        <div class="station-header">
          <span class="station-name">${esc(stationName)}</span>
        </div>
        ${rows}
      </div>`;
  }

  function maxDepartures(config) {
    const n = Number(config?.max_departures);
    if (!Number.isFinite(n) || n <= 0) return 8;
    return Math.min(20, Math.floor(n));
  }

  function departureRow(a) {
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
      if (delaySecs <= 0)       minsCls = ' rt-ontime';
      else if (delaySecs < 60)  minsCls = ' rt-slight-delay';
      else                      minsCls = ' rt-late';
      timeCls = ' rt';
    }

    let delayHtml = '';
    if (a.delay != null && a.delay !== 0) {
      const sign = a.delay > 0 ? '+' : '';
      const dm   = Math.round(a.delay / 60);
      const cls  = a.delay > 60 ? ' delay-late' : (a.delay < -30 ? ' delay-early' : '');
      delayHtml  = `<span class="departure-delay${cls}">${sign}${dm}m</span>`;
    }

    return `
      <div class="station-row${delayHtml ? ' has-delay' : ''}">
        <span class="departure-agency" style="${agencyBadgeStyle(a._agency)}">${esc(a._agency)}</span>
        <span class="departure-route">${esc(a._route)}</span>
        <span class="departure-headsign">${esc(a.headsign || '—')}</span>
        <span class="departure-mins${minsCls}">${esc(when)}</span>
        <span class="departure-time${timeCls}">${fmtTime(t)}</span>
        ${delayHtml}
      </div>`;
  }

  // ── Config panel ──────────────────────────────────────────────────────────

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

    const cfg       = wc.config || {};
    const stopRows  = (cfg.stops || []).filter(s => s.agency_id);
    const maxDep    = cfg.max_departures != null ? cfg.max_departures : 8;
    const windowMin = cfg.time_window_minutes != null ? cfg.time_window_minutes : 60;

    panel.innerHTML = `
      <form class="config-form">
        <label class="config-row">
          <span>Station name</span>
          <input type="text" name="station_name" value="${esc(cfg.station_name || '')}" placeholder="e.g. Union Station">
        </label>
        <div class="config-row" style="flex-direction:column;align-items:stretch;gap:4px">
          <span>Stops</span>
          <div class="station-stop-list"></div>
          <button type="button" class="btn-add-stop">+ Add stop</button>
        </div>
        <label class="config-row">
          <span>Max departures</span>
          <input type="number" name="max_departures" min="1" max="30" value="${esc(maxDep)}">
        </label>
        <label class="config-row">
          <span>Time window (min)</span>
          <input type="number" name="time_window_minutes" min="0" max="1440" value="${esc(windowMin)}">
        </label>
        <div class="config-actions">
          <button type="submit" class="btn-small btn-primary">Save</button>
          <button type="button" class="btn-small btn-secondary btn-config-cancel">Cancel</button>
        </div>
      </form>`;

    const form      = panel.querySelector('.config-form');
    const stopList  = form.querySelector('.station-stop-list');
    const addBtn    = form.querySelector('.btn-add-stop');

    panel.querySelector('.btn-config-cancel').addEventListener('click', () => {
      panel.classList.add('hidden');
    });

    // Cache loaded stops per agency to avoid re-fetching on re-render.
    const stopsCache = {};

    async function fetchStops(agencyID) {
      if (stopsCache[agencyID]) return stopsCache[agencyID];
      try {
        const r = await fetch(`/api/v1/transit/agencies/${encodeURIComponent(agencyID)}/stops`);
        if (r.ok) {
          stopsCache[agencyID] = await r.json();
          return stopsCache[agencyID];
        }
        if (r.status === 503) return 'loading';
      } catch {}
      return [];
    }

    function agOpts(selected) {
      return agencies.map(a =>
        `<option value="${esc(a.id)}"${a.id === selected ? ' selected' : ''}>${esc(a.id)}</option>`
      ).join('');
    }

    async function buildStopRow(agencyID, stopID) {
      const row = document.createElement('div');
      row.className = 'station-stop-row';
      row.innerHTML = `
        <select class="stop-agency-sel">${agOpts(agencyID)}</select>
        <select class="stop-stop-sel"><option value="">Loading…</option></select>
        <button type="button" class="btn-remove-stop" title="Remove">&times;</button>`;

      row.querySelector('.btn-remove-stop').addEventListener('click', () => row.remove());

      const agSel   = row.querySelector('.stop-agency-sel');
      const stopSel = row.querySelector('.stop-stop-sel');

      async function loadStops(selAgency, selStop) {
        stopSel.innerHTML = `<option value="">Loading…</option>`;
        const stops = await fetchStops(selAgency);
        if (stops === 'loading') {
          stopSel.innerHTML = `<option value="">Data still loading</option>`;
          return;
        }
        if (!stops || stops.length === 0) {
          stopSel.innerHTML = `<option value="">No stops found</option>`;
          return;
        }
        stopSel.innerHTML = stops.map(st => {
          const dist = st.distance_km < 1
            ? `${Math.round(st.distance_km * 1000)} m`
            : `${st.distance_km.toFixed(1)} km`;
          const label = st.code ? `${st.name} (${st.code}) — ${dist}` : `${st.name} — ${dist}`;
          return `<option value="${esc(st.id)}"${st.id === selStop ? ' selected' : ''}>${esc(label)}</option>`;
        }).join('');
      }

      agSel.addEventListener('change', () => loadStops(agSel.value, ''));
      await loadStops(agencyID || agencies[0]?.id || '', stopID || '');

      return row;
    }

    // Populate existing stop rows.
    for (const s of stopRows) {
      stopList.appendChild(await buildStopRow(s.agency_id, s.stop_id));
    }
    // At least one row if none configured.
    if (stopRows.length === 0) {
      stopList.appendChild(await buildStopRow(agencies[0]?.id || '', ''));
    }

    addBtn.addEventListener('click', async () => {
      addBtn.disabled = true;
      const row = await buildStopRow(agencies[0]?.id || '', '');
      stopList.appendChild(row);
      addBtn.disabled = false;
    });

    form.addEventListener('submit', e => {
      e.preventDefault();
      const data = new FormData(e.target);
      const stops = [];
      stopList.querySelectorAll('.station-stop-row').forEach(row => {
        const ag   = row.querySelector('.stop-agency-sel')?.value || '';
        const stop = row.querySelector('.stop-stop-sel')?.value   || '';
        if (ag && stop) stops.push({ agency_id: ag, stop_id: stop });
      });
      const twRaw  = Number(data.get('time_window_minutes'));
      const maxRaw = Number(data.get('max_departures'));
      wc.config = {
        station_name:        (data.get('station_name') || '').trim(),
        stops,
        max_departures:      maxRaw > 0 ? maxRaw : 8,
        time_window_minutes: isNaN(twRaw) ? 60 : Math.max(0, twRaw),
      };
      onSave();
    });
  }

  // ── Widget definition ─────────────────────────────────────────────────────

  window.CupolaWidgets.push({
    type:        'transit-station',
    domain:      'transit.arrivals',
    defaultSize: { w: 6, h: 5 },
    buildConfig,

    subscriptionParams() { return null; },

    async render(container, _state, config) {
      renderDepartures(container, config);
      await setupSubscriptions(container, config);
      renderDepartures(container, config);
    },

    onUpdate(container, data, config) {
      // If render() was never called (domain was initially unavailable), set up subscriptions now.
      if (!container._stationSubKeys) {
        this.render(container, data, config);
        return;
      }
      mergeArrivals(container, data);
      renderDepartures(container, config);
    },

    onRemove(container) {
      cleanupSubscriptions(container);
    },
  });
})();
