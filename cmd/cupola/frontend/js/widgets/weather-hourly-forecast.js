(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function fmtTime(value) {
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  }

  function fmtDateTime(value) {
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleString([], {
      month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit',
    });
  }

  function rounded(value) {
    return value == null ? null : Math.round(value);
  }

  // WHO/Health Canada UV risk scale. Keep colors in sync with weather-current.
  function uvRisk(uv) {
    if (uv == null) return null;
    if (uv <= 0)  return { label: 'None',      color: 'rgba(255,255,255,0.35)' };
    if (uv <= 2)  return { label: 'Low',       color: '#57d9a3' };
    if (uv <= 5)  return { label: 'Moderate',  color: '#f7b733' };
    if (uv <= 7)  return { label: 'High',      color: '#fc7b1a' };
    if (uv <= 10) return { label: 'Very High', color: '#e53935' };
    return              { label: 'Extreme',    color: '#9c27b0' };
  }

  function metric(label, value, suffix) {
    if (value == null) return '';
    return `<span>${label} ${rounded(value)}${suffix || ''}</span>`;
  }

  function periodEnd(h) {
    const explicit = Date.parse(h.ends_at);
    if (!Number.isNaN(explicit)) return explicit;
    const start = Date.parse(h.starts_at);
    return Number.isNaN(start) ? NaN : start + 60 * 60 * 1000;
  }

  function currentHourStart(now) {
    const d = new Date(now);
    d.setMinutes(0, 0, 0);
    return d.getTime();
  }

  function isCurrentOrFutureHour(h, now) {
    const start = Date.parse(h.starts_at);
    if (!Number.isNaN(start)) return start >= currentHourStart(now);
    const end = periodEnd(h);
    return Number.isNaN(end) || end > now.getTime();
  }

  function displayTemp(h) {
    const apparent = h.humidex ?? h.wind_chill;
    const actual = h.temperature;
    const uv = uvRisk(h.uv_index);
    const uvText = uv ? `<span class="hp-temp-uv" style="color:${uv.color}">UV ${rounded(h.uv_index)}</span>` : '';
    if (apparent == null) {
      return actual == null ? uvText : `<span class="hp-temp-line"><span class="hp-temp-main">${rounded(actual)}&deg;</span></span>${uvText}`;
    }
    return `
      <span class="hp-temp-line">
        <span class="hp-temp-main">${rounded(apparent)}&deg;</span>
        ${actual == null ? '' : `<span class="hp-temp-actual">${rounded(actual)}&deg;</span>`}
      </span>
      ${uvText}
    `;
  }

  function detailFields(h) {
    const fields = [
      { key: 'starts_at', value: h.starts_at },
      { key: 'ends_at', value: h.ends_at },
      { key: 'condition', value: h.condition },
      { key: 'temperature', value: h.temperature, unit: 'celsius' },
      { key: 'feels_like', value: h.feels_like, unit: 'celsius' },
      { key: 'humidex', value: h.humidex, unit: 'celsius' },
      { key: 'wind_chill', value: h.wind_chill, unit: 'celsius' },
      { key: 'precip_chance', value: h.precip_chance, unit: 'percent' },
      { key: 'uv_index', value: h.uv_index },
      { key: 'wind_direction', value: h.wind_direction },
      { key: 'wind_speed', value: h.wind_speed, unit: 'km_per_h' },
      { key: 'wind_gust', value: h.wind_gust, unit: 'km_per_h' },
    ];
    return fields.filter(f => f.value != null && f.value !== '');
  }

  function openDetail(h) {
    window.CupolaDetails?.show?.({
      domain: 'weather.forecast.hourly',
      title: fmtTime(h.starts_at) || 'Hourly forecast',
      subtitle: h.condition || '',
      fields: detailFields(h),
    });
  }

  function bindRows(container, hours) {
    if (!container.querySelectorAll) return;
    container.querySelectorAll('.hourly-period').forEach(row => {
      row.addEventListener('click', () => {
        const idx = Number(row.dataset.hourIndex);
        if (Number.isFinite(idx) && hours[idx]) openDetail(hours[idx]);
      });
      row.addEventListener('keydown', e => {
        if (e.key !== 'Enter' && e.key !== ' ') return;
        e.preventDefault();
        const idx = Number(row.dataset.hourIndex);
        if (Number.isFinite(idx) && hours[idx]) openDetail(hours[idx]);
      });
    });
  }

  function render(container, data) {
    const now = new Date();
    const hours = (data?.hours || []).filter(h => {
      return isCurrentOrFutureHour(h, now);
    });
    if (!hours.length) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No hourly forecast</span></div>`;
      return;
    }
    container.innerHTML = `
      <div class="widget-weather-hourly">
        <div class="hourly-header" aria-hidden="true">
          <span class="hp-time">Time</span>
          <span></span>
          <span class="hp-temp">Temp</span>
          <span class="hp-pop">POP</span>
          <span class="hp-wind">Wind</span>
        </div>
        ${hours.map((h, idx) => {
          const wind = [
            h.wind_direction || '',
            h.wind_speed != null ? `${rounded(h.wind_speed)} km/h` : '',
            h.wind_gust != null ? `G ${rounded(h.wind_gust)}` : '',
          ].filter(Boolean).join(' ');
          const hasIcon = !!h.icon_url;
          return `
            <div class="hourly-period ${hasIcon ? 'has-icon' : 'no-icon'}" data-hour-index="${idx}" role="button" tabindex="0" title="${esc(fmtDateTime(h.starts_at))}">
              <span class="hp-time">${esc(fmtTime(h.starts_at))}</span>
              <span class="hp-icon-wrap">${hasIcon ? `<img class="hp-icon" src="${esc(h.icon_url)}" alt="${esc(h.condition)}" loading="lazy" onerror="const row=this.closest('.hourly-period');row?.classList.remove('has-icon');row?.classList.add('no-icon');this.hidden=true">` : ''}</span>
              <span class="hp-condition">${esc(h.condition || '')}</span>
              <span class="hp-temp">${displayTemp(h)}</span>
              <span class="hp-pop">${h.precip_chance == null ? '' : `${h.precip_chance}%`}</span>
              <span class="hp-wind">${esc(wind)}</span>
              <span class="hp-extra">
                ${h.wind_gust == null ? '' : metric('Gust', h.wind_gust, '')}
              </span>
            </div>
          `;
        }).join('')}
      </div>
    `;
    bindRows(container, hours);
  }

  window.CupolaWidgets.push({
    type: 'weather-hourly-forecast',
    domain: 'weather.forecast.hourly',
    defaultSize: { w: 7, h: 7 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config)  { render(container, data); },
  });
})();
