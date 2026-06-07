(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function moonSVG(phase) {
    phase = ((Number(phase) || 0) % 1 + 1) % 1;
    const illum = (1 - Math.cos(2 * Math.PI * phase)) / 2;
    const r = 36;
    const cx = 50, cy = 50;

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

    const top = `${cx} ${cy - r}`;
    const bottom = `${cx} ${cy + r}`;
    const fmt = n => Number(n.toFixed(3));
    const crescentPath = (side, amount) => {
      const t = Math.max(0, Math.min(1, amount));
      const outerSweep = side === 'right' ? 1 : 0;
      const innerSweep = side === 'right' ? 0 : 1;
      if (t >= 0.995) {
        return `M ${top} A ${r} ${r} 0 0 ${outerSweep} ${bottom} L ${top} Z`;
      }
      const rx = fmt(r * (1 - t));
      return `M ${top} A ${r} ${r} 0 0 ${outerSweep} ${bottom} A ${rx} ${r} 0 0 ${innerSweep} ${top} Z`;
    };

    const uniqueId = 'mp' + Math.floor(phase * 10000);
    const clip = `<defs><clipPath id="${uniqueId}"><circle cx="${cx}" cy="${cy}" r="${r}"/></clipPath></defs>`;
    const border = `<circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="${borderColor}" stroke-width="1"/>`;
    if (phase < 0.25) {
      return `<svg viewBox="0 0 100 100" class="moon-visual">
        ${clip}
        <circle cx="${cx}" cy="${cy}" r="${r}" fill="${shadowColor}"/>
        <path d="${crescentPath('right', phase / 0.25)}" fill="${moonColor}" clip-path="url(#${uniqueId})"/>
        ${border}
      </svg>`;
    }
    if (phase < 0.5) {
      return `<svg viewBox="0 0 100 100" class="moon-visual">
        ${clip}
        <circle cx="${cx}" cy="${cy}" r="${r}" fill="${moonColor}"/>
        <path d="${crescentPath('left', 1 - ((phase - 0.25) / 0.25))}" fill="${shadowColor}" clip-path="url(#${uniqueId})"/>
        ${border}
      </svg>`;
    }
    if (phase < 0.75) {
      return `<svg viewBox="0 0 100 100" class="moon-visual">
        ${clip}
        <circle cx="${cx}" cy="${cy}" r="${r}" fill="${moonColor}"/>
        <path d="${crescentPath('right', (phase - 0.5) / 0.25)}" fill="${shadowColor}" clip-path="url(#${uniqueId})"/>
        ${border}
      </svg>`;
    }
    return `<svg viewBox="0 0 100 100" class="moon-visual">
      ${clip}
      <circle cx="${cx}" cy="${cy}" r="${r}" fill="${shadowColor}"/>
      <path d="${crescentPath('left', 1 - ((phase - 0.75) / 0.25))}" fill="${moonColor}" clip-path="url(#${uniqueId})"/>
      ${border}
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
