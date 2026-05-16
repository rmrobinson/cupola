(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function moonSVG(phase) {
    // Draw the moon using two overlapping circles.
    // Lit side is always on the right for waxing, left for waning.
    const illum = (1 - Math.cos(2 * Math.PI * phase)) / 2;
    const waxing = phase < 0.5;
    const r = 36;
    const cx = 50, cy = 50;

    // The illuminated fraction determines how much of the shadow circle overlaps.
    // shadow ellipse x-radius: at 0 illum → r (full shadow), at 1 → 0 (no shadow)
    const shadowRX = r * Math.abs(1 - 2 * illum);
    const shadowDir = waxing ? -1 : 1; // shadow side
    const shadowCX = cx + shadowDir * r * (1 - 2 * illum);

    const moonColor = 'rgba(255,230,100,0.88)';
    const shadowColor = '#0a0a1e';
    const borderColor = 'rgba(255,230,100,0.3)';

    if (illum < 0.03) {
      // New moon: dark disc with faint border
      return `<svg viewBox="0 0 100 100" class="moon-visual">
        <circle cx="${cx}" cy="${cy}" r="${r}" fill="${shadowColor}" stroke="${borderColor}" stroke-width="1.5"/>
      </svg>`;
    }
    if (illum > 0.97) {
      // Full moon
      return `<svg viewBox="0 0 100 100" class="moon-visual">
        <circle cx="${cx}" cy="${cy}" r="${r}" fill="${moonColor}"/>
      </svg>`;
    }

    const uniqueId = 'mp' + Math.floor(phase * 1000);
    return `<svg viewBox="0 0 100 100" class="moon-visual">
      <defs>
        <mask id="${uniqueId}">
          <circle cx="${cx}" cy="${cy}" r="${r}" fill="white"/>
          <ellipse cx="${shadowCX}" cy="${cy}" rx="${shadowRX}" ry="${r}" fill="black"/>
        </mask>
      </defs>
      <circle cx="${cx}" cy="${cy}" r="${r}" fill="${shadowColor}" stroke="${borderColor}" stroke-width="1"/>
      <circle cx="${cx}" cy="${cy}" r="${r}" fill="${moonColor}" mask="url(#${uniqueId})"/>
    </svg>`;
  }

  function render(container, data) {
    if (!data) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No astro data</span></div>`;
      return;
    }
    const illumPct = Math.round(data.moon_illumination * 100);
    const phase = data.moon_phase;
    container.innerHTML = `
      <div class="widget-moon">
        ${moonSVG(phase)}
        <div class="moon-phase-name">${data.moon_phase_name}</div>
        <div class="moon-illumination">${illumPct}% illuminated</div>
        <div class="moon-bar">
          <div class="moon-bar-fill" style="width:${illumPct}%"></div>
        </div>
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'moon-phase',
    domain: 'astro',
    defaultSize: { w: 3, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config)  { render(container, data); },
  });
})();
