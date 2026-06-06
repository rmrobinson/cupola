(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function color(index) {
    return index?.color || '#f7b733';
  }

  function renderUPI(aggregate) {
    if (!aggregate) return `<div class="pollen-empty-metric">UPI unavailable</div>`;
    return `
      <div class="pollen-current-metric">
        <div class="pollen-upi" style="color:${esc(aggregate.color || '#f7b733')}">${aggregate.value}</div>
        <div class="pollen-upi-label">
          <span>${esc(aggregate.category || 'UPI')}</span>
          <span>${esc(aggregate.label)}</span>
        </div>
      </div>
    `;
  }

  function renderRows(rows, cls) {
    rows = Array.isArray(rows) ? rows : [];
    if (!rows.length) return '';
    return `
      <div class="${cls}">
        ${rows.map(row => `
          <div class="pollen-row ${row.in_season ? 'in-season' : ''}">
            <span class="pollen-row-name">${esc(row.display_name || row.code)}</span>
            <span class="pollen-row-season">${row.in_season ? 'In season' : ''}</span>
            <span class="pollen-row-value" style="color:${esc(color(row.upi))}">${row.upi ? row.upi.value : '—'}</span>
            <span class="pollen-row-cat">${esc(row.upi?.category || '')}</span>
          </div>
        `).join('')}
      </div>
    `;
  }

  function render(container, state) {
    const day = state && state.current;
    if (!day) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No current pollen data</span><span style="font-size:10px;opacity:.5">weather.pollen</span></div>`;
      return;
    }
    container.innerHTML = `
      <div class="widget-weather-pollen-current">
        <div class="pollen-head">
          <span>Pollen</span>
          <span>${esc(day.date || '')}</span>
        </div>
        ${renderUPI(day.aggregate)}
        <div class="pollen-section-title">Types</div>
        ${renderRows(day.types, 'pollen-types')}
        <div class="pollen-section-title">Plants</div>
        ${renderRows((day.plants || []).filter(p => p.in_season || p.upi).slice(0, 8), 'pollen-plants')}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'weather-pollen-current',
    domain: 'weather.pollen',
    defaultSize: { w: 4, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, state, _config) { render(container, state); },
  });
})();
