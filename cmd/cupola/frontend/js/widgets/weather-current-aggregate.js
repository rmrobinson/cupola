(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function bearingToCompass(deg) {
    if (deg == null) return '';
    const dirs = ['N','NNE','NE','ENE','E','ESE','SE','SSE','S','SSW','SW','WSW','W','WNW','NW','NNW'];
    return dirs[Math.round((deg % 360) / 22.5) % 16];
  }

  function uvRisk(uv) {
    if (uv == null) return null;
    if (uv <= 0)  return { label: 'None',      color: 'rgba(255,255,255,0.35)', pct: 0 };
    if (uv <= 2)  return { label: 'Low',       color: '#57d9a3', pct: uv/11*100 };
    if (uv <= 5)  return { label: 'Moderate',  color: '#f7b733', pct: uv/11*100 };
    if (uv <= 7)  return { label: 'High',      color: '#fc7b1a', pct: uv/11*100 };
    if (uv <= 10) return { label: 'Very High', color: '#e53935', pct: uv/11*100 };
    return              { label: 'Extreme',    color: '#9c27b0', pct: 100 };
  }

  function aqhiColor(value, risk) {
    const label = String(risk || '').toLowerCase();
    if (label.includes('very high') || value > 10) return '#c0392b';
    if (label.includes('high') || value >= 7) return '#e53935';
    if (label.includes('moderate') || value >= 4) return '#f7b733';
    if (label.includes('low') || value >= 1) return '#57d9a3';
    return 'rgba(255,255,255,0.45)';
  }

  function aqhiValue(v) {
    if (!v) return '—';
    if (v.value == null) return esc(v.risk || '—');
    return String(v.value);
  }

  function kpColor(kp) {
    if (kp < 2)  return '#74b9ff';
    if (kp < 4)  return '#a8ff78';
    if (kp < 5)  return '#f7b733';
    if (kp < 7)  return '#fc4a1a';
    return '#c0392b';
  }

  function kpDescription(data, kp) {
    if (data?.kp_description) return data.kp_description;
    if (kp < 2) return 'Quiet';
    if (kp < 4) return 'Unsettled';
    if (kp < 5) return 'Active';
    if (kp < 7) return 'Storm';
    return 'Strong storm';
  }

  function renderWeather(data) {
    if (!data) return '';
    const uv = uvRisk(data.uv);
    return `
      <section class="wca-current">
        <div class="wca-temp">${Math.round(data.feels_like)}&deg;C</div>
        <div class="wca-condition">${esc(data.condition)}</div>
        <div class="wca-details">
          <span>Currently ${Math.round(data.temperature)}&deg;C</span>
          <span>Humidity ${Math.round(data.humidity)}%</span>
          <span>Wind ${Math.round(data.wind_speed)} km/h ${bearingToCompass(data.wind_direction)}</span>
          ${data.wind_gust > 0 ? `<span>Gust ${Math.round(data.wind_gust)} km/h</span>` : ''}
          <span>Pressure ${Math.round(data.pressure)} hPa</span>
        </div>
        ${uv ? `
          <div class="wca-bar-block">
            <div class="wca-metric-head">
              <span style="color:${uv.color}">UV ${data.uv.toFixed(0)}</span>
              <span>${uv.label}</span>
            </div>
            <div class="wca-bar"><div class="wca-bar-fill" style="width:${Math.min(100, uv.pct).toFixed(0)}%;background:${uv.color}"></div></div>
          </div>
        ` : ''}
      </section>
    `;
  }

  function renderAQHI(data) {
    if (!data) return '';
    const observed = data.observed || null;
    const value = observed && observed.value != null ? observed.value : null;
    const risk = observed ? observed.risk : '';
    const color = aqhiColor(value, risk);
    const place = [data.location, data.province].filter(Boolean).join(' ');
    return `
      <section class="wca-side-section wca-aqhi">
        <div class="wca-side-title">Air quality</div>
        <div class="wca-side-main">
          <span class="wca-side-value" style="color:${color}">${aqhiValue(observed)}</span>
          <span class="wca-side-label">${esc(risk || 'AQHI')}</span>
        </div>
        <div class="wca-side-bar"><div class="wca-side-bar-fill" style="width:${Math.min(100, Math.max(0, ((value || 0) / 10) * 100)).toFixed(0)}%;background:${color}"></div></div>
        ${place ? `<div class="wca-side-note">${esc(place)}</div>` : ''}
      </section>
    `;
  }

  function renderSolar(data) {
    if (!data) return '';
    const kp = data.kp_index ?? 0;
    const color = kpColor(kp);
    const desc = kpDescription(data, kp);
    return `
      <section class="wca-side-section wca-solar">
        <div class="wca-side-title">Solar weather</div>
        <div class="wca-side-main">
          <span class="wca-side-value" style="color:${color}">${kp.toFixed(1)}</span>
          <span class="wca-side-label">${esc(desc)}</span>
        </div>
        <div class="wca-side-bar"><div class="wca-side-bar-fill" style="width:${Math.min(100, (kp / 9) * 100).toFixed(0)}%;background:${color}"></div></div>
        <div class="wca-side-note">${data.aurora_viewable ? 'Aurora possible' : 'Aurora unlikely'}${data.flare_class ? ` · Flare ${esc(data.flare_class)}` : ''}</div>
      </section>
    `;
  }

  function render(container, stateMap) {
    const weather = stateMap && stateMap['weather.current'];
    const aqhi = stateMap && stateMap['weather.air_quality'];
    const solar = stateMap && stateMap['solar.weather.current'];

    if (!weather && !aqhi && !solar) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">Source unavailable</span><span style="font-size:10px;opacity:.5">weather aggregate</span></div>`;
      return;
    }

    container.innerHTML = `
      <div class="widget-weather-current-aggregate ${weather ? 'has-weather' : 'no-weather'}">
        ${renderWeather(weather)}
        <div class="wca-side">
          ${renderAQHI(aqhi)}
          ${renderSolar(solar)}
        </div>
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'weather-current-aggregate',
    domains: ['weather.current', 'weather.air_quality', 'solar.weather.current'],
    defaultSize: { w: 6, h: 4 },
    subscriptionParams: () => null,
    render(container, stateMap, _config) { render(container, stateMap); },
    onUpdate(container, stateMap, _config) { render(container, stateMap); },
  });
})();
