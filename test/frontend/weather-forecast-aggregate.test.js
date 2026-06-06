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
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/weather-forecast-aggregate.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

function forecastState(overrides = {}) {
  return {
    periods: [
      { label: 'Today', starts_at: '2026-06-06T12:00:00Z', condition: 'Sunny', high: 25, low: 14, precip_chance: 10 },
      { label: 'Tonight', starts_at: '2026-06-07T00:00:00Z', condition: 'Clear', low: 12, precip_chance: 0 },
    ],
    ...overrides,
  };
}

function aqhiState(overrides = {}) {
  return {
    location: 'Kitchener',
    forecasts: [
      { label: 'Today', max: { value: 3, risk: 'Low Risk' } },
      { label: 'Tomorrow', max: { value: 5, risk: 'Moderate Risk' } },
    ],
    ...overrides,
  };
}

function solarForecastState(overrides = {}) {
  return {
    periods: [
      {
        starts_at: '2999-06-05T15:00:00Z',
        kp_expected: 2.5,
        kp_description: 'Quiet',
        aurora_viewable: false,
      },
      {
        starts_at: '2999-06-05T18:00:00Z',
        kp_expected: 5.2,
        kp_description: 'Minor storm',
        aurora_viewable: true,
      },
    ],
    ...overrides,
  };
}

test('weather forecast aggregate registers for forecast weather, AQHI, and solar domains', () => {
  const widget = loadWidget();

  assert.equal(widget.type, 'weather-forecast-aggregate');
  assert.deepEqual(Array.from(widget.domains), ['weather.forecast', 'weather.air_quality', 'solar.weather.forecast', 'weather.pollen']);
  assert.equal(widget.defaultSize.w, 8);
  assert.equal(widget.defaultSize.h, 7);
});

test('weather forecast aggregate renders all three forecast sources', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.forecast': forecastState(),
    'weather.air_quality': aqhiState(),
    'solar.weather.forecast': solarForecastState(),
  });

  assert.match(container.innerHTML, /widget-weather-forecast-aggregate/);
  assert.match(container.innerHTML, /Today/);
  assert.match(container.innerHTML, /Sunny/);
  assert.match(container.innerHTML, /H:25&deg;/);
  assert.match(container.innerHTML, /wfa-inline-aqhi/);
  assert.match(container.innerHTML, /AQHI 3/);
  assert.match(container.innerHTML, /Kitchener/);
  assert.match(container.innerHTML, /Moderate Risk/);
  assert.match(container.innerHTML, /Solar forecast/);
  assert.match(container.innerHTML, /Minor storm/);
  assert.match(container.innerHTML, /Aurora/);
});

test('weather forecast aggregate reports matching AQHI alongside weather rows', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.forecast': forecastState({
      periods: [
        { label: 'Friday night', condition: 'Cloudy', low: 9, precip_chance: 30 },
      ],
    }),
    'weather.air_quality': aqhiState({
      forecasts: [
        { label: 'Friday night', max: { value: 6, risk: 'Moderate Risk' } },
        { label: 'Saturday', max: { value: 2, risk: 'Low Risk' } },
      ],
    }),
  });

  assert.match(container.innerHTML, /Friday night/);
  assert.match(container.innerHTML, /Cloudy/);
  assert.match(container.innerHTML, /AQHI 6/);
  assert.doesNotMatch(container.innerHTML, /wfa-inline-risk/);
  assert.match(container.innerHTML, /Saturday/);
  assert.match(container.innerHTML, /Low Risk/);
});

test('weather forecast aggregate renders without AQHI', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.forecast': forecastState({ periods: [{ label: 'Friday', condition: 'Rain', precip_chance: 80 }] }),
    'solar.weather.forecast': solarForecastState(),
  });

  assert.match(container.innerHTML, /Friday/);
  assert.match(container.innerHTML, /Rain/);
  assert.match(container.innerHTML, /Solar forecast/);
  assert.doesNotMatch(container.innerHTML, /AQHI forecast/);
  assert.match(container.innerHTML, /wfa-inline-aqhi is-empty/);
});

test('weather forecast aggregate renders without solar', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.forecast': forecastState(),
    'weather.air_quality': aqhiState(),
  });

  assert.match(container.innerHTML, /Sunny/);
  assert.match(container.innerHTML, /Kitchener/);
  assert.match(container.innerHTML, /AQHI 3/);
  assert.doesNotMatch(container.innerHTML, /Solar forecast/);
});

test('weather forecast aggregate renders useful output with only AQHI and solar', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.air_quality': aqhiState({ location: 'Waterloo' }),
    'solar.weather.forecast': solarForecastState(),
  });

  assert.match(container.innerHTML, /Waterloo/);
  assert.match(container.innerHTML, /Low Risk/);
  assert.match(container.innerHTML, /Solar forecast/);
});

test('weather forecast aggregate renders pollen metrics when present', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.forecast': forecastState({
      periods: [
        { label: 'Sunday', starts_at: '2026-06-06T12:00:00Z', condition: 'Sunny', high: 25, precip_chance: 10 },
        { label: 'Monday', starts_at: '2026-06-07T12:00:00Z', condition: 'Rain', high: 18, precip_chance: 80 },
      ],
    }),
    'weather.air_quality': aqhiState({
      forecasts: [
        { label: 'Sunday', max: { value: 3, risk: 'Low Risk' } },
        { label: 'Monday', max: { value: 5, risk: 'Moderate Risk' } },
      ],
    }),
    'weather.pollen': {
      days: [
        { date: '2026-06-07', aggregate: { value: 5, label: 'Grass', code: 'GRASS', category: 'Very high', color: '#e53935' } },
        { date: '2026-06-08', aggregate: { value: 2, label: 'Cypress pine', code: 'CYPRESS_PINE', category: 'Low', color: '#57d9a3' } },
      ],
    },
  });

  assert.doesNotMatch(container.innerHTML, /Pollen forecast/);
  assert.match(container.innerHTML, /Sunday[\s\S]*Sunny[\s\S]*AQHI 3[\s\S]*Grass 5/);
  assert.match(container.innerHTML, /Monday[\s\S]*Rain[\s\S]*AQHI 5[\s\S]*Tree 2/);
  assert.doesNotMatch(container.innerHTML, /Cypress pine 2/);
});

test('weather forecast aggregate matches pollen by forecast label before timestamp date', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.forecast': forecastState({
      periods: [
        { label: 'Saturday night', starts_at: '2026-06-07T03:00:00Z', condition: 'Clear', low: 12, precip_chance: 0 },
      ],
    }),
    'weather.pollen': {
      days: [
        { date: '2026-06-06', aggregate: { value: 1, label: 'Grass', code: 'GRASS', category: 'Very low', color: '#57d9a3' } },
        { date: '2026-06-07', aggregate: { value: 5, label: 'Tree', code: 'TREE', category: 'Very high', color: '#e53935' } },
      ],
    },
  });

  assert.match(container.innerHTML, /Saturday night[\s\S]*Clear[\s\S]*Grass 1/);
  assert.doesNotMatch(container.innerHTML, /Tree 5/);
});

test('weather forecast aggregate renders empty state when all sources are missing', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {});

  assert.match(container.innerHTML, /No forecast data/);
  assert.match(container.innerHTML, /weather aggregate/);
});

test('weather forecast aggregate escapes source text', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.forecast': forecastState({
      periods: [{ label: '<Today>', condition: '<script>alert(1)</script>', summary: '', precip_chance: 20 }],
    }),
    'weather.air_quality': aqhiState({
      location: '<Kitchener>',
      forecasts: [{ label: '<Tonight>', max: { value: 2, risk: '<b>Low</b>' } }],
    }),
    'solar.weather.forecast': solarForecastState({
      periods: [{ starts_at: '2999-06-05T15:00:00Z', kp_expected: 1, kp_description: '<quiet>', aurora_viewable: false }],
    }),
  });

  assert.doesNotMatch(container.innerHTML, /<script>/);
  assert.match(container.innerHTML, /&lt;Today&gt;/);
  assert.match(container.innerHTML, /&lt;script&gt;/);
  assert.match(container.innerHTML, /&lt;Kitchener&gt;/);
  assert.match(container.innerHTML, /&lt;Tonight&gt;/);
  assert.match(container.innerHTML, /&lt;b&gt;Low&lt;\/b&gt;/);
  assert.match(container.innerHTML, /&lt;quiet&gt;/);
});
