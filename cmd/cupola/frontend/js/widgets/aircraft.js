(function () {
  'use strict';
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function bearingArrow(track) {
    // 8 compass arrows offset by 22.5° so N is centred on 0/360.
    const arrows = ['↑','↗','→','↘','↓','↙','←','↖'];
    return arrows[Math.round(((track % 360) + 360) % 360 / 45) % 8];
  }

  function fmtAlt(ft) {
    if (ft == null || ft === 0) return '—';
    if (ft >= 1000) return (ft / 1000).toFixed(1) + 'k ft';
    return ft + ' ft';
  }

  function render(container, data) {
    const aircraft = data?.aircraft ?? [];

    if (!aircraft.length) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No aircraft in range</span></div>`;
      return;
    }

    // Sort: airborne first by descending altitude, then ground traffic.
    const sorted = [...aircraft].sort((a, b) => {
      if (a.on_ground !== b.on_ground) return a.on_ground ? 1 : -1;
      return (b.alt_ft ?? 0) - (a.alt_ft ?? 0);
    });

    const rows = sorted.map(a => {
      const label = esc(a.flight || a.callsign || a.icao);
      const alt   = a.on_ground ? '<span class="ac-ground">GND</span>' : esc(fmtAlt(a.alt_ft));
      const hdg   = a.track != null ? `<span class="ac-track">${bearingArrow(a.track)}</span>` : '';
      const spd   = a.speed_kts != null ? `${Math.round(a.speed_kts)}&thinsp;kt` : '';
      return `
        <div class="ac-row">
          <span class="ac-label">${label}</span>
          <span class="ac-alt">${alt}</span>
          <span class="ac-spd">${hdg}${spd}</span>
        </div>`;
    }).join('');

    container.innerHTML = `<div class="widget-aircraft"><div class="ac-list">${rows}</div></div>`;
  }

  window.CupolaWidgets.push({
    type:    'aircraft',
    domain:  'aircraft',
    defaultSize: { w: 2, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config) { render(container, data); },
  });
})();
