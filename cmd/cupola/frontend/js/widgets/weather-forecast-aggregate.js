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

  function aqhiValue(v) {
    if (!v) return '—';
    if (v.value == null) return esc(v.risk || '—');
    return String(v.value);
  }

  function labelKey(label) {
    return String(label || '').trim().toLowerCase().replace(/\s+/g, ' ');
  }

  function dateKey(iso) {
    if (!iso) return '';
    const raw = String(iso);
    const m = raw.match(/^(\d{4}-\d{2}-\d{2})/);
    if (m) return m[1];
    const d = new Date(raw);
    if (Number.isNaN(d.getTime())) return '';
    const y = d.getFullYear();
    const mo = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${y}-${mo}-${day}`;
  }

  function localDateFromKey(key) {
    const m = String(key || '').match(/^(\d{4})-(\d{2})-(\d{2})$/);
    if (!m) return null;
    return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  }

  function pollenDateFromLabel(label, pollenDays) {
    const days = Array.isArray(pollenDays) ? pollenDays : [];
    if (!days.length) return '';
    const normalized = labelKey(label).replace(/\s+night$/, '');
    if (normalized === 'today' || normalized === 'tonight') return dateKey(days[0].date);
    if (normalized === 'tomorrow') return days[1] ? dateKey(days[1].date) : '';

    const weekdays = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];
    const target = weekdays.indexOf(normalized);
    if (target < 0) return '';
    for (const day of days) {
      const key = dateKey(day.date);
      const d = localDateFromKey(key);
      if (d && d.getDay() === target) return key;
    }
    return '';
  }

  function aqhiMetric(period) {
    const max = period?.max || null;
    const color = aqhiColor(max && max.value, max && max.risk);
    return {
      color,
      value: aqhiValue(max),
      risk: max && max.risk || '',
    };
  }

  function pollenTypeLabel(code, label) {
    const c = String(code || '').toUpperCase();
    if (c === 'GRASS' || c === 'TREE' || c === 'WEED') return label || c.charAt(0) + c.slice(1).toLowerCase();
    if (c === 'GRAMINALES') return 'Grass';
    if (c === 'RAGWEED' || c === 'MUGWORT') return 'Weed';
    if ([
      'ALDER',
      'ASH',
      'BIRCH',
      'COTTONWOOD',
      'ELM',
      'MAPLE',
      'OLIVE',
      'JUNIPER',
      'OAK',
      'PINE',
      'CYPRESS_PINE',
      'HAZEL',
      'JAPANESE_CEDAR',
      'JAPANESE_CYPRESS',
    ].includes(c)) return 'Tree';
    return label || 'Pollen';
  }

  function pollenMetric(day) {
    const agg = day?.aggregate || null;
    const label = agg ? pollenTypeLabel(agg.code, agg.label) : 'Pollen';
    return {
      color: agg?.color || '#f7b733',
      value: agg ? String(agg.value) : '—',
      label,
    };
  }

  function kpColor(kp) {
    if (kp < 2)  return '#74b9ff';
    if (kp < 4)  return '#a8ff78';
    if (kp < 5)  return '#f7b733';
    if (kp < 7)  return '#fc4a1a';
    return '#c0392b';
  }

  function fmtTime(iso) {
    if (!iso) return '—';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '—';
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  function renderWeather(data, aqhiData, pollenData, config) {
    const periods = data?.periods || [];
    const limit = config?.days_to_show ? config.days_to_show * 2 : 10;
    const shown = periods.slice(0, limit);

    const aqhiByLabel = new Map();
    for (const p of aqhiData?.forecasts || []) {
      const key = labelKey(p.label);
      if (key) aqhiByLabel.set(key, p);
    }

    const pollenByDate = new Map();
    for (const day of pollenData?.days || []) {
      const key = dateKey(day.date);
      if (key) pollenByDate.set(key, day);
    }

    const matchedAQHI = new Set();
    const matchedPollen = new Set();

    if (!shown.length && pollenByDate.size) {
      const pollenRows = Array.from(pollenByDate.values()).slice(0, 5).map(day => {
        const metric = pollenMetric(day);
        return `
          <div class="wfa-period has-pollen">
            <span class="wfa-label">${esc(day.date)}</span>
            <span class="wfa-condition">${esc(metric.label)}</span>
            <span class="wfa-temp"></span>
            <span class="wfa-pop"></span>
            <span class="wfa-inline-aqhi is-empty"></span>
            <span class="wfa-inline-pollen" style="color:${esc(metric.color)}">${esc(metric.label)} ${metric.value}</span>
          </div>
        `;
      }).join('');
      return { html: `<section class="wfa-weather">${pollenRows}</section>`, matchedAQHI, matchedPollen };
    }

    if (!shown.length) return { html: '', matchedAQHI, matchedPollen };

    const html = `
      <section class="wfa-weather">
        ${shown.map(p => {
          const key = labelKey(p.label);
          const aqhi = aqhiByLabel.get(key);
          if (aqhi) matchedAQHI.add(key);
          const metric = aqhiMetric(aqhi);
          const pDate = pollenDateFromLabel(p.label, pollenData?.days) || dateKey(p.starts_at || p.date);
          const pollen = pollenByDate.get(pDate);
          if (pollen) matchedPollen.add(pDate);
          const pMetric = pollenMetric(pollen);
          return `
            <div class="wfa-period ${aqhi ? 'has-aqhi' : ''} ${pollen ? 'has-pollen' : ''}">
              <span class="wfa-label">${esc(p.label)}</span>
              <span class="wfa-condition">${esc(p.condition || p.summary || '')}</span>
              <span class="wfa-temp">
                ${p.high != null ? `<span class="wfa-high">H:${Math.round(p.high)}&deg;</span>` : ''}
                ${p.low != null ? `<span class="wfa-low">L:${Math.round(p.low)}&deg;</span>` : ''}
              </span>
              <span class="wfa-pop">${p.precip_chance}%</span>
              ${aqhi ? `<span class="wfa-inline-aqhi" style="color:${metric.color}">AQHI ${metric.value}</span>` : '<span class="wfa-inline-aqhi is-empty"></span>'}
              ${pollen ? `<span class="wfa-inline-pollen" style="color:${esc(pMetric.color)}">${esc(pMetric.label)} ${pMetric.value}</span>` : '<span class="wfa-inline-pollen is-empty"></span>'}
            </div>
          `;
        }).join('')}
      </section>
    `;

    return { html, matchedAQHI };
  }

  function renderAQHI(data, matchedLabels) {
    const forecasts = Array.isArray(data?.forecasts)
      ? data.forecasts.filter(p => !matchedLabels?.has(labelKey(p.label))).slice(0, 5)
      : [];
    if (!forecasts.length) return '';

    return `
      <section class="wfa-strip">
        <div class="wfa-section-title">${esc(data.location || 'AQHI forecast')}</div>
        <div class="wfa-aqhi-list">
          ${forecasts.map(p => {
            const metric = aqhiMetric(p);
            return `
              <div class="wfa-aqhi-item">
                <span class="wfa-aqhi-label">${esc(p.label)}</span>
                <span class="wfa-aqhi-value" style="color:${metric.color}">${metric.value}</span>
                <span class="wfa-aqhi-risk">${esc(metric.risk)}</span>
              </div>
            `;
          }).join('')}
        </div>
      </section>
    `;
  }

  function renderSolar(data) {
    const periods = Array.isArray(data?.periods) ? data.periods.slice(0, 6) : [];
    if (!periods.length) return '';

    return `
      <section class="wfa-strip">
        <div class="wfa-section-title">Solar forecast</div>
        <div class="wfa-solar-list">
          ${periods.map(p => {
            const kp = p.kp_expected ?? 0;
            const color = kpColor(kp);
            return `
              <div class="wfa-solar-item">
                <span class="wfa-solar-time">${fmtTime(p.starts_at)}</span>
                <span class="wfa-solar-kp" style="color:${color}">${kp.toFixed(1)}</span>
                <span class="wfa-solar-desc">${esc(p.kp_description || p.summary || '')}</span>
                <span class="wfa-solar-aurora ${p.aurora_viewable ? '' : 'is-dim'}">${p.aurora_viewable ? 'Aurora' : 'Quiet'}</span>
              </div>
            `;
          }).join('')}
        </div>
      </section>
    `;
  }

  function render(container, stateMap, config) {
    const weather = renderWeather(
      stateMap && stateMap['weather.forecast'],
      stateMap && stateMap['weather.air_quality'],
      stateMap && stateMap['weather.pollen'],
      config || {}
    );
    const weatherHTML = weather.html;
    const aqhiHTML = renderAQHI(stateMap && stateMap['weather.air_quality'], weather.matchedAQHI);
    const solarHTML = renderSolar(stateMap && stateMap['solar.weather.forecast']);

    if (!weatherHTML && !aqhiHTML && !solarHTML) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No forecast data</span><span style="font-size:10px;opacity:.5">weather aggregate</span></div>`;
      return;
    }

    container.innerHTML = `
      <div class="widget-weather-forecast-aggregate">
        ${weatherHTML}
        ${aqhiHTML}
        ${solarHTML}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'weather-forecast-aggregate',
    domains: ['weather.forecast', 'weather.air_quality', 'solar.weather.forecast', 'weather.pollen'],
    defaultSize: { w: 8, h: 7 },
    subscriptionParams: () => null,
    render(container, stateMap, config) { render(container, stateMap, config); },
    onUpdate(container, stateMap, config) { render(container, stateMap, config); },
  });
})();
