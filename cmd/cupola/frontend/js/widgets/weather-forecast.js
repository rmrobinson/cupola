(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  function render(container, data, config) {
    const periods = data?.periods || [];
    const limit = config?.days_to_show ? config.days_to_show * 2 : 14;
    const shown = periods.slice(0, limit);

    if (!shown.length) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No forecast data</span></div>`;
      return;
    }

    container.innerHTML = `
      <div class="widget-weather-forecast">
        ${shown.map(p => `
          <div class="forecast-period">
            <span class="fp-label">${esc(p.label)}</span>
            <span class="fp-condition">${esc(p.condition || p.summary || '')}</span>
            <span class="fp-temp">
              ${p.high != null ? `<span class="fp-high">H:${Math.round(p.high)}&deg;</span>` : ''}
              ${p.low  != null ? `<span class="fp-low">L:${Math.round(p.low)}&deg;</span>` : ''}
            </span>
            <span class="fp-pop">${p.precip_chance}%</span>
          </div>
        `).join('')}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'weather-forecast',
    domain: 'weather.forecast',
    defaultSize: { w: 7, h: 8 },
    subscriptionParams: () => null,
    render(container, state, config) { render(container, state, config); },
    onUpdate(container, data, config)  { render(container, data, config); },
  });
})();
