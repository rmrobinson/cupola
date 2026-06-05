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
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/weather-air-quality.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

test('weather air quality widget registers for air quality domain', () => {
  const widget = loadWidget();
  assert.equal(widget.type, 'weather-air-quality');
  assert.equal(widget.domain, 'weather.air_quality');
  assert.equal(widget.defaultSize.w, 4);
  assert.equal(widget.defaultSize.h, 4);
});

test('weather air quality widget renders current AQHI and forecasts', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    location: 'Kitchener',
    province: 'ON',
    observed: { value: 4, risk: 'Moderate Risk' },
    forecasts: [
      { label: 'Tonight', max: { value: 4, risk: 'Moderate Risk' } },
      { label: 'Friday', max: { value: 5, risk: 'Moderate Risk' } },
      { label: 'Saturday', max: { value: 3, risk: 'Low Risk' } },
    ],
    issued_at: '2026-06-04T21:00:00Z',
  });

  assert.match(container.innerHTML, /widget-air-quality/);
  assert.match(container.innerHTML, /Kitchener/);
  assert.match(container.innerHTML, /ON/);
  assert.match(container.innerHTML, /aq-value/);
  assert.match(container.innerHTML, />4<\/div>/);
  assert.match(container.innerHTML, /Moderate Risk/);
  assert.match(container.innerHTML, /Friday/);
  assert.match(container.innerHTML, /Low Risk/);
  assert.match(container.innerHTML, /Issued/);
});

test('weather air quality widget renders empty state', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, null);

  assert.match(container.innerHTML, /No air quality data/);
  assert.match(container.innerHTML, /weather.air_quality/);
});

test('weather air quality widget escapes source text', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    location: '<script>alert(1)</script>',
    province: 'ON',
    observed: { value: 2, risk: '<b>Low</b>' },
    forecasts: [{ label: '<Tonight>', max: { value: 2, risk: 'Low Risk' } }],
  });

  assert.doesNotMatch(container.innerHTML, /<script>/);
  assert.match(container.innerHTML, /&lt;script&gt;/);
  assert.match(container.innerHTML, /&lt;b&gt;Low&lt;\/b&gt;/);
  assert.match(container.innerHTML, /&lt;Tonight&gt;/);
});
