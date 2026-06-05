(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function escAttr(s) {
    return esc(s).replace(/"/g, '&quot;');
  }

  function fmtDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleString(undefined, {
      month: 'short', day: 'numeric',
      hour: 'numeric', minute: '2-digit',
    });
  }

  function render(container, state, config) {
    if (!state) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">Source unavailable</span><span style="font-size:10px;opacity:.5">municipal.events</span></div>`;
      return;
    }

    let events = state.events || [];

    const sourceFilter = (config?.source || '').trim();
    if (sourceFilter) {
      const filters = sourceFilter.split(',').map(s => s.trim()).filter(Boolean);
      events = events.filter(e => filters.some(f => e.source_id === f || e.source_id?.startsWith(f)));
    }

    events = events.slice().sort((a, b) =>
      new Date(b.published_at || 0).getTime() - new Date(a.published_at || 0).getTime()
    );

    if (events.length === 0) {
      container.innerHTML = `<div class="widget-muni-events"><p class="muni-events-empty">No upcoming events</p></div>`;
      return;
    }

    const cards = events.map(ev => {
      const startsStr = ev.starts_at ? fmtDate(ev.starts_at) : '';
      const endsStr   = ev.ends_at   ? ` – ${fmtDate(ev.ends_at)}` : '';
      const timeRange = startsStr ? `<div class="muni-event-time">${esc(startsStr)}${esc(endsStr)}</div>` : '';
      const source = ev.source_id ? `<span class="muni-event-source">${esc(ev.source_id)}</span>` : '';
      const type = ev.event_type ? `<span class="muni-event-type">${esc(ev.event_type.replace(/-/g, ' '))}</span>` : '';
      const desc = ev.description
        ? `<div class="muni-event-desc">${esc(ev.description)}</div>`
        : '';
      return `
        <div class="muni-event-card detail-clickable"
             data-detail-domain="municipal.events" data-detail-id="${escAttr(ev.id)}"
             role="button" tabindex="0">
          <div class="muni-event-card-top">
            ${type}${source}
          </div>
          <div class="muni-event-title">${esc(ev.title)}</div>
          ${timeRange}
          ${desc}
        </div>
      `;
    }).join('');

    container.innerHTML = `<div class="widget-muni-events">${cards}</div>`;
  }

  window.CupolaWidgets.push({
    type: 'municipal-events',
    domain: 'municipal.events',
    defaultSize: { w: 7, h: 4 },
    configSchema: [
      { key: 'source', label: 'Source filter (comma-sep)', type: 'text', default: '', placeholder: 'e.g. city.roadwork' },
    ],
    subscriptionParams: () => null,
    render(container, state, config)  { render(container, state, config); },
    onUpdate(container, data, config)  { render(container, data, config); },
  });
})();
