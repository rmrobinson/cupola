(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function color(index, fallback) {
    return index?.color || fallback || '#f7b733';
  }

  function rowsHTML(rows, limit) {
    rows = Array.isArray(rows) ? rows.slice(0, limit) : [];
    return rows.map(row => `
      <div class="pollen-mini-row ${row.in_season ? 'in-season' : ''}">
        <span>${esc(row.display_name || row.code)}</span>
        <span style="color:${esc(color(row.upi))}">${row.upi ? row.upi.value : '—'}</span>
        <span>${esc(row.upi?.category || '')}</span>
      </div>
    `).join('');
  }

  function renderDay(day) {
    const agg = day.aggregate || null;
    return `
      <section class="pollen-forecast-day">
        <div class="pollen-day-head">
          <span>${esc(day.date)}</span>
          <span style="color:${esc(agg?.color || '#f7b733')}">${agg ? `UPI ${agg.value}` : 'UPI —'}</span>
        </div>
        <div class="pollen-day-label">${esc(agg ? `${agg.label} ${agg.category || ''}` : 'No index')}</div>
        <div class="pollen-day-grid">
          <div>
            <div class="pollen-section-title">Types</div>
            ${rowsHTML(day.types, 3)}
          </div>
          <div>
            <div class="pollen-section-title">Plants</div>
            ${rowsHTML((day.plants || []).filter(p => p.in_season || p.upi), 8)}
          </div>
        </div>
      </section>
    `;
  }

  function render(container, state) {
    const days = Array.isArray(state?.days) ? state.days.slice(0, 5) : [];
    if (!days.length) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No pollen forecast</span><span style="font-size:10px;opacity:.5">weather.pollen</span></div>`;
      return;
    }
    container.innerHTML = `
      <div class="widget-weather-pollen-forecast">
        ${days.map(renderDay).join('')}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'weather-pollen-forecast',
    domain: 'weather.pollen',
    defaultSize: { w: 6, h: 6 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, state, _config) { render(container, state); },
  });
})();
