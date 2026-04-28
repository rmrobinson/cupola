(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function kpColor(kp) {
    if (kp < 2)  return '#74b9ff';
    if (kp < 4)  return '#a8ff78';
    if (kp < 5)  return '#f7b733';
    if (kp < 7)  return '#fc4a1a';
    return '#c0392b';
  }

  function fmtTime(iso) {
    if (!iso) return '—';
    return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  function render(container, data) {
    if (!data) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No forecast data</span></div>`;
      return;
    }

    const periods = data.periods || [];
    if (periods.length === 0) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No forecast periods</span></div>`;
      return;
    }

    const rows = periods.map(p => {
      const kp    = p.kp_expected ?? 0;
      const color = kpColor(kp);
      return `
        <div class="solar-forecast-row">
          <span class="sfr-time">${fmtTime(p.starts_at)}</span>
          <span class="sfr-kp" style="color:${color}">${kp.toFixed(1)}</span>
          <span class="sfr-desc">${p.kp_description || ''}</span>
          <span class="sfr-aurora${p.aurora_viewable ? '' : ' sfr-aurora-dim'}" title="${p.aurora_viewable ? 'Aurora possible' : 'Aurora unlikely'}">&#9728;</span>
        </div>
      `;
    }).join('');

    container.innerHTML = `<div class="widget-solar-forecast">${rows}</div>`;
  }

  window.CupolaWidgets.push({
    type: 'solar-forecast',
    domain: 'solar.weather.forecast',
    defaultSize: { w: 2, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config)  { render(container, data); },
  });
})();
