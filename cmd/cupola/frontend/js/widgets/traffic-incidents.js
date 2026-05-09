(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function escAttr(s) {
    return esc(s).replace(/"/g, '&quot;');
  }

  function fmtTime(iso) {
    if (!iso) return null;
    return new Date(iso).toLocaleString(undefined, {
      month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
    });
  }

  const TYPE_LABELS = {
    collision:    'Collision',
    construction: 'Construction',
    closure:      'Closure',
    hazard:       'Hazard',
  };

  const SEVERITIES = ['major', 'moderate', 'minor'];

  // Fallback sort order used only when CupolaConfig has no home coordinates.
  const SEV_ORDER = { major: 0, moderate: 1, minor: 2 };

  function haversineKm(lat1, lon1, lat2, lon2) {
    const R = 6371;
    const dLat = (lat2 - lat1) * Math.PI / 180;
    const dLon = (lon2 - lon1) * Math.PI / 180;
    const a = Math.sin(dLat / 2) ** 2 +
              Math.cos(lat1 * Math.PI / 180) * Math.cos(lat2 * Math.PI / 180) *
              Math.sin(dLon / 2) ** 2;
    return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  }

  function hasLocation(i) {
    return Number.isFinite(i.lat) && Number.isFinite(i.lon) && (i.lat !== 0 || i.lon !== 0);
  }

  function render(container, state, config) {
    const maxN = config?.max_items > 0 ? Number(config.max_items) : 10;
    const sevFilter = Array.isArray(config?.severities) && config.severities.length > 0
      ? config.severities : null;
    const radiusKm = config?.radius_km > 0 ? Number(config.radius_km) : null;
    const homeLat = window.CupolaConfig?.lat;
    const homeLon = window.CupolaConfig?.lon;
    const incidents = (state?.incidents || [])
      .filter(i => !sevFilter || sevFilter.includes(i.severity))
      .filter(i => !radiusKm || !homeLat || !homeLon || !hasLocation(i) ||
                   haversineKm(homeLat, homeLon, i.lat, i.lon) <= radiusKm)
      .slice()
      .sort((a, b) => {
        const al = hasLocation(a);
        const bl = hasLocation(b);
        if (homeLat && homeLon && al && bl) {
          return haversineKm(homeLat, homeLon, a.lat, a.lon) -
                 haversineKm(homeLat, homeLon, b.lat, b.lon);
        }
        if (al !== bl) {
          return (SEV_ORDER[a.severity] ?? 3) - (SEV_ORDER[b.severity] ?? 3);
        }
        return (SEV_ORDER[a.severity] ?? 3) - (SEV_ORDER[b.severity] ?? 3);
      })
      .slice(0, maxN);

    if (incidents.length === 0) {
      container.innerHTML = `
        <div class="widget-traffic-incidents">
          <div class="traffic-header">
            <span class="traffic-title">Traffic Incidents</span>
          </div>
          <p class="traffic-empty">${state ? 'No active incidents' : 'Waiting for data…'}</p>
        </div>`;
      return;
    }

    container.innerHTML = `
      <div class="widget-traffic-incidents">
        <div class="traffic-header">
          <span class="traffic-title">Traffic Incidents</span>
          <span class="traffic-count">${incidents.length}</span>
        </div>
        <div class="traffic-list">
          ${incidents.map(incidentRow).join('')}
        </div>
      </div>`;
  }

  function incidentRow(inc) {
    const typeLabel = TYPE_LABELS[inc.type] || esc(inc.type);
    const starts = fmtTime(inc.starts_at);
    const ends   = fmtTime(inc.ends_at);
    const timeStr = starts
      ? (ends ? `${starts} – ${ends}` : `Since ${starts}`)
      : '';
    const locationNote = inc.approximate_location
      ? `${inc.location_label || 'Approximate location'} · not shown on map`
      : '';

    return `
      <div class="incident-row sev-${esc(inc.severity)} detail-clickable"
           data-detail-domain="traffic.incidents" data-detail-id="${escAttr(inc.id)}"
           role="button" tabindex="0">
        <div class="incident-top">
          <span class="incident-type">${typeLabel}</span>
          <span class="incident-road">${esc(inc.road_name)}</span>
          <span class="incident-sev incident-sev-${esc(inc.severity)}">${esc(inc.severity)}</span>
        </div>
        <div class="incident-desc">${esc(inc.description)}</div>
        ${locationNote ? `<div class="incident-location-note">${esc(locationNote)}</div>` : ''}
        ${timeStr ? `<div class="incident-time">${timeStr}</div>` : ''}
      </div>`;
  }

  function buildConfig(panel, wc, onSave) {
    const cfg      = wc.config || {};
    const maxN     = cfg.max_items != null ? cfg.max_items : 10;
    const selSevs  = Array.isArray(cfg.severities) ? cfg.severities : [];
    const radiusKm = cfg.radius_km != null ? cfg.radius_km : '';

    panel.innerHTML = `
      <form class="config-form config-form-wide">
        <label class="config-row">
          <span>Radius (km)</span>
          <input type="number" name="radius_km" min="1" max="500" value="${esc(radiusKm)}" placeholder="all">
        </label>
        <label class="config-row config-row-multiselect">
          <span>Severities</span>
          <select name="severities" multiple size="${SEVERITIES.length}">
            ${SEVERITIES.map(s => `<option value="${esc(s)}"${selSevs.includes(s) ? ' selected' : ''}>${esc(s.charAt(0).toUpperCase() + s.slice(1))}</option>`).join('')}
          </select>
        </label>
        <label class="config-row">
          <span>Max incidents</span>
          <input type="number" name="max_items" min="1" max="100" value="${esc(maxN)}">
        </label>
        <div class="config-actions">
          <button type="submit" class="btn-small btn-primary">Save</button>
          <button type="button" class="btn-small btn-secondary btn-config-cancel">Cancel</button>
        </div>
      </form>`;

    panel.querySelector('.btn-config-cancel').addEventListener('click', () => {
      panel.classList.add('hidden');
    });

    panel.querySelector('.config-form').addEventListener('submit', e => {
      e.preventDefault();
      const data = new FormData(e.target);
      const rawRadius = data.get('radius_km');
      wc.config = {
        radius_km:  rawRadius !== '' ? Number(rawRadius) : null,
        severities: data.getAll('severities'),
        max_items:  Number(data.get('max_items')) || 10,
      };
      onSave();
    });
  }

  window.CupolaWidgets.push({
    type:        'traffic-incidents',
    domain:      'traffic.incidents',
    defaultSize: { w: 3, h: 5 },
    buildConfig,
    subscriptionParams: () => ({ province: 'ON' }),
    render(container, state, config)      { render(container, state, config); },
    onUpdate(container, data, config)     { render(container, data, config); },
  });
})();
