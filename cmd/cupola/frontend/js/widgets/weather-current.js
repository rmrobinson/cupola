(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function bearingToCompass(deg) {
    const dirs = ['N','NNE','NE','ENE','E','ESE','SE','SSE','S','SSW','SW','WSW','W','WNW','NW','NNW'];
    return dirs[Math.round((deg % 360) / 22.5) % 16];
  }

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  // WHO/Health Canada UV risk scale
  function uvRisk(uv) {
    if (!uv || uv <= 0)  return null;
    if (uv <= 2)  return { label: 'Low',       color: '#57d9a3', pct: uv/11*100 };
    if (uv <= 5)  return { label: 'Moderate',   color: '#f7b733', pct: uv/11*100 };
    if (uv <= 7)  return { label: 'High',       color: '#fc7b1a', pct: uv/11*100 };
    if (uv <= 10) return { label: 'Very High',  color: '#e53935', pct: uv/11*100 };
    return              { label: 'Extreme',     color: '#9c27b0', pct: 100 };
  }

  function render(container, data) {
    if (!data) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">Source unavailable</span><span style="font-size:10px;opacity:.5">weather.current</span></div>`;
      return;
    }
    const uv = uvRisk(data.uv);
    container.innerHTML = `
      <div class="widget-weather-current">
        <div class="wc-temp">${Math.round(data.temperature)}&deg;C</div>
        <div class="wc-condition">${esc(data.condition)}</div>
        <div class="wc-details">
          <span>Feels like ${Math.round(data.feels_like)}&deg;C</span>
          <span>Humidity ${data.humidity}%</span>
          <span>Wind ${Math.round(data.wind_speed)}&thinsp;km/h ${bearingToCompass(data.wind_direction)}</span>
          ${data.wind_gust > 0 ? `<span>Gusts ${Math.round(data.wind_gust)}&thinsp;km/h</span>` : ''}
          <span>Pressure ${data.pressure}&thinsp;hPa</span>
        </div>
        ${uv ? `
          <div class="wc-uv">
            <div class="wc-uv-header">
              <span class="wc-uv-value" style="color:${uv.color}">UV&thinsp;${data.uv.toFixed(0)}</span>
              <span class="wc-uv-label" style="color:${uv.color}">${uv.label}</span>
            </div>
            <div class="wc-uv-bar">
              <div class="wc-uv-fill" style="width:${Math.min(100,uv.pct).toFixed(0)}%;background:${uv.color}"></div>
            </div>
          </div>
        ` : ''}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'weather-current',
    domain: 'weather.current',
    defaultSize: { w: 3, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config)  { render(container, data); },
  });
})();
