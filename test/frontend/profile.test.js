const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function element({ value = '', dataset = {} } = {}) {
  return {
    value,
    dataset,
    disabled: false,
    listeners: {},
    addEventListener(name, cb) { this.listeners[name] = cb; },
    select() { this.selected = true; },
    classList: {
      add() {},
      remove() {},
    },
  };
}

function loadProfile({ saveProfile }) {
  const els = {
    '#btn-new-layout': element(),
    '#btn-import-dashboard': element(),
    '#dashboard-import-file': element(),
    '#btn-confirm-name': element(),
    '#btn-cancel-name': element(),
    '#landing-name-form': element(),
    '#landing-name-input': element({ value: 'Kitchen Dashboard' }),
  };
  const landing = {
    innerHTML: '',
    classList: { remove() {}, add() {} },
    querySelector: sel => els[sel] || null,
    querySelectorAll: sel => sel === '.btn-load-profile' ? [] : [],
  };
  const context = {
    Date: { now: () => 12345 },
    window: { innerWidth: 1024 },
    document: { getElementById: id => id === 'landing' ? landing : null },
    DashboardAPI: {
      listProfiles: async () => [],
      saveProfile,
    },
    AppUI: {
      reportError(message, err) { context.error = { message, err }; },
    },
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/profile.js');
  vm.runInContext(fs.readFileSync(file, 'utf8') + '\nthis.__Profile = Profile;', context);
  return { Profile: context.__Profile, els, error: () => context.error };
}

test('new dashboard waits for save before launch', async () => {
  const events = [];
  let resolveSave;
  const { Profile, els } = loadProfile({
    saveProfile: async profile => {
      events.push(['save-start', profile.id]);
      await new Promise(resolve => { resolveSave = resolve; });
      events.push(['save-done', profile.id]);
    },
  });

  await Profile.showLanding(profile => events.push(['launch', profile.id]));
  const click = els['#btn-confirm-name'].listeners.click();

  assert.equal(els['#btn-confirm-name'].disabled, true);
  assert.deepEqual(events, [['save-start', 'kitchen-dashboard-12345']]);

  resolveSave();
  await click;

  assert.deepEqual(events, [
    ['save-start', 'kitchen-dashboard-12345'],
    ['save-done', 'kitchen-dashboard-12345'],
    ['launch', 'kitchen-dashboard-12345'],
  ]);
});

test('new dashboard does not launch when save fails', async () => {
  const events = [];
  const err = new Error('boom');
  const { Profile, els, error } = loadProfile({
    saveProfile: async () => { throw err; },
  });

  await Profile.showLanding(profile => events.push(['launch', profile.id]));
  await els['#btn-confirm-name'].listeners.click();

  assert.deepEqual(events, []);
  assert.equal(els['#btn-confirm-name'].disabled, false);
  assert.deepEqual(error(), { message: 'Profile save failed', err });
});
