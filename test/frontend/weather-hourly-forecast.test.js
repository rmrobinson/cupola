const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadWidget() {
  const context = {
    window: { CupolaWidgets: [], CupolaDetails: { show: () => {} } },
    Date,
    Number,
    Math,
    String,
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/weather-hourly-forecast.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

test('weather hourly forecast widget registers for hourly domain', () => {
  const widget = loadWidget();
  assert.equal(widget.type, 'weather-hourly-forecast');
  assert.equal(widget.domain, 'weather.forecast.hourly');
  assert.equal(widget.defaultSize.w, 7);
  assert.equal(widget.defaultSize.h, 7);
});

test('weather hourly forecast widget renders populated data and icon URL', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    hours: [{
      starts_at: '2999-05-17T13:00:00Z',
      ends_at: '2999-05-17T14:00:00Z',
      condition: 'Chance of showers',
      temperature: 21,
      humidex: 25,
      precip_chance: 60,
      uv_index: 3,
      wind_direction: 'SW',
      wind_speed: 20,
      wind_gust: 40,
      icon_url: 'https://weather.gc.ca/weathericons/small/09.png',
    }],
  });

  assert.match(container.innerHTML, /widget-weather-hourly/);
  assert.match(container.innerHTML, /hourly-period has-icon/);
  assert.match(container.innerHTML, /hp-temp-main">25&deg;/);
  assert.match(container.innerHTML, /hp-temp-actual">21&deg;/);
  assert.match(container.innerHTML, /21&deg;/);
  assert.match(container.innerHTML, /POP 60%/);
  assert.match(container.innerHTML, /hp-uv">UV 3/);
  assert.doesNotMatch(container.innerHTML, /Feels/);
  assert.match(container.innerHTML, /SW 20 km\/h G 40/);
  assert.match(container.innerHTML, /https:\/\/weather\.gc\.ca\/weathericons\/small\/09\.png/);
  assert.match(container.innerHTML, /classList\.remove\('has-icon'\)/);
  assert.match(container.innerHTML, /classList\.add\('no-icon'\)/);
});

test('weather hourly forecast widget renders empty state', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, { hours: [] });

  assert.match(container.innerHTML, /No hourly forecast/);
});

test('weather hourly forecast widget omits image when icon URL is missing', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    hours: [{
      starts_at: '2999-05-17T13:00:00Z',
      ends_at: '2999-05-17T14:00:00Z',
      condition: 'Clear',
      temperature: 19,
    }],
  });

  assert.doesNotMatch(container.innerHTML, /<img/);
  assert.match(container.innerHTML, /hourly-period no-icon/);
  assert.match(container.innerHTML, /<span class="hp-condition">Clear<\/span>/);
});

test('weather hourly forecast widget handles mixed icon availability per row', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    hours: [
      {
        starts_at: '2999-05-17T13:00:00Z',
        ends_at: '2999-05-17T14:00:00Z',
        condition: 'Sunny',
        temperature: 19,
        icon_url: 'https://weather.gc.ca/weathericons/small/00.png',
      },
      {
        starts_at: '2999-05-17T14:00:00Z',
        ends_at: '2999-05-17T15:00:00Z',
        condition: 'Cloudy',
        temperature: 20,
      },
    ],
  });

  assert.match(container.innerHTML, /hourly-period has-icon/);
  assert.match(container.innerHTML, /hourly-period no-icon/);
  assert.match(container.innerHTML, /<span class="hp-condition">Cloudy<\/span>/);
});

test('weather hourly forecast widget hides expired hours', () => {
  const widget = loadWidget();
  const container = {};

  widget.render(container, {
    hours: [
      {
        starts_at: '2000-01-01T12:00:00Z',
        ends_at: '2000-01-01T13:00:00Z',
        condition: 'Expired',
        temperature: 1,
      },
      {
        starts_at: '2999-05-17T13:00:00Z',
        ends_at: '2999-05-17T14:00:00Z',
        condition: 'Current',
        temperature: 19,
      },
    ],
  });

  assert.doesNotMatch(container.innerHTML, /Expired/);
  assert.match(container.innerHTML, /Current/);
});

test('weather hourly forecast widget keeps the current hour visible', () => {
  const widget = loadWidget();
  const container = {};
  const currentHour = new Date();
  currentHour.setMinutes(0, 0, 0);
  const previousHour = new Date(currentHour.getTime() - 60 * 60 * 1000);
  const nextHour = new Date(currentHour.getTime() + 60 * 60 * 1000);

  widget.render(container, {
    hours: [
      {
        starts_at: previousHour.toISOString(),
        condition: 'Previous',
        temperature: 1,
      },
      {
        starts_at: currentHour.toISOString(),
        condition: 'Current hour',
        temperature: 2,
      },
      {
        starts_at: nextHour.toISOString(),
        condition: 'Next hour',
        temperature: 3,
      },
    ],
  });

  assert.doesNotMatch(container.innerHTML, /Previous/);
  assert.match(container.innerHTML, /Current hour/);
  assert.match(container.innerHTML, /Next hour/);
});

test('weather hourly forecast widget opens local detail on row click', () => {
  const context = {
    window: {
      CupolaWidgets: [],
      CupolaDetails: {
        shown: null,
        show(detail) { this.shown = detail; },
      },
    },
    Date,
    Number,
    Math,
    String,
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/weather-hourly-forecast.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  const widget = context.window.CupolaWidgets[0];
  const row = {
    dataset: { hourIndex: '0' },
    handlers: {},
    addEventListener(name, fn) { this.handlers[name] = fn; },
  };
  const container = {
    innerHTML: '',
    querySelectorAll: () => [row],
  };

  widget.render(container, {
    hours: [{
      starts_at: '2999-05-17T13:00:00Z',
      ends_at: '2999-05-17T14:00:00Z',
      condition: 'Sunny',
      temperature: 20,
      precip_chance: 10,
      wind_speed: 15,
    }],
  });
  row.handlers.click();

  assert.equal(context.window.CupolaDetails.shown.domain, 'weather.forecast.hourly');
  assert.equal(context.window.CupolaDetails.shown.subtitle, 'Sunny');
  assert.equal(
    JSON.stringify(context.window.CupolaDetails.shown.fields.map(f => f.key)),
    JSON.stringify(['starts_at', 'ends_at', 'condition', 'temperature', 'precip_chance', 'wind_speed'])
  );
});
