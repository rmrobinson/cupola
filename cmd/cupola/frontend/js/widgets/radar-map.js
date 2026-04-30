(function () {
  'use strict';

  window.CupolaWidgets = window.CupolaWidgets || [];

  const INC_COLORS = {
    major:    '#e74c3c',
    moderate: '#f39c12',
    minor:    '#95a5a6',
  };

  const VEHICLE_EMOJI = { bus: '🚌', lrt: '🚊', train: '🚆', metro: '🚇' };

  const _iconCache = new Map();
  function emojiIcon(emoji) {
    if (!_iconCache.has(emoji)) {
      _iconCache.set(emoji, L.divIcon({ html: emoji, className: 'map-emoji-icon', iconSize: [22, 22], iconAnchor: [11, 11], popupAnchor: [0, -11] }));
    }
    return _iconCache.get(emoji);
  }

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function initMap(container, config) {
    // Prefer explicitly-configured centre, then fall back to the home location
    // exposed by the server via window.CupolaConfig.  Use ?? (not ||) so that a
    // configured value of 0 is honoured rather than treated as falsy.
    const lat   = config?.center_lat ?? window.CupolaConfig?.lat  ?? 43.45;
    const lon   = config?.center_lon ?? window.CupolaConfig?.lon  ?? -80.49;
    const zoom  = config?.zoom       ?? 12;
    const theme = config?.theme      ?? 'grayscale';

    const mapDiv = document.createElement('div');
    mapDiv.style.cssText = 'width:100%;height:100%';
    container.appendChild(mapDiv);

    const map = L.map(mapDiv, { zoomControl: true, attributionControl: false }).setView([lat, lon], zoom);

    const baseLayer = protomapsL.leafletLayer({ url: '/tiles/local.pmtiles', flavor: theme });
    baseLayer.addTo(map);

    const incidentLayer = L.layerGroup().addTo(map);
    const vehicleLayer  = L.layerGroup().addTo(map);
    const aircraftLayer = L.layerGroup().addTo(map);

    // ResizeObserver handles two problems in one:
    // 1. Leaflet is initialised before the cell is appended to the DOM grid, so
    //    the container has zero size at L.map() time — tiles never load.
    // 2. When the user drags the resize handle, CSS grid dimensions change but
    //    Leaflet doesn't detect that without an explicit invalidateSize() call.
    const ro = new ResizeObserver(() => map.invalidateSize());
    ro.observe(mapDiv);

    return { map, ro, incidentLayer, vehicleLayer, aircraftLayer };
  }

  function updateIncidents(layerGroup, incidents, config) {
    layerGroup.clearLayers();
    if (!incidents?.length || config?.layer_incidents === false) return;
    for (const inc of incidents) {
      if (!inc.lat || !inc.lon) continue;
      const popup = `<b>${esc(inc.type)}</b><br>${esc(inc.road_name)}<br>${esc(inc.description)}<br><i>${esc(inc.severity)}</i>`;
      let marker;
      if (inc.type === 'construction') {
        marker = L.marker([inc.lat, inc.lon], { icon: emojiIcon('🚧') });
      } else if (inc.type === 'collision') {
        marker = L.marker([inc.lat, inc.lon], { icon: emojiIcon('🚗') });
      } else {
        const color = INC_COLORS[inc.severity] || '#95a5a6';
        marker = L.circleMarker([inc.lat, inc.lon], {
          radius: 8, fillColor: color, color: '#fff', weight: 1.5, fillOpacity: 0.9,
        });
      }
      marker.bindPopup(popup).addTo(layerGroup);
    }
  }

  function updateVehicles(layerGroup, vehicles, config) {
    layerGroup.clearLayers();
    if (!vehicles?.length || config?.layer_transit === false) return;
    for (const v of vehicles) {
      if (!v.lat || !v.lon) continue;
      const emoji = VEHICLE_EMOJI[v.vehicle_type] || '🚌';
      L.marker([v.lat, v.lon], { icon: emojiIcon(emoji) }).bindPopup(
        `<b>Route ${esc(v.route_name)}</b><br>Vehicle ${esc(v.vehicle_id)}<br>${esc(v.agency_id)}`
      ).addTo(layerGroup);
    }
  }

  // ── Aircraft SVG icons ──────────────────────────────────────────────────────
  // SVG paths and dimensions taken verbatim from tar1090 markers.js.
  // cx/cy = centre of viewBox for the SVG-native rotate(angle, cx, cy) transform.
  const _AC_SHAPES = {
    airliner: {
      viewBox: '-1 -2 34 34', sw: 1.2, cx: 16, cy: 15,
      path: 'M16 1c-.17 0-.67.58-.9 1.03-.6 1.21-.6 1.15-.65 5.2-.04 2.97-.08 3.77-.18 3.9-.15.17-1.82 1.1-1.98 1.1-.08 0-.1-.25-.05-.83.03-.5.01-.92-.05-1.08-.1-.25-.13-.26-.71-.26-.82 0-.86.07-.78 1.5.03.6.08 1.17.11 1.25.05.12-.02.2-.25.33l-8 4.2c-.2.2-.18.1-.19 1.29 3.9-1.2 3.71-1.21 3.93-1.21.06 0 .1 0 .13.14.08.3.28.3.28-.04 0-.25.03-.27 1.16-.6.65-.2 1.22-.35 1.28-.35.05 0 .12.04.15.17.07.3.27.27.27-.08 0-.25.01-.27.7-.47.68-.1.98-.09 1.47-.1.18 0 .22 0 .26.18.06.34.22.35.27-.01.04-.2.1-.17 1.06-.14l1.07.02.05 4.2c.05 3.84.07 4.28.26 5.09.11.49.2.99.2 1.11 0 .19-.31.43-1.93 1.5l-1.93 1.26v1.02l4.13-.95.63 1.54c.05.07.12.09.19.09s.14-.02.19-.09l.63-1.54 4.13.95V29.3l-1.93-1.27c-1.62-1.06-1.93-1.3-1.93-1.49 0-.12.09-.62.2-1.11.19-.81.2-1.25.26-5.09l.05-4.2 1.07-.02c.96-.03 1.02-.05 1.06.14.05.36.21.35.27 0 .04-.17.08-.16.26-.16.49 0 .8-.02 1.48.1.68.2.69.21.69.46 0 .35.2.38.27.08.03-.13.1-.17.15-.17.06 0 .63.15 1.28.34 1.13.34 1.16.36 1.16.61 0 .35.2.34.28.04.03-.13.07-.14.13-.14.22 0 .03 0 3.93 1.2-.01-1.18.02-1.07-.19-1.27l-8-4.21c-.23-.12-.3-.21-.25-.33.03-.08.08-.65.11-1.25.08-1.43.04-1.5-.78-1.5-.58 0-.61.01-.71.26-.06.16-.08.58-.05 1.08.04.58.03.83-.05.83-.16 0-1.83-.93-1.98-1.1-.1-.13-.14-.93-.18-3.9-.05-4.05-.05-3.99-.65-5.2C16.67 1.58 16.17 1 16 1z',
    },
    cessna: {
      viewBox: '0 -1 32 31', sw: 1.2, cx: 16, cy: 14.5,
      path: 'M16.36 20.96l2.57.27s.44.05.4.54l-.02.63s-.03.47-.45.54l-2.31.34-.44-.74-.22 1.63-.25-1.62-.38.73-2.35-.35s-.44-.1-.43-.6l-.02-.6s0-.5.48-.5l2.5-.27-.56-5.4-3.64-.1-5.83-1.02h-.45v-2.06s-.07-.37.46-.34l5.8-.17 3.55.12s-.1-2.52.52-2.82l-1.68-.04s-.1-.06 0-.14l1.94-.03s.35-1.18.7 0l1.91.04s.11.05 0 .14l-1.7.02s.62-.09.56 2.82l3.54-.1 5.81.17s.51-.04.48.35l-.01 2.06h-.47l-5.8 1-3.67.11z',
    },
    jet_swept: {
      viewBox: '-1 -1 20 26', sw: 1.2, cx: 9, cy: 12,
      path: 'M9.44,23c-.1.6-.35.6-.44.6s-.34,0-.44-.6l-3,.67V22.6A.54.54,0,0,1,6,22.05l2.38-1.12L8,19.33H6.69l0-.2a8.23,8.23,0,0,1-.14-3.85l.06-.18H7.73V13.19h-2L.26,14.29v-.93c0-.28.07-.46.22-.53l7.25-3.6V3.85A4.47,4.47,0,0,1,8.83.49L9,.34l.17.15a4.47,4.47,0,0,1,1.1,3.36V9.23l7.25,3.6c.14.07.22.25.22.53v.93l-5.51-1.1h-2V15.1h1.17l.06.18a8.24,8.24,0,0,1-.15,3.84l0,.2H10l-.36,1.6,2.43,1.14a.52.52,0,0,1,.35.53v1.08Z',
    },
    heavy_2e: {
      viewBox: '0 -3.2 64.2 64.2', sw: 1.8, cx: 32.1, cy: 28.9,
      path: 'm 31.414,2.728 c -0.314,0.712 -1.296,2.377 -1.534,6.133 l -0.086,13.379 c 0.006,0.400 -0.380,0.888 -0.945,1.252 l -2.631,1.729 c 0.157,-0.904 0.237,-3.403 -0.162,-3.850 l -2.686,0.006 c -0.336,1.065 -0.358,2.518 -0.109,4.088 h 0.434 L 24.057,26.689 8.611,36.852 7.418,38.432 7.381,39.027 8.875,38.166 l 8.295,-2.771 0.072,0.730 0.156,-0.004 0.150,-0.859 3.799,-1.234 0.074,0.727 0.119,0.004 0.117,-0.832 2.182,-0.730 h 1.670 l 0.061,0.822 h 0.176 l 0.062,-0.822 4.018,-0.002 v 13.602 c 0.051,1.559 0.465,3.272 0.826,4.963 l -6.836,5.426 c -0.097,0.802 -0.003,1.372 0.049,1.885 l 7.734,-2.795 0.477,1.973 h 0.232 l 0.477,-1.973 7.736,2.795 c 0.052,-0.513 0.146,-1.083 0.049,-1.885 l -6.836,-5.426 c 0.361,-1.691 0.775,-3.404 0.826,-4.963 V 33.193 l 4.016,0.002 0.062,0.822 h 0.178 L 38.875,33.195 h 1.672 l 2.182,0.730 0.117,0.832 0.119,-0.004 0.072,-0.727 3.799,1.234 0.152,0.859 0.154,0.004 0.072,-0.730 8.297,2.771 1.492,0.861 -0.037,-0.596 -1.191,-1.580 -15.447,-10.162 0.363,-1.225 H 41.125 c 0.248,-1.569 0.225,-3.023 -0.111,-4.088 l -2.686,-0.006 c -0.399,0.447 -0.317,2.945 -0.160,3.850 L 35.535,23.492 C 34.970,23.128 34.584,22.640 34.590,22.240 L 34.504,8.910 C 34.193,4.926 33.369,3.602 32.934,2.722 32.442,1.732 31.894,1.828 31.414,2.728 Z',
    },
    helicopter: {
      viewBox: '-13 -13 90 90', sw: 3.0, cx: 32, cy: 32,
      path: 'm 24.698,60.712 c 0,0 -0.450,2.134 -0.861,2.142 -0.561,0.011 -0.480,-3.836 -0.593,-5.761 -0.064,-1.098 1.381,-1.192 1.481,-0.042 l 5.464,0.007 -0.068,-9.482 -0.104,-1.108 c -2.410,-2.131 -3.028,-3.449 -3.152,-7.083 l -12.460,13.179 c -0.773,0.813 -2.977,0.599 -3.483,-0.428 L 26.920,35.416 26.866,29.159 11.471,14.513 c -0.813,-0.773 -0.599,-2.977 0.428,-3.483 l 14.971,14.428 0.150,-5.614 c -0.042,-1.324 1.075,-4.784 3.391,-5.633 0.686,-0.251 2.131,-0.293 3.033,0.008 2.349,0.783 3.433,4.309 3.391,5.633 l 0.073,4.400 12.573,-12.763 c 0.779,-0.807 2.977,-0.599 3.483,0.428 L 37.054,28.325 37.027,35.027 52.411,49.365 c 0.813,0.773 0.599,2.977 -0.428,3.483 L 36.992,38.359 c -0.124,3.634 -0.742,5.987 -3.152,8.118 l -0.104,1.108 -0.068,9.482 5.321,-0.068 c 0.101,-1.150 1.546,-1.057 1.481,0.042 -0.113,1.925 -0.032,5.772 -0.593,5.761 -0.412,-0.008 -0.861,-2.142 -0.861,-2.142 l -5.387,-0.011 0.085,9.377 -1.094,2.059 -1.386,-0.018 -1.093,-2.049 0.085,-9.377 z',
    },
  };

  const _AC_CATEGORY_SHAPE = {
    A1: 'cessna', A2: 'jet_swept',
    A3: 'airliner', A4: 'airliner', A5: 'heavy_2e',
    A6: 'jet_swept', A7: 'helicopter',
  };

  // Altitude → colour using the exact tar1090 ColorByAlt table (from defaults.js).
  const _ALT_H = [
    { alt: 0, val: 20 }, { alt: 2000, val: 32.5 }, { alt: 4000, val: 43 },
    { alt: 6000, val: 54 }, { alt: 8000, val: 72 }, { alt: 9000, val: 85 },
    { alt: 11000, val: 140 }, { alt: 40000, val: 300 }, { alt: 51000, val: 360 },
  ];
  const _ALT_L = [
    { h: 0, val: 53 }, { h: 20, val: 50 }, { h: 32, val: 54 }, { h: 40, val: 52 },
    { h: 46, val: 51 }, { h: 50, val: 46 }, { h: 60, val: 43 }, { h: 80, val: 41 },
    { h: 100, val: 41 }, { h: 120, val: 41 }, { h: 140, val: 41 }, { h: 160, val: 40 },
    { h: 180, val: 40 }, { h: 190, val: 44 }, { h: 198, val: 50 }, { h: 200, val: 58 },
    { h: 220, val: 58 }, { h: 240, val: 58 }, { h: 255, val: 55 }, { h: 266, val: 55 },
    { h: 270, val: 58 }, { h: 280, val: 58 }, { h: 290, val: 47 }, { h: 300, val: 43 },
    { h: 310, val: 48 }, { h: 320, val: 48 }, { h: 340, val: 52 }, { h: 360, val: 53 },
  ];

  function _interp(pts, key, val) {
    for (let i = pts.length - 1; i >= 0; i--) {
      if (val > pts[i][key]) {
        if (i === pts.length - 1) return pts[i].val;
        return pts[i].val + (pts[i + 1].val - pts[i].val) * (val - pts[i][key]) / (pts[i + 1][key] - pts[i][key]);
      }
    }
    return pts[0].val;
  }

  function altToColor(altFt, onGround) {
    if (onGround) return 'hsl(220,0%,30%)';
    if (altFt == null) return 'hsl(0,0%,20%)';
    const h = _interp(_ALT_H, 'alt', altFt);
    const l = _interp(_ALT_L, 'h', h);
    return `hsl(${h.toFixed(0)},88%,${l.toFixed(0)}%)`;
  }

  const _AC_SIZE = 32;

  function aircraftIcon(a) {
    const shape  = _AC_SHAPES[_AC_CATEGORY_SHAPE[a.category] || 'airliner'];
    const angle  = a.track ?? 0;
    const color  = altToColor(a.alt_ft, a.on_ground);
    const svg = [
      `<svg xmlns="http://www.w3.org/2000/svg" width="${_AC_SIZE}" height="${_AC_SIZE}" viewBox="${shape.viewBox}">`,
      `<g transform="rotate(${angle} ${shape.cx} ${shape.cy})">`,
      `<path d="${shape.path}" fill="${color}" stroke="#1a2530" stroke-width="${shape.sw}" stroke-linejoin="round"/>`,
      `</g></svg>`,
    ].join('');
    return L.divIcon({
      html: svg,
      className: 'aircraft-svg-icon',
      iconSize:   [_AC_SIZE, _AC_SIZE],
      iconAnchor: [_AC_SIZE / 2, _AC_SIZE / 2],
      popupAnchor: [0, -_AC_SIZE / 2],
    });
  }

  function updateAircraft(layerGroup, aircraft, config) {
    layerGroup.clearLayers();
    if (!aircraft?.length || config?.layer_aircraft === false) return;
    for (const a of aircraft) {
      if (!a.lat || !a.lon) continue;
      const label = a.flight || a.callsign || a.icao;
      const alt   = a.alt_ft    ? ` · ${Math.round(a.alt_ft / 100) * 100}ft` : '';
      const spd   = a.speed_kts ? ` · ${Math.round(a.speed_kts)}kts`         : '';
      L.marker([a.lat, a.lon], { icon: aircraftIcon(a) })
        .bindPopup(`<b>${esc(label)}</b>${alt}${spd}`)
        .addTo(layerGroup);
    }
  }

  function render(container, stateMap, config) {
    if (typeof L === 'undefined' || typeof protomapsL === 'undefined') {
      container.innerHTML = '<div class="widget-unavailable"><span class="widget-unavailable-label">Map libraries not loaded</span></div>';
      return;
    }
    if (container._radarMap) {
      container._radarMap.ro.disconnect();
      container._radarMap.map.remove();
      container._radarMap = null;
    }
    container.innerHTML = '';

    const inst = initMap(container, config);
    container._radarMap = inst;

    const sm = stateMap || {};
    updateIncidents(inst.incidentLayer, sm['traffic.incidents']?.incidents, config);
    updateVehicles(inst.vehicleLayer,   sm['transit.vehicles']?.vehicles,   config);
    updateAircraft(inst.aircraftLayer,  sm['aircraft']?.aircraft,           config);
  }

  function onUpdate(container, stateMap, config) {
    if (!container._radarMap) { render(container, stateMap, config); return; }
    const inst = container._radarMap;
    const sm = stateMap || {};
    updateIncidents(inst.incidentLayer, sm['traffic.incidents']?.incidents, config);
    updateVehicles(inst.vehicleLayer,   sm['transit.vehicles']?.vehicles,   config);
    updateAircraft(inst.aircraftLayer,  sm['aircraft']?.aircraft,           config);
  }

  window.CupolaWidgets.push({
    type:    'radar-map',
    domains: ['traffic.incidents', 'transit.vehicles', 'aircraft'],
    defaultSize: { w: 6, h: 8 },
    configSchema: [
      { key: 'center_lat',      label: 'Center latitude',  type: 'number', placeholder: () => window.CupolaConfig?.lat ?? '' },
      { key: 'center_lon',      label: 'Center longitude', type: 'number', placeholder: () => window.CupolaConfig?.lon ?? '' },
      { key: 'zoom',            label: 'Zoom level',       type: 'number',  default: 12 },
      { key: 'theme',           label: 'Map style',        type: 'select',  default: 'grayscale',
        options: [
          { value: 'grayscale', label: 'Grayscale' },
          { value: 'light',     label: 'Light' },
          { value: 'dark',      label: 'Dark' },
          { value: 'black',     label: 'Black' },
          { value: 'white',     label: 'White' },
        ] },
      { key: 'layer_incidents', label: 'Show incidents',   type: 'boolean', default: true },
      { key: 'layer_transit',   label: 'Show transit',     type: 'boolean', default: true },
      { key: 'layer_aircraft',  label: 'Show aircraft',    type: 'boolean', default: true },
    ],
    subscriptionParams: () => null,
    render,
    onUpdate,
  });
})();
