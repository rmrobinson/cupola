(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  const SEVERITY_ORDER = { emergency: 0, warning: 1, watch: 2, info: 3 };
  const SEVERITY_COLOR = {
    emergency: { bg: 'rgba(220,38,38,0.3)',  text: '#ff8080',  label: 'Emergency' },
    warning:   { bg: 'rgba(220,150,30,0.3)', text: '#ffc060',  label: 'Warning'   },
    watch:     { bg: 'rgba(200,180,30,0.3)', text: '#ffe066',  label: 'Watch'     },
    info:      { bg: 'rgba(100,160,220,0.3)',text: '#74b9ff',  label: 'Info'      },
  };

  // Maps alert_type values to the emoji shown at the polygon centroid on the map.
  const ALERT_TYPE_EMOJI = {
    'power-outage': '⚡',
    'water-outage': '💧',
  };

  const SOURCE_LABEL = {
    'weather.alerts':   'Weather',
    'transit.alerts':   'Transit',
    'municipal.alerts': 'Municipal',
  };

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  function escAttr(s) {
    return esc(s).replace(/"/g, '&quot;');
  }

  function fmtTime(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleString(undefined, {
      month: 'short', day: 'numeric',
      hour: 'numeric', minute: '2-digit',
    });
  }

  function collect(stateMap, config) {
    const show = {
      'weather.alerts':   config.showWeather   !== false,
      'transit.alerts':   config.showTransit   !== false,
      'municipal.alerts': config.showMunicipal !== false,
    };

    const items = [];
    for (const [domain, state] of Object.entries(stateMap)) {
      if (!show[domain] || !state) continue;
      const alerts = state.alerts || [];
      for (const a of alerts) {
        items.push({ ...a, _domain: domain });
      }
    }

    items.sort((a, b) => {
      const sd = (SEVERITY_ORDER[a.severity] ?? 9) - (SEVERITY_ORDER[b.severity] ?? 9);
      if (sd !== 0) return sd;
      const ta = new Date(a.published_at || a.onset || 0).getTime();
      const tb = new Date(b.published_at || b.onset || 0).getTime();
      return tb - ta;
    });
    return items;
  }

  // ── Map overlay integration ───────────────────────────────────────────────

  function syncOutageOverlays(container, stateMap, config) {
    const ovl = window.CupolaOverlays;
    if (!ovl) return;

    const widgetId = container.dataset.widgetId;
    if (!widgetId) return;

    // prev: id → change-detection key (string) for already-registered overlays.
    const prev = container._outageOverlayMap || new Map();

    // Build the desired set. Empty when no map widget is present.
    const next = new Map(); // id → { overlay, changeKey }
    if (ovl.hasMap() && config?.showMunicipal !== false) {
      const alerts = stateMap?.['municipal.alerts']?.alerts || [];
      for (const a of alerts) {
        if (!a.polygon?.length) continue;
        const color = SEVERITY_COLOR[a.severity]?.text || '#74b9ff';
        const emoji = ALERT_TYPE_EMOJI[a.alert_type] || null;
        const overlayId = `${widgetId}:${a.id}`;
        const changeKey = `${color}|${emoji || ''}|${a.title}|${a.description || ''}|${JSON.stringify(a.polygon)}`;
        next.set(overlayId, {
          overlay: {
            type: 'polygon',
            color,
            emoji,
            coordinates: a.polygon,
            label: a.title,
            description: a.description || '',
            detail_domain: 'municipal.alerts',
            detail_id: a.id,
          },
          changeKey,
        });
      }
    }

    // Unregister overlays that are no longer present.
    for (const id of prev.keys()) {
      if (!next.has(id)) ovl.unregister(id);
    }
    // Register new overlays or re-register changed ones.
    for (const [id, { overlay, changeKey }] of next) {
      if (prev.get(id) !== changeKey) ovl.register(id, overlay);
    }

    container._outageOverlayMap = new Map([...next].map(([id, { changeKey }]) => [id, changeKey]));
  }

  function setupMapAvailCb(container, stateMap, config) {
    const ovl = window.CupolaOverlays;
    if (!ovl) return;

    if (container._mapAvailCb) ovl.offMapAvail(container._mapAvailCb);

    const cb = () => syncOutageOverlays(container, stateMap, config);
    container._mapAvailCb = cb;
    ovl.onMapAvail(cb);
  }

  function render(container, stateMap, config) {
    setupMapAvailCb(container, stateMap || {}, config || {});
    syncOutageOverlays(container, stateMap || {}, config || {});

    if (!stateMap || Object.keys(stateMap).length === 0) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">Source unavailable</span><span style="font-size:10px;opacity:.5">alerts</span></div>`;
      return;
    }

    const items = collect(stateMap, config || {});

    if (items.length === 0) {
      container.innerHTML = `<div class="widget-alerts"><p class="alerts-empty">No active alerts</p></div>`;
      return;
    }

    const cards = items.map(a => {
      const sev = SEVERITY_COLOR[a.severity] || SEVERITY_COLOR.info;
      const srcLabel = esc(SOURCE_LABEL[a._domain] || a._domain);
      const desc = a.description || a.summary || '';
      const timeStr = fmtTime(a.published_at || a.onset);
      const endsStr = (a.expires || a.ends_at) ? ` — ends ${fmtTime(a.expires || a.ends_at)}` : '';
      const area = a.area ? `<span class="alert-area">${esc(a.area)}</span>` : '';
      return `
        <div class="alert-card detail-clickable" style="border-left-color:${sev.text}"
             data-detail-domain="${escAttr(a._domain)}" data-detail-id="${escAttr(a.id)}"
             role="button" tabindex="0">
          <div class="alert-card-top">
            <span class="alert-sev-badge" style="background:${sev.bg};color:${sev.text}">${esc(sev.label)}</span>
            <span class="alert-source">${srcLabel}</span>
            <span class="alert-time">${esc(timeStr)}${esc(endsStr)}</span>
          </div>
          <div class="alert-title">${esc(a.title)}</div>
          ${area}
          ${desc ? `<div class="alert-desc">${esc(desc)}</div>` : ''}
        </div>
      `;
    }).join('');

    container.innerHTML = `<div class="widget-alerts">${cards}</div>`;
  }

  window.CupolaWidgets.push({
    type: 'alerts',
    domains: ['weather.alerts', 'transit.alerts', 'municipal.alerts'],
    defaultSize: { w: 7, h: 5 },
    configSchema: [
      { key: 'showWeather',   label: 'Weather alerts',   type: 'boolean', default: true },
      { key: 'showTransit',   label: 'Transit alerts',   type: 'boolean', default: true },
      { key: 'showMunicipal', label: 'Municipal alerts', type: 'boolean', default: true },
    ],
    subscriptionParams: () => null,
    render(container, stateMap, config)   { render(container, stateMap, config); },
    onUpdate(container, stateMap, config) { render(container, stateMap, config); },
    onRemove(container) {
      const ovl = window.CupolaOverlays;
      if (!ovl) return;
      const prev = container._outageOverlayMap || new Map();
      for (const id of prev.keys()) ovl.unregister(id);
      if (container._mapAvailCb) ovl.offMapAvail(container._mapAvailCb);
    },
  });
})();
