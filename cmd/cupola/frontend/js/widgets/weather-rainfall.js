(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  // Rain intensity thresholds in mm/Hr (WMO scale).
  function rainIntensity(rate) {
    if (rate <= 0)   return { label: 'No rain',    color: 'rgba(255,255,255,0.3)', pct: 0 };
    if (rate <= 2.5) return { label: 'Light',      color: '#74b9ff', pct: Math.log1p(rate) / Math.log1p(50) * 100 };
    if (rate <= 7.6) return { label: 'Moderate',   color: '#0984e3', pct: Math.log1p(rate) / Math.log1p(50) * 100 };
    if (rate <= 50)  return { label: 'Heavy',      color: '#6c5ce7', pct: Math.log1p(rate) / Math.log1p(50) * 100 };
    return                  { label: 'Very Heavy', color: '#fd79a8', pct: 100 };
  }

  function render(container, data) {
    if (!data) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">Source unavailable</span><span style="font-size:10px;opacity:.5">weather.current</span></div>`;
      return;
    }

    const rate = data.precipitation ?? 0;
    const intensity = rainIntensity(rate);
    const ratePct = Math.min(100, intensity.pct).toFixed(0);

    // Accumulation totals — bars scaled relative to the largest non-zero value.
    const totals = [
      { label: 'Event', value: data.rain_event   ?? 0 },
      { label: '24h',   value: data.rain_daily   ?? 0 },
      { label: 'Week',  value: data.rain_weekly  ?? 0 },
      { label: 'Month', value: data.rain_monthly ?? 0 },
      { label: 'Year',  value: data.rain_yearly  ?? 0 },
    ];
    const maxVal = Math.max(...totals.map(t => t.value), 0.001);

    const rows = totals.map(t => {
      const pct = (t.value / maxVal * 100).toFixed(0);
      const valStr = t.value > 0 ? `${t.value.toFixed(1)}&thinsp;mm` : '—';
      return `
        <div class="rf-period">
          <span class="rf-period-label">${t.label}</span>
          <span class="rf-period-bar-wrap"><span class="rf-period-bar" style="width:${pct}%"></span></span>
          <span class="rf-period-amt">${valStr}</span>
        </div>`;
    }).join('');

    container.innerHTML = `
      <div class="widget-rainfall">
        <div class="rf-current">
          <div class="rf-rate">${rate.toFixed(1)}<span class="rf-rate-unit">mm/Hr</span></div>
          <div class="rf-intensity-label" style="color:${intensity.color}">${intensity.label}</div>
          <div class="rf-bar">
            <div class="rf-bar-fill" style="width:${ratePct}%;background:${intensity.color}"></div>
          </div>
        </div>
        <div class="rf-forecast">
          <span class="rf-forecast-title">Accumulation</span>
          ${rows}
        </div>
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'weather-rainfall',
    label: 'Rainfall',
    domain: 'weather.current',
    defaultSize: { w: 5, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config) { render(container, data); },
  });
})();
