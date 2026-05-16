const test = require('node:test');
const assert = require('node:assert/strict');

const DashboardAPI = require('../../cmd/cupola/frontend/js/dashboard-api.js');

test('listProfiles fetches saved dashboard profiles', async () => {
  global.fetch = async url => {
    assert.equal(url, '/api/v1/profiles');
    return {
      ok: true,
      json: async () => [{ id: 'home', name: 'Home' }],
    };
  };

  assert.deepEqual(await DashboardAPI.listProfiles(), [{ id: 'home', name: 'Home' }]);
});

test('getProfile fetches a dashboard profile by encoded id', async () => {
  global.fetch = async url => {
    assert.equal(url, '/api/v1/profiles/home%20screen');
    return {
      ok: true,
      json: async () => ({ id: 'home screen', name: 'Home' }),
    };
  };

  assert.deepEqual(await DashboardAPI.getProfile('home screen'), { id: 'home screen', name: 'Home' });
});

test('saveProfile posts dashboard profile JSON', async () => {
  const calls = [];
  global.fetch = async (url, opts) => {
    calls.push({ url, opts });
    return { ok: true };
  };

  await DashboardAPI.saveProfile({ id: 'home', name: 'Home' });

  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, '/api/v1/profiles');
  assert.equal(calls[0].opts.method, 'POST');
  assert.equal(calls[0].opts.headers['Content-Type'], 'application/json');
  assert.deepEqual(JSON.parse(calls[0].opts.body), { id: 'home', name: 'Home' });
});

test('saveProfile rejects failed saves', async () => {
  global.fetch = async () => ({ ok: false, status: 500 });

  await assert.rejects(
    () => DashboardAPI.saveProfile({ id: 'home' }),
    /HTTP 500/
  );
});

test('deleteProfile deletes a dashboard profile by encoded id', async () => {
  const calls = [];
  global.fetch = async (url, opts) => {
    calls.push({ url, opts });
    return { ok: true };
  };

  await DashboardAPI.deleteProfile('home screen');

  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, '/api/v1/profiles/home%20screen');
  assert.equal(calls[0].opts.method, 'DELETE');
});

test('filenameFromContentDisposition extracts quoted filename', () => {
  assert.equal(
    DashboardAPI.filenameFromContentDisposition('attachment; filename="cupola-dashboard-home.json"'),
    'cupola-dashboard-home.json'
  );
});
