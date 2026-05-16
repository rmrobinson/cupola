(function () {
  'use strict';
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function bearingArrow(track) {
    const arrows = ['↑','↗','→','↘','↓','↙','←','↖'];
    return arrows[Math.round(((track % 360) + 360) % 360 / 45) % 8];
  }

  function fmtAlt(ft, onGround) {
    if (onGround) return '<span class="ac-ground">GND</span>';
    if (!ft) return '—';
    return ft >= 1000 ? (ft / 1000).toFixed(1) + 'k ft' : ft + ' ft';
  }

  function distNm(lat1, lon1, lat2, lon2) {
    const R = 3440.065;
    const dLat = (lat2 - lat1) * Math.PI / 180;
    const dLon = (lon2 - lon1) * Math.PI / 180;
    const a = Math.sin(dLat / 2) ** 2 +
              Math.cos(lat1 * Math.PI / 180) * Math.cos(lat2 * Math.PI / 180) *
              Math.sin(dLon / 2) ** 2;
    return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  }

  function render(container, state, config) {
    const aircraft = state?.aircraft ?? [];
    const cLat = config?.center_lat ?? window.CupolaConfig?.lat;
    const cLon = config?.center_lon ?? window.CupolaConfig?.lon;
    const hasCenter = cLat != null && cLon != null;

    if (!aircraft.length) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No aircraft in range</span></div>`;
      return;
    }

    const withDist = aircraft.map(a => ({
      a,
      dist: hasCenter ? distNm(cLat, cLon, a.lat, a.lon) : null,
    }));

    if (hasCenter) {
      withDist.sort((x, y) => (x.dist ?? Infinity) - (y.dist ?? Infinity));
    } else {
      // Fall back to original sort: airborne first by descending altitude.
      withDist.sort((x, y) => {
        if (x.a.on_ground !== y.a.on_ground) return x.a.on_ground ? 1 : -1;
        return (y.a.alt_ft ?? 0) - (x.a.alt_ft ?? 0);
      });
    }

    const rows = withDist.map(({ a, dist }) => {
      const label = esc(a.flight || a.callsign || a.icao);
      const alt   = fmtAlt(a.alt_ft, a.on_ground);
      const hdg   = a.track != null ? `<span class="ac-track">${bearingArrow(a.track)}</span>` : '';
      const spd   = a.speed_kts != null ? `${Math.round(a.speed_kts)}&thinsp;kt` : '';
      const nm    = dist != null ? `<span class="ac-dist">${dist.toFixed(0)}<span class="ac-dist-unit">nm</span></span>` : '';
      return `
        <div class="ac-row">
          <span class="ac-label">${label}</span>
          <span class="ac-alt">${alt}</span>
          <span class="ac-spd">${hdg}${spd}</span>
          ${nm}
        </div>`;
    }).join('');

    container.innerHTML = `<div class="widget-aircraft"><div class="ac-list">${rows}</div></div>`;
  }

  window.CupolaWidgets.push({
    type:    'aircraft',
    domain:  'aircraft',
    defaultSize: { w: 4, h: 4 },
    configSchema: [
      { key: 'center_lat', label: 'Center latitude',  type: 'number', placeholder: () => window.CupolaConfig?.lat ?? '' },
      { key: 'center_lon', label: 'Center longitude', type: 'number', placeholder: () => window.CupolaConfig?.lon ?? '' },
    ],
    subscriptionParams: () => null,
    render(container, state, config) { render(container, state, config); },
    onUpdate(container, state, config) { render(container, state, config); },
  });
})();
