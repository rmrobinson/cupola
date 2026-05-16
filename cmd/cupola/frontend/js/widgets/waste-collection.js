(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  // Collection type → display label and accent color
  const TYPES = {
    'Organics':    { label: 'Organics',    color: '#3a9c3a' },
    'Garbage':     { label: 'Garbage',     color: '#888' },
    'Recycling':   { label: 'Recycling',   color: '#2979c2' },
    'Yard Waste':  { label: 'Yard Waste',  color: '#a07830' },
    'Bulky Items': { label: 'Bulky Items', color: '#8a4fbf' },
  };

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function fmtDate(iso) {
    if (!iso) return '';
    // Parse as local date to avoid UTC-midnight timezone shifts
    const [y, m, d] = iso.split('-').map(Number);
    return new Date(y, m - 1, d).toLocaleDateString(undefined, {
      weekday: 'long', month: 'long', day: 'numeric',
    });
  }

  function render(container, data) {
    const inner = container.closest('.widget-inner');

    if (!data || !data.collections || data.collections.length === 0) {
      if (inner) inner.style.background = '';
      container.innerHTML = `<div class="widget-waste-collection widget-waste-empty">
        <span class="waste-no-collection">No collection this week</span>
      </div>`;
      return;
    }

    const anyToday = data.is_today || (data.extra_collections || []).some(e => e.is_today);
    if (inner) {
      inner.style.background = anyToday ? 'rgba(180, 140, 0, 0.45)' : '';
    }

    const pills = data.collections.map(name => {
      const t = TYPES[name] || { label: name, color: '#666' };
      return `<span class="waste-pill" style="background:${t.color}20;border-color:${t.color};color:#fff">${t.label}</span>`;
    }).join('');

    const extras = (data.extra_collections || []).map(e => `
      <div class="waste-extra-row${e.is_today ? ' waste-extra-today' : ''}">
        <span class="waste-extra-dot"></span>
        <span class="waste-extra-type">${esc(e.type)}</span>
        <span class="waste-extra-day">${esc(e.day_of_week)}</span>
      </div>`).join('');

    container.innerHTML = `
      <div class="widget-waste-collection${anyToday ? ' waste-today' : ''}">
        <div class="waste-date">${fmtDate(data.date)}</div>
        <div class="waste-pills">${pills}</div>
        ${extras ? `<div class="waste-extras">${extras}</div>` : ''}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'waste-collection',
    domain: 'waste.collection',
    defaultSize: { w: 5, h: 2 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config)  { render(container, data); },
  });
})();
