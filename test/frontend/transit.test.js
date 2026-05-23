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
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/transit.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

function stateWithArrivals(count) {
  const arrivals = [];
  for (let i = 0; i < count; i += 1) {
    arrivals.push({
      trip_id: `T${i}`,
      headsign: `Trip ${i}`,
      scheduled: `2999-05-17T${String(10 + i).padStart(2, '0')}:00:00Z`,
    });
  }
  return {
    stops: {
      'go:KI:KITCHENER': {
        agency_id: 'go',
        route_id: 'KI',
        route_name: 'KI',
        stop_id: 'KITCHENER',
        stop_name: 'Kitchener GO',
        arrivals,
      },
    },
  };
}

test('transit widget renders configured max trips', () => {
  const widget = loadWidget();
  const container = { innerHTML: '', dataset: {} };

  widget.render(container, stateWithArrivals(8), {
    agency: 'go',
    route: 'KI',
    stop_id: 'KITCHENER',
    max_trips: 6,
  });

  assert.equal((container.innerHTML.match(/class="arrival-row"/g) || []).length, 6);
});

test('transit widget includes max trips in subscription params', () => {
  const widget = loadWidget();

  const params = widget.subscriptionParams({
    agency: 'go',
    route: 'KI',
    stop_id: 'KITCHENER',
    max_trips: '9',
  });
  assert.deepEqual(JSON.parse(JSON.stringify(params)), {
    agency: 'go',
    route: 'KI',
    stop_id: 'KITCHENER',
    max_trips: 9,
  });
});

test('transit widget clamps max trips to supported range', () => {
  const widget = loadWidget();

  assert.equal(widget.subscriptionParams({
    agency: 'go',
    route: 'KI',
    stop_id: 'KITCHENER',
    max_trips: 99,
  }).max_trips, 20);

  assert.equal(widget.subscriptionParams({
    agency: 'go',
    route: 'KI',
    stop_id: 'KITCHENER',
    max_trips: 0,
  }).max_trips, 4);
});
