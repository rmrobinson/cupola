(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function aqhiColor(value, risk) {
    const label = String(risk || '').toLowerCase();
    if (label.includes('very high') || value > 10) return '#c0392b';
    if (label.includes('high') || value >= 7) return '#e53935';
    if (label.includes('moderate') || value >= 4) return '#f7b733';
    if (label.includes('low') || value >= 1) return '#57d9a3';
    return 'rgba(255,255,255,0.45)';
  }

  function fmtValue(v) {
    if (!v) return '—';
    if (v.value == null) return esc(v.risk || '—');
    return String(v.value);
  }

  function fmtTime(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  function render(container, data) {
    if (!data) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No air quality data</span><span style="font-size:10px;opacity:.5">weather.air_quality</span></div>`;
      return;
    }

    const observed = data.observed || null;
    const value = observed && observed.value != null ? observed.value : null;
    const risk = observed ? observed.risk : '';
    const color = aqhiColor(value, risk);
    const forecasts = Array.isArray(data.forecasts) ? data.forecasts.slice(0, 4) : [];
    const issued = fmtTime(data.issued_at);

    container.innerHTML = `
      <div class="widget-air-quality">
        <div class="aq-header">
          <div class="aq-location">${esc(data.location || 'Air quality')}</div>
          <div class="aq-province">${esc(data.province || '')}</div>
        </div>
        <div class="aq-current">
          <div class="aq-value" style="color:${color}">${fmtValue(observed)}</div>
          <div class="aq-risk" style="color:${color}">${esc(risk || 'AQHI')}</div>
        </div>
        <div class="aq-bar">
          <div class="aq-bar-fill" style="width:${Math.min(100, Math.max(0, ((value || 0) / 10) * 100)).toFixed(0)}%;background:${color}"></div>
        </div>
        <div class="aq-forecast">
          ${forecasts.map(p => {
            const max = p.max || null;
            const maxColor = aqhiColor(max && max.value, max && max.risk);
            return `
              <div class="aq-period">
                <span class="aq-period-label">${esc(p.label)}</span>
                <span class="aq-period-value" style="color:${maxColor}">${fmtValue(max)}</span>
                <span class="aq-period-risk">${esc(max && max.risk || '')}</span>
              </div>
            `;
          }).join('')}
        </div>
        ${issued ? `<div class="aq-issued">Issued ${issued}</div>` : ''}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'weather-air-quality',
    domain: 'weather.air_quality',
    defaultSize: { w: 4, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config)  { render(container, data); },
  });
})();
