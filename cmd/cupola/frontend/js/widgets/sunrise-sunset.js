(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function fmt(iso, tz) {
    if (!iso) return '—';
    return new Date(iso).toLocaleTimeString(undefined, {
      hour: '2-digit', minute: '2-digit', timeZone: tz || undefined,
    });
  }

  function render(container, data, config) {
    if (!data) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No astro data</span></div>`;
      return;
    }
    const tz = config?.timezone || undefined;
    const rows = [
      ['Civil dawn',  data.civil_dawn],
      ['Sunrise',     data.sunrise],
      ['Solar noon',  data.solar_noon],
      ['Sunset',      data.sunset],
      ['Civil dusk',  data.civil_dusk],
    ];
    container.innerHTML = `
      <div class="widget-sun-times">
        ${rows.map(([label, t]) => `
          <div class="sun-row">
            <span class="sun-label">${label}</span>
            <span class="sun-time">${fmt(t, tz)}</span>
          </div>
        `).join('')}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'sunrise-sunset',
    domain: 'astro',
    defaultSize: { w: 3, h: 4 },
    subscriptionParams: () => null,
    render(container, state, config) { render(container, state, config); },
    onUpdate(container, data, config)  { render(container, data, config); },
  });
})();
