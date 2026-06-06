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
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/weather-current-aggregate.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

function weatherState(overrides = {}) {
  return {
    temperature: 21.4,
    feels_like: 22.1,
    humidity: 64,
    wind_speed: 18,
    wind_direction: 225,
    wind_gust: 34,
    pressure: 1014,
    uv: 5,
    condition: 'Partly cloudy',
    ...overrides,
  };
}

function aqhiState(overrides = {}) {
  return {
    location: 'Kitchener',
    province: 'ON',
    observed: { value: 4, risk: 'Moderate Risk' },
    ...overrides,
  };
}

function solarState(overrides = {}) {
  return {
    kp_index: 3.7,
    kp_description: 'Unsettled',
    aurora_viewable: true,
    flare_class: 'M1',
    region: 107,
    ...overrides,
  };
}

test('weather current aggregate registers for current weather, AQHI, solar, and pollen domains', () => {
  const widget = loadWidget();

  assert.equal(widget.type, 'weather-current-aggregate');
  assert.deepEqual(Array.from(widget.domains), ['weather.current', 'weather.air_quality', 'solar.weather.current', 'weather.pollen']);
  assert.equal(widget.defaultSize.w, 6);
  assert.equal(widget.defaultSize.h, 4);
});

test('weather current aggregate renders all three sources', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.current': weatherState(),
    'weather.air_quality': aqhiState(),
    'solar.weather.current': solarState(),
    'weather.pollen': {
      current: {
        aggregate: { value: 5, label: 'Grass', category: 'Very high', color: '#e53935' },
      },
    },
  });

  assert.match(container.innerHTML, /widget-weather-current-aggregate/);
  assert.match(container.innerHTML, /22&deg;C/);
  assert.match(container.innerHTML, /Partly cloudy/);
  assert.match(container.innerHTML, /Currently 21&deg;C/);
  assert.match(container.innerHTML, /Humidity 64%/);
  assert.match(container.innerHTML, /Wind 18 km\/h SW/);
  assert.match(container.innerHTML, /Air quality/);
  assert.match(container.innerHTML, /Kitchener ON/);
  assert.match(container.innerHTML, />4<\/span>/);
  assert.match(container.innerHTML, /Moderate Risk/);
  assert.doesNotMatch(container.innerHTML, /Kp index/);
  assert.match(container.innerHTML, /3\.7/);
  assert.match(container.innerHTML, /Solar weather/);
  assert.match(container.innerHTML, /Unsettled/);
  assert.match(container.innerHTML, /Pollen/);
  assert.match(container.innerHTML, /style="color:#f7b733">UV 5<\/span>\s*<span style="color:#f7b733">Moderate<\/span>/);
  assert.match(container.innerHTML, /style="color:#f7b733">Moderate Risk<\/span>/);
  assert.match(container.innerHTML, /style="color:#e53935">Very high<\/span>/);
  assert.match(container.innerHTML, /style="color:#a8ff78">Unsettled<\/span>/);
  assert.match(container.innerHTML, /Currently 21&deg;C[\s\S]*Humidity 64%[\s\S]*Wind 18 km\/h SW[\s\S]*Gust 34 km\/h[\s\S]*Pressure 1014 hPa/);
  assert.match(container.innerHTML, /Air quality[\s\S]*Kitchener ON[\s\S]*Pollen[\s\S]*Solar weather/);
});

test('weather current aggregate renders with only weather', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, { 'weather.current': weatherState({ condition: 'Sunny' }) });

  assert.match(container.innerHTML, /Sunny/);
  assert.doesNotMatch(container.innerHTML, /Air quality/);
  assert.doesNotMatch(container.innerHTML, /Solar weather/);
});

test('weather current aggregate renders with only AQHI', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, { 'weather.air_quality': aqhiState({ location: 'Waterloo' }) });

  assert.match(container.innerHTML, /no-weather/);
  assert.match(container.innerHTML, /Air quality/);
  assert.match(container.innerHTML, /Waterloo ON/);
  assert.match(container.innerHTML, /Moderate Risk/);
  assert.match(container.innerHTML, />4<\/span>/);
});

test('weather current aggregate renders with only solar', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'solar.weather.current': solarState({ kp_description: 'Quiet', aurora_viewable: false, flare_class: null }),
  });

  assert.match(container.innerHTML, /Solar weather/);
  assert.match(container.innerHTML, /Quiet/);
  assert.match(container.innerHTML, /Aurora unlikely/);
});

test('weather current aggregate renders pollen only when current pollen exists', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.pollen': {
      current: {
        aggregate: { value: 5, label: 'Grass', category: 'Very high', color: '#e53935' },
      },
    },
  });

  assert.match(container.innerHTML, /Pollen/);
  assert.match(container.innerHTML, />5<\/span>/);
  assert.match(container.innerHTML, /Very high/);
  assert.match(container.innerHTML, /Grass/);

  widget.render(container, {
    'weather.pollen': {
      days: [{ aggregate: { value: 5, label: 'Grass' } }],
    },
  });

  assert.match(container.innerHTML, /Source unavailable/);
  assert.doesNotMatch(container.innerHTML, /Pollen/);
});

test('weather current aggregate renders empty state when all sources are missing', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {});

  assert.match(container.innerHTML, /Source unavailable/);
  assert.match(container.innerHTML, /weather aggregate/);
});

test('weather current aggregate escapes source text', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    'weather.current': weatherState({ condition: '<script>alert(1)</script>' }),
  });

  assert.doesNotMatch(container.innerHTML, /<script>/);
  assert.match(container.innerHTML, /&lt;script&gt;/);

  widget.render(container, {
    'weather.air_quality': aqhiState({ location: '<Kitchener>', observed: { value: 2, risk: '<b>Low</b>' } }),
    'solar.weather.current': solarState({ kp_description: '<quiet>', flare_class: '<M1>' }),
  });

  assert.match(container.innerHTML, /&lt;Kitchener&gt;/);
  assert.match(container.innerHTML, /&lt;quiet&gt;/);
  assert.match(container.innerHTML, /&lt;M1&gt;/);
});
