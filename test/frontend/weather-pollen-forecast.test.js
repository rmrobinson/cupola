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
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/weather-pollen-forecast.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

test('weather pollen forecast registers weather.pollen domain', () => {
  const widget = loadWidget();

  assert.equal(widget.type, 'weather-pollen-forecast');
  assert.equal(widget.domain, 'weather.pollen');
  assert.equal(widget.defaultSize.w, 6);
  assert.equal(widget.defaultSize.h, 6);
});

test('weather pollen forecast renders forecast days and max UPI', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    days: [
      {
        date: '2026-06-06',
        aggregate: { value: 5, label: 'Grass', category: 'Very high', color: '#e53935' },
        types: [{ code: 'GRASS', display_name: 'Grass', in_season: true, upi: { value: 5, category: 'Very high', color: '#e53935' } }],
        plants: [{ code: 'RAGWEED', display_name: 'Ragweed', in_season: true, upi: { value: 4, category: 'High', color: '#f7b733' } }],
      },
      {
        date: '2026-06-07',
        aggregate: { value: 2, label: 'Tree', category: 'Low', color: '#57d9a3' },
        types: [{ code: 'TREE', display_name: 'Tree', in_season: false, upi: { value: 2, category: 'Low', color: '#57d9a3' } }],
        plants: [],
      },
    ],
  });

  assert.match(container.innerHTML, /widget-weather-pollen-forecast/);
  assert.match(container.innerHTML, /2026-06-06/);
  assert.match(container.innerHTML, /UPI 5/);
  assert.match(container.innerHTML, /Grass Very high/);
  assert.match(container.innerHTML, /Ragweed/);
  assert.match(container.innerHTML, /2026-06-07/);
  assert.match(container.innerHTML, /UPI 2/);
});

test('weather pollen forecast renders empty state and escapes text', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {});
  assert.match(container.innerHTML, /No pollen forecast/);

  widget.render(container, {
    days: [{
      date: '<today>',
      aggregate: { value: 5, label: '<Grass>', category: '<High>', color: '#e53935' },
      types: [{ code: 'GRASS', display_name: '<script>', in_season: true, upi: { value: 5, category: '<b>High</b>' } }],
      plants: [{ code: 'RAGWEED', display_name: '<Ragweed>', in_season: true }],
    }],
  });

  assert.doesNotMatch(container.innerHTML, /<script>/);
  assert.match(container.innerHTML, /&lt;today&gt;/);
  assert.match(container.innerHTML, /&lt;Grass&gt;/);
  assert.match(container.innerHTML, /&lt;High&gt;/);
  assert.match(container.innerHTML, /&lt;script&gt;/);
  assert.match(container.innerHTML, /&lt;Ragweed&gt;/);
});
