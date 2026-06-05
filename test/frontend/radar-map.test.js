const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadWidget() {
  const markers = [];
  const polylines = [];
  const views = [];

  function layerGroup() {
    return {
      cleared: 0,
      addTo() { return this; },
      clearLayers() { this.cleared += 1; },
    };
  }

  const context = {
    window: {
      CupolaWidgets: [],
      CupolaConfig: { lat: 43.45, lon: -80.49 },
      CupolaOverlays: {
        subscribeMap(cb) { cb([]); return () => {}; },
        unsubscribeMap() {},
      },
    },
    document: {
      createElement() {
        return { style: {}, children: [], appendChild(child) { this.children.push(child); } };
      },
    },
    ResizeObserver: class {
      observe() {}
      disconnect() {}
    },
    L: {
      divIcon(opts) { return { opts }; },
      map() {
        return {
          setView(center, zoom) { views.push({ center, zoom }); return this; },
          invalidateSize() {},
          remove() {},
        };
      },
      layerGroup,
      marker(latLng, opts) {
        const marker = {
          latLng,
          opts,
          popup: '',
          bindPopup(html) { this.popup = html; return this; },
          addTo(layer) { markers.push(this); this.layer = layer; return this; },
        };
        return marker;
      },
      polyline(latLngs, opts) {
        const line = {
          latLngs,
          opts,
          popup: '',
          bindPopup(html) { this.popup = html; return this; },
          addTo(layer) { polylines.push(this); this.layer = layer; return this; },
        };
        return line;
      },
      polygon() { return { bindPopup() { return this; }, addTo() { return this; } }; },
      circleMarker() { return { bindPopup() { return this; }, addTo() { return this; } }; },
    },
    protomapsL: {
      leafletLayer() { return { addTo() {} }; },
    },
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/radar-map.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return { widget: context.window.CupolaWidgets[0], markers, polylines, views };
}

test('radar map config includes home marker toggle', () => {
  const { widget } = loadWidget();
  const homeToggle = widget.configSchema.find(field => field.key === 'show_home_marker');

  assert.equal(homeToggle.type, 'boolean');
  assert.equal(homeToggle.default, false);
});

test('radar map shows home marker at configured centre when enabled', () => {
  const { widget, markers, views } = loadWidget();
  const container = { innerHTML: '', appendChild() {} };

  widget.render(container, {}, {
    center_lat: 43.471,
    center_lon: -80.542,
    zoom: 13,
    show_home_marker: true,
  });

  assert.equal(views[0].center[0], 43.471);
  assert.equal(views[0].center[1], -80.542);
  assert.equal(views[0].zoom, 13);
  assert.equal(markers.length, 1);
  assert.equal(markers[0].latLng[0], 43.471);
  assert.equal(markers[0].latLng[1], -80.542);
  assert.equal(markers[0].popup, 'You are here');
  assert.match(markers[0].opts.icon.opts.html, /map-home-marker-dot/);
});

test('radar map does not show home marker by default', () => {
  const { widget, markers } = loadWidget();
  const container = { innerHTML: '', appendChild() {} };

  widget.render(container, {}, {});

  assert.equal(markers.length, 0);
});

test('radar map renders traffic incident line geometry as polylines', () => {
  const { widget, markers, polylines } = loadWidget();
  const container = { innerHTML: '', appendChild() {} };

  widget.render(container, {
    'traffic.incidents': {
      incidents: [{
        id: 'region-waterloo-roadclosures:test',
        type: 'closure',
        road_name: 'King St',
        description: 'Status: Lane Reduced',
        severity: 'moderate',
        lines: [[[-80.5, 43.4], [-80.6, 43.5]]],
      }],
    },
  }, {});

  assert.equal(markers.length, 0);
  assert.equal(polylines.length, 1);
  assert.equal(JSON.stringify(polylines[0].latLngs), JSON.stringify([[43.4, -80.5], [43.5, -80.6]]));
  assert.equal(polylines[0].opts.color, '#f39c12');
  assert.match(polylines[0].popup, /traffic\.incidents/);
});
