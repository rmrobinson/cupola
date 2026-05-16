(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function escAttr(s) {
    return esc(s).replace(/"/g, '&quot;');
  }

  function fmtNum(v, decimals) {
    if (v == null) return null;
    return Number(v).toFixed(decimals);
  }

  function dist2(lat1, lon1, lat2, lon2) {
    const dlat = lat1 - lat2, dlon = lon1 - lon2;
    return dlat * dlat + dlon * dlon;
  }

  function severityClass(status) {
    switch (status) {
      case 'emergency': return 'advisory-emergency';
      case 'warning':   return 'advisory-warning';
      case 'watch':     return 'advisory-watch';
      case 'advisory':  return 'advisory-advisory';
      default:          return '';
    }
  }

  function selectGauges(gauges, config) {
    const ids = (config.gauge_ids || '').split(',').map(s => s.trim()).filter(Boolean);
    if (ids.length > 0) {
      return ids.map(id => gauges.find(g => g.id === id)).filter(Boolean);
    }
    // Auto mode: sort by distance from site location, take top N.
    const max = parseInt(config.max_gauges, 10) || 3;
    const cfg = window.CupolaConfig || {};
    const lat = cfg.lat || 0, lon = cfg.lon || 0;
    const sorted = gauges.slice().sort((a, b) =>
      dist2(lat, lon, a.lat, a.lon) - dist2(lat, lon, b.lat, b.lon)
    );
    return sorted.slice(0, max);
  }

  function renderGauge(g) {
    const advClass = severityClass(g.advisory_status);
    const level = fmtNum(g.level_m, 3);
    const flow  = fmtNum(g.flow_cms, 3);
    const temp  = fmtNum(g.temp_c, 1);

    let metrics = '';
    if (level != null) metrics += `<span class="ww-metric"><span class="ww-label">Level</span><span class="ww-value">${esc(level)} m</span></span>`;
    if (flow  != null) metrics += `<span class="ww-metric"><span class="ww-label">Flow</span><span class="ww-value">${esc(flow)} m³/s</span></span>`;
    if (temp  != null) metrics += `<span class="ww-metric"><span class="ww-label">Temp</span><span class="ww-value">${esc(temp)} °C</span></span>`;

    const badge = (g.advisory_status && g.advisory_status !== 'none')
      ? `<span class="ww-advisory-badge ${esc(advClass)}">${esc(g.advisory_status)}</span>`
      : '';

    return `
      <div class="ww-gauge ${advClass ? 'ww-gauge-alert' : ''} ${esc(advClass)} detail-clickable"
           data-detail-domain="waterway.conditions" data-detail-id="${escAttr(g.id)}"
           role="button" tabindex="0">
        <div class="ww-gauge-left">
          <div class="ww-gauge-title">
            <span class="ww-name">${esc(g.name)}</span>${badge}
          </div>
          <span class="ww-waterway">${esc(g.waterway_name)}</span>
        </div>
        <div class="ww-metrics">${metrics || '<span class="ww-no-data">—</span>'}</div>
      </div>`;
  }

  function render(container, state, config) {
    if (!state) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">Source unavailable</span><span style="font-size:10px;opacity:.5">waterway.conditions</span></div>`;
      return;
    }
    const gauges = selectGauges(state.gauges || [], config || {});
    if (gauges.length === 0) {
      container.innerHTML = `<div class="widget-empty">No gauge data available</div>`;
      return;
    }
    container.innerHTML = `<div class="widget-waterway">${gauges.map(renderGauge).join('')}</div>`;
  }

  window.CupolaWidgets.push({
    type: 'waterway',
    label: 'Waterway Conditions',
    domain: 'waterway.conditions',
    defaultSize: { w: 5, h: 2 },
    configSchema: [
      {
        key: 'gauge_ids',
        label: 'Gauge IDs (optional)',
        type: 'text',
        placeholder: 'e.g. grca_bridgeport, grca_doon',
      },
      {
        key: 'max_gauges',
        label: 'Max gauges (auto mode)',
        type: 'number',
        placeholder: '3',
      },
    ],
    subscriptionParams(_config) { return null; },
    render,
    onUpdate(container, data, config) { render(container, data, config); },
  });
})();
