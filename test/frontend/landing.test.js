const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadLanding({ search = '', standalone = false, lastDashboard = '' } = {}) {
  let ready = null;
  let showLandingCallback = null;
  const location = {
    search,
    href: '',
    replaced: '',
    replace(url) { this.replaced = url; },
  };
  const context = {
    URLSearchParams,
    window: {
      location,
      localStorage: {
        getItem: key => key === 'cupola:last-dashboard-id' ? lastDashboard : null,
      },
      matchMedia: () => ({ matches: standalone }),
      navigator: {},
    },
    document: {
      addEventListener(name, cb) {
        if (name === 'DOMContentLoaded') ready = cb;
      },
    },
    AppUI: {
      registerServiceWorker() {},
    },
    Profile: {
      showLanding(cb) { showLandingCallback = cb; },
    },
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/landing.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  ready();
  return { location, showLanding: profile => showLandingCallback(profile), didShowLanding: () => !!showLandingCallback };
}

test('landing navigates selected dashboard by query param', () => {
  const page = loadLanding();

  assert.equal(page.didShowLanding(), true);
  page.showLanding({ id: 'home screen' });

  assert.equal(page.location.href, '/dashboard.html?id=home+screen');
});

test('landing restores last dashboard when opened as PWA', () => {
  const page = loadLanding({ standalone: true, lastDashboard: 'kitchen' });

  assert.equal(page.location.replaced, '/dashboard.html?id=kitchen');
  assert.equal(page.didShowLanding(), false);
});

test('landing list flag suppresses PWA dashboard restore', () => {
  const page = loadLanding({ search: '?list=1', standalone: true, lastDashboard: 'kitchen' });

  assert.equal(page.location.replaced, '');
  assert.equal(page.didShowLanding(), true);
});

test('landing maps legacy kiosk URL to dashboard page', () => {
  const page = loadLanding({ search: '?kiosk=1&profile=wall' });

  assert.equal(page.location.replaced, '/dashboard.html?id=wall&kiosk=1');
  assert.equal(page.didShowLanding(), false);
});
