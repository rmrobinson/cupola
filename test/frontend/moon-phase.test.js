const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadWidget() {
  const context = {
    window: { CupolaWidgets: [] },
    Number,
    Math,
    String,
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/widgets/moon-phase.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return context.window.CupolaWidgets[0];
}

function renderPhase(phase, illumination = 0.5, name = 'quarter') {
  const widget = loadWidget();
  const container = {};
  widget.render(container, {
    moon_phase: phase,
    moon_phase_name: name,
    moon_illumination: illumination,
  });
  return container.innerHTML;
}

test('moon phase renders last quarter as lit left half', () => {
  const html = renderPhase(0.75, 0.48, 'Last Quarter');

  assert.match(html, /48% illuminated/);
  assert.match(html, /<circle cx="50" cy="50" r="36" fill="#0a0a1e"\/>/);
  assert.match(html, /<path d="M 50 14 A 36 36 0 0 0 50 86 L 50 14 Z" fill="rgba\(255,230,100,0\.88\)"/);
  assert.doesNotMatch(html, /<ellipse/);
  assert.doesNotMatch(html, /mask=/);
});

test('moon phase renders first quarter as lit right half', () => {
  const html = renderPhase(0.25, 0.5, 'First Quarter');

  assert.match(html, /<circle cx="50" cy="50" r="36" fill="rgba\(255,230,100,0\.88\)"\/>/);
  assert.match(html, /<path d="M 50 14 A 36 36 0 0 0 50 86 L 50 14 Z" fill="#0a0a1e"/);
});

test('moon phase keeps new and full moon special cases', () => {
  assert.match(renderPhase(0, 0, 'New'), /fill="#0a0a1e" stroke="rgba\(255,230,100,0\.3\)"/);
  assert.match(renderPhase(0.5, 1, 'Full'), /<circle cx="50" cy="50" r="36" fill="rgba\(255,230,100,0\.88\)"\/>/);
});
