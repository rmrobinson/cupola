const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadWidget() {
  const context = {
    window: { CupolaWidgets: [] },
    Date,
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/municipal-events.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

test('municipal events render cards as detail-clickable items', () => {
  const widget = loadWidget();
  const container = { innerHTML: '' };

  widget.render(container, {
    events: [{
      id: 'kitchener.roadclosures:event-1',
      source_id: 'kitchener.roadclosures',
      title: 'King St road closure',
      description: 'Closed for a special event',
      event_type: 'road-closure',
      starts_at: '2026-06-07T10:00:00Z',
      url: 'https://www.kitchener.ca/roadclosures',
    }],
  }, {});

  assert.match(container.innerHTML, /muni-event-card detail-clickable/);
  assert.match(container.innerHTML, /data-detail-domain="municipal\.events"/);
  assert.match(container.innerHTML, /data-detail-id="kitchener\.roadclosures:event-1"/);
  assert.doesNotMatch(container.innerHTML, /Details/);
  assert.doesNotMatch(container.innerHTML, /href="https:\/\/www\.kitchener\.ca\/roadclosures"/);
});
