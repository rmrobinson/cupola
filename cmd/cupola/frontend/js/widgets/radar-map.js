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

  function updateAircraft(layerGroup, aircraft, config) {
    layerGroup.clearLayers();
    if (!aircraft?.length || config?.layer_aircraft === false) return;
    for (const a of aircraft) {
      if (!a.lat || !a.lon) continue;
      const label = a.callsign || a.flight || a.icao;
      const alt   = a.alt_ft    ? ` · ${Math.round(a.alt_ft / 100) * 100}ft` : '';
      const spd   = a.speed_kts ? ` · ${Math.round(a.speed_kts)}kts`         : '';
      L.circleMarker([a.lat, a.lon], {
        radius: 5, fillColor: '#2ecc71', color: '#fff', weight: 1.5, fillOpacity: 0.9,
      }).bindPopup(`<b>${esc(label)}</b>${alt}${spd}`).addTo(layerGroup);
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
