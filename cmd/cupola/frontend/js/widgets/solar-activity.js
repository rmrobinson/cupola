(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function kpColor(kp) {
    if (kp < 2)  return '#74b9ff'; // quiet
    if (kp < 4)  return '#a8ff78'; // unsettled
    if (kp < 5)  return '#f7b733'; // active
    if (kp < 7)  return '#fc4a1a'; // minor/moderate storm
    return '#c0392b';              // strong+ storm
  }

  function fmtTime(iso) {
    if (!iso) return '—';
    return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  function render(container, data) {
    if (!data) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No solar data</span></div>`;
      return;
    }
    const kp    = data.kp_index ?? 0;
    const pct   = Math.min(100, (kp / 9) * 100).toFixed(0);
    const color = kpColor(kp);

    container.innerHTML = `
      <div class="widget-solar">
        <div class="solar-kp" style="color:${color}">${kp.toFixed(1)}</div>
        <div class="solar-kp-label">Kp index &mdash; ${data.kp_description || ''}</div>
        <div class="solar-bar">
          <div class="solar-bar-fill" style="width:${pct}%;background:${color}"></div>
        </div>
        ${data.aurora_viewable
          ? `<div class="solar-aurora">Aurora may be visible</div>`
          : `<div class="solar-aurora solar-aurora-dim">Aurora unlikely at this latitude</div>`
        }
        ${data.flare_class ? `<div class="solar-flare">Flare: ${data.flare_class}</div>` : ''}
        <div class="solar-region">Region ${data.region || '—'} &mdash; updated ${fmtTime(data.updated_at)}</div>
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'solar-activity',
    domain: 'solar.weather.current',
    defaultSize: { w: 3, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config)  { render(container, data); },
  });
})();
