(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
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

  const SEV_ORDER = { major: 0, moderate: 1, minor: 2 };

  function dist2(lat1, lon1, lat2, lon2) {
    const dlat = lat1 - lat2, dlon = lon1 - lon2;
    return dlat * dlat + dlon * dlon;
  }

  function render(container, state, config) {
    const maxN = config?.max_items > 0 ? Number(config.max_items) : 10;
    const homeLat = window.CupolaConfig?.lat;
    const homeLon = window.CupolaConfig?.lon;
    const incidents = (state?.incidents || [])
      .slice()
      .sort((a, b) => {
        if (homeLat && homeLon) {
          return dist2(a.lat, a.lon, homeLat, homeLon) - dist2(b.lat, b.lon, homeLat, homeLon);
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

    return `
      <div class="incident-row sev-${esc(inc.severity)}">
        <div class="incident-top">
          <span class="incident-type">${typeLabel}</span>
          <span class="incident-road">${esc(inc.road_name)}</span>
          <span class="incident-sev incident-sev-${esc(inc.severity)}">${esc(inc.severity)}</span>
        </div>
        <div class="incident-desc">${esc(inc.description)}</div>
        ${timeStr ? `<div class="incident-time">${timeStr}</div>` : ''}
      </div>`;
  }

  window.CupolaWidgets.push({
    type:        'traffic-incidents',
    domain:      'traffic.incidents',
    defaultSize: { w: 3, h: 5 },
    configSchema: [
      { key: 'max_items', label: 'Max incidents', type: 'number', default: 10 },
    ],
    subscriptionParams: () => ({ province: 'ON' }),
    render(container, state, config)      { render(container, state, config); },
    onUpdate(container, data, config)     { render(container, data, config); },
  });
})();
