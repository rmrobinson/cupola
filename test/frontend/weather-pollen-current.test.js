const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadWidget() {
  const context = {
    window: { CupolaWidgets: [] },
    Date,
    Number,
    Math,
    String,
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/weather-pollen-current.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

function pollenState(overrides = {}) {
  return {
    current: {
      date: '2026-06-06',
      aggregate: { value: 5, label: 'Grass', code: 'GRASS', category: 'Very high', color: '#e53935' },
      types: [
        { code: 'GRASS', display_name: 'Grass', in_season: true, upi: { value: 5, category: 'Very high', color: '#e53935' } },
        { code: 'TREE', display_name: 'Tree', in_season: false, upi: { value: 2, category: 'Low', color: '#57d9a3' } },
        { code: 'WEED', display_name: 'Weed', in_season: true },
      ],
      plants: [
        { code: 'RAGWEED', display_name: 'Ragweed', in_season: true, upi: { value: 4, category: 'High', color: '#f7b733' } },
      ],
      health_recommendations: ['Close windows', 'Shower after being outside'],
    },
    ...overrides,
  };
}

test('weather pollen current registers weather.pollen domain', () => {
  const widget = loadWidget();

  assert.equal(widget.type, 'weather-pollen-current');
  assert.equal(widget.domain, 'weather.pollen');
  assert.equal(widget.defaultSize.w, 4);
  assert.equal(widget.defaultSize.h, 4);
});

test('weather pollen current renders empty state without current data', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, { days: [] });

  assert.match(container.innerHTML, /No current pollen data/);
  assert.match(container.innerHTML, /weather\.pollen/);
});

test('weather pollen current renders max UPI, type rows, and plant rows without commentary', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, pollenState());

  assert.match(container.innerHTML, /widget-weather-pollen-current/);
  assert.match(container.innerHTML, /2026-06-06/);
  assert.match(container.innerHTML, /pollen-upi/);
  assert.match(container.innerHTML, />5<\/div>/);
  assert.match(container.innerHTML, /Very high/);
  assert.match(container.innerHTML, /Grass/);
  assert.match(container.innerHTML, /Tree/);
  assert.match(container.innerHTML, /Weed/);
  assert.match(container.innerHTML, /Ragweed/);
  assert.doesNotMatch(container.innerHTML, /Close windows/);
  assert.doesNotMatch(container.innerHTML, /Shower after being outside/);
});

test('weather pollen current escapes text', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, pollenState({
    current: {
      date: '<today>',
      aggregate: { value: 5, label: '<Grass>', category: '<Very high>', color: '#e53935' },
      types: [{ code: 'GRASS', display_name: '<script>', in_season: true, upi: { value: 5, category: '<b>High</b>' } }],
      plants: [{ code: 'RAGWEED', display_name: '<Ragweed>', in_season: true }],
      health_recommendations: ['<close windows>'],
    },
  }));

  assert.doesNotMatch(container.innerHTML, /<script>/);
  assert.match(container.innerHTML, /&lt;today&gt;/);
  assert.match(container.innerHTML, /&lt;Grass&gt;/);
  assert.match(container.innerHTML, /&lt;Very high&gt;/);
  assert.match(container.innerHTML, /&lt;script&gt;/);
  assert.match(container.innerHTML, /&lt;Ragweed&gt;/);
  assert.doesNotMatch(container.innerHTML, /&lt;close windows&gt;/);
});
