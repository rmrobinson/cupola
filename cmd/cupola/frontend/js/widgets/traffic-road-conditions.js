(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // Severity heuristic: anything other than "Bare and dry road" is noteworthy.
  function conditionClass(conditions) {
    if (!conditions || conditions.length === 0) return '';
    const lower = conditions.join(' ').toLowerCase();
    if (lower.includes('closed') || lower.includes('impassable')) return 'road-cond-severe';
    if (lower.includes('snow') || lower.includes('ice') || lower.includes('frost') ||
        lower.includes('slippery') || lower.includes('packed')) return 'road-cond-warn';
    if (lower.includes('wet') || lower.includes('construction')) return 'road-cond-caution';
    return '';
  }

  function render(container, state, config) {
    const roadFilter    = (config?.road || '').trim().toUpperCase();
    // regions is stored as an array; accept a legacy string too
    const rawRegions    = config?.regions ?? (config?.region ? [config.region] : []);
    const regionFilter  = Array.isArray(rawRegions) ? rawRegions.filter(Boolean) : [];
    const maxN = config?.max_segments > 0 ? Number(config.max_segments) : 10;

    let segments = state?.conditions || [];
    if (roadFilter) {
      segments = segments.filter(c =>
        (c.roadway_name || '').toUpperCase().includes(roadFilter)
      );
    }
    if (regionFilter.length > 0) {
      segments = segments.filter(c => regionFilter.includes(c.region || ''));
    }

    // Show adverse conditions first, then limit.
    segments = segments
      .slice()
      .sort((a, b) => {
        const aOk = conditionClass(a.conditions) === '';
        const bOk = conditionClass(b.conditions) === '';
        if (aOk !== bOk) return aOk ? 1 : -1;
        return 0;
      })
      .slice(0, maxN);

    const filterLabels = [
      roadFilter ? `<span class="road-filter-label">${esc(roadFilter)}</span>` : '',
      ...regionFilter.map(rg => `<span class="road-filter-label">${esc(rg)}</span>`),
    ].join('');

    if (segments.length === 0) {
      container.innerHTML = `
        <div class="widget-road-conditions">
          <div class="traffic-header">
            <span class="traffic-title">Road Conditions</span>
            ${filterLabels}
          </div>
          <p class="traffic-empty">${state ? 'No road condition data' : 'Waiting for data…'}</p>
        </div>`;
      return;
    }

    container.innerHTML = `
      <div class="widget-road-conditions">
        <div class="traffic-header">
          <span class="traffic-title">Road Conditions</span>
          ${filterLabels}
        </div>
        <div class="road-cond-list">
          ${segments.map(segmentRow).join('')}
        </div>
      </div>`;
  }

  function segmentRow(seg) {
    const cls = conditionClass(seg.conditions);
    const conds = (seg.conditions || []).join(', ') || 'Unknown';
    const vis = seg.visibility && seg.visibility.toLowerCase() !== 'good' ? seg.visibility : null;
    const drifting = seg.drifting && seg.drifting.toLowerCase() === 'yes';

    return `
      <div class="road-cond-row ${cls}">
        <div class="road-cond-top">
          <span class="road-cond-name">${esc(seg.roadway_name)}</span>
          <span class="road-cond-status">${esc(conds)}</span>
        </div>
        <div class="road-cond-desc">${esc(seg.location_description)}</div>
        ${vis || drifting ? `
          <div class="road-cond-extras">
            ${vis ? `<span class="road-cond-vis">Visibility: ${esc(vis)}</span>` : ''}
            ${drifting ? `<span class="road-cond-drift">Drifting snow</span>` : ''}
          </div>` : ''}
      </div>`;
  }

  async function buildConfig(panel, wc, onSave) {
    panel.innerHTML = '<p class="config-loading">Loading regions…</p>';

    let conditions = [];
    try {
      const r = await fetch('/api/v1/state/traffic.road_conditions');
      if (r.ok) {
        const s = await r.json();
        conditions = s?.conditions || [];
      }
    } catch {}

    const regions = [...new Set(conditions.map(c => c.region).filter(Boolean))].sort();

    const cfg     = wc.config || {};
    const road    = cfg.road || '';
    // accept legacy single-string region
    const selRegs = Array.isArray(cfg.regions) ? cfg.regions : (cfg.region ? [cfg.region] : []);
    const maxSeg  = cfg.max_segments != null ? cfg.max_segments : 10;

    const regionOpts = regions.map(rg =>
      `<option value="${esc(rg)}"${selRegs.includes(rg) ? ' selected' : ''}>${esc(rg)}</option>`
    ).join('');

    panel.innerHTML = `
      <form class="config-form config-form-wide">
        <label class="config-row">
          <span>Road filter</span>
          <input type="text" name="road" value="${esc(road)}" placeholder="e.g. 401">
        </label>
        <label class="config-row config-row-multiselect">
          <span>Regions</span>
          <select name="regions" multiple size="${Math.min(regions.length, 6)}">${regionOpts}</select>
        </label>
        <label class="config-row">
          <span>Max segments</span>
          <input type="number" name="max_segments" min="1" max="100" value="${esc(maxSeg)}">
        </label>
        <div class="config-actions">
          <button type="submit" class="btn-small btn-primary">Save</button>
          <button type="button" class="btn-small btn-secondary btn-config-cancel">Cancel</button>
        </div>
      </form>`;

    panel.querySelector('.btn-config-cancel').addEventListener('click', () => {
      panel.classList.add('hidden');
    });

    panel.querySelector('.config-form').addEventListener('submit', e => {
      e.preventDefault();
      const data = new FormData(e.target);
      wc.config = {
        road:         data.get('road') || '',
        regions:      data.getAll('regions'),
        max_segments: Number(data.get('max_segments')) || 10,
      };
      onSave();
    });
  }

  window.CupolaWidgets.push({
    type:        'traffic-road-conditions',
    domain:      'traffic.road_conditions',
    defaultSize: { w: 3, h: 5 },
    buildConfig,
    subscriptionParams: () => null,
    render(container, state, config)  { render(container, state, config); },
    onUpdate(container, data, config) { render(container, data, config); },
  });
})();
