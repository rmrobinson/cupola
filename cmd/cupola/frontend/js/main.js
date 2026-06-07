/**
 * main.js — dashboard page orchestrator (loaded last).
 *
 * Initialises: SSE -> Horizon -> dashboard profile load -> Grid
 * Exposes: Widgets registry (consumed by grid.js)
 */

// ── Config ────────────────────────────────────────────────────────────────────
// Fetch home lat/lon at script-load time so the promise is already in flight
// before DOMContentLoaded. launchDashboard awaits it, guaranteeing distance-sort
// widgets have coordinates on their first render.
window.CupolaConfig = {};
const _configReady = fetch('/api/v1/config')
  .then(r => r.json())
  .then(cfg => { window.CupolaConfig = cfg; })
  .catch(() => {});

// ── Widget Registry ──────────────────────────────────────────────────────────

// Consumed by grid.js via window.CupolaWidgets (populated by widget files).
// Also provide a typed registry for legacy Widgets.initProfile callers.
const Widgets = (() => {
  const registry = {};
  (window.CupolaWidgets || []).forEach(w => { registry[w.type] = w; });
  return { registry };
})();

// ── Profile save helper ───────────────────────────────────────────────────────

const LAST_DASHBOARD_KEY = 'cupola:last-dashboard-id';

function saveProfile(profile) {
  return DashboardAPI.saveProfile(profile).catch(err => {
    AppUI.reportError('Dashboard save failed', err);
  });
}

// ── Init ──────────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
  const params = new URLSearchParams(window.location.search);
  const isKiosk = params.get('kiosk') === '1';
  const profileId = params.get('id') || params.get('profile');

  if (!profileId) {
    window.location.replace('/');
    return;
  }
  if (isKiosk) {
    document.getElementById('canvas')?.classList.add('kiosk');
  }

  Stream.connect();
  installForegroundRefresh();
  AppUI.registerServiceWorker();
  if (typeof Horizon !== 'undefined') Horizon.start();

  // Wire horizon to astro domain
  if (typeof Horizon !== 'undefined') {
    Stream.on('astro', data => Horizon.setAstro(data));
  }

  DashboardAPI.getProfile(profileId)
    .then(profile => launchDashboard(profile, { isKiosk }))
    .catch(err => {
      if (window.localStorage?.getItem(LAST_DASHBOARD_KEY) === profileId) {
        window.localStorage.removeItem(LAST_DASHBOARD_KEY);
      }
      AppUI.reportError('Dashboard load failed', err);
      window.setTimeout(() => { window.location.href = '/?list=1'; }, 1200);
    });
});

function installForegroundRefresh() {
  let lastRefreshAt = Date.now();
  const refresh = (evt) => {
    if (document.visibilityState && document.visibilityState !== 'visible') return;
    const now = Date.now();
    if (now - lastRefreshAt < 750) return;
    lastRefreshAt = now;

    if (window.CupolaActiveProfile) Grid.refreshState();

    const restoredFromPageCache = evt?.type === 'pageshow' && evt.persisted;
    if (restoredFromPageCache || Stream.shouldReconnectOnResume()) {
      Stream.reconnectNow();
    }
  };

  window.addEventListener('pageshow', refresh);
  window.addEventListener('focus', refresh);
  window.addEventListener('online', refresh);
  document.addEventListener('visibilitychange', refresh);
}

async function launchDashboard(profile, opts = {}) {
  await _configReady;

  if (!opts.isKiosk && profile?.id) {
    window.localStorage?.setItem(LAST_DASHBOARD_KEY, profile.id);
  }

  const dashboardName = document.getElementById('canvas-dashboard-name');
  if (dashboardName) dashboardName.textContent = profile.name || profile.id || 'Dashboard';

  Grid.init(profile, saveProfile);
  window.CupolaActiveProfile = profile;

  const lockBtn = document.getElementById('btn-layout-lock');
  if (lockBtn && !lockBtn.dataset.bound) {
    lockBtn.dataset.bound = '1';
    lockBtn.addEventListener('click', () => setLayoutLocked(!Grid.isLocked()));
  }
  setLayoutLocked(isPWAApp());

  const dashboardListBtn = document.getElementById('btn-dashboard-list');
  if (dashboardListBtn && !dashboardListBtn.dataset.bound) {
    dashboardListBtn.dataset.bound = '1';
    dashboardListBtn.addEventListener('click', () => {
      window.location.href = '/?list=1';
    });
  }

  const addBtn = document.getElementById('btn-add-widget');
  if (addBtn && !addBtn.dataset.bound) {
    addBtn.dataset.bound = '1';
    addBtn.addEventListener('click', () => {
      WidgetPicker.show(def => {
        const wc = {
          id: 'w-' + Math.random().toString(36).slice(2, 9),
          type: def.type,
          pos: { col: 0, row: 0, w: def.defaultSize?.w || 4, h: def.defaultSize?.h || 2 },
          config: {},
        };
        Grid.addWidget(wc);
      });
    });
  }

  const exportBtn = document.getElementById('btn-export-dashboard');
  if (exportBtn && !exportBtn.dataset.bound) {
    exportBtn.dataset.bound = '1';
    exportBtn.addEventListener('click', () => exportDashboard(exportBtn));
  }

  const adminBtn = document.getElementById('btn-admin');
  if (adminBtn && !adminBtn.dataset.bound) {
    adminBtn.dataset.bound = '1';
    adminBtn.addEventListener('click', () => {
      window.location.href = '/admin';
    });
  }
}

function setLayoutLocked(locked) {
  Grid.setLocked(locked);
  const addBtn = document.getElementById('btn-add-widget');
  const lockBtn = document.getElementById('btn-layout-lock');
  if (addBtn) addBtn.disabled = locked;
  if (lockBtn) {
    lockBtn.textContent = locked ? 'Unlock' : 'Lock';
    lockBtn.title = locked ? 'Unlock layout editing' : 'Lock layout';
    lockBtn.setAttribute('aria-pressed', locked ? 'true' : 'false');
  }
}

function isPWAApp() {
  return window.matchMedia?.('(display-mode: standalone)').matches ||
    window.matchMedia?.('(display-mode: fullscreen)').matches ||
    window.navigator?.standalone === true;
}

async function exportDashboard(btn) {
  const profile = window.CupolaActiveProfile;
  if (!profile?.id) return;
  const prevText = btn.textContent;
  btn.disabled = true;
  btn.textContent = 'Exporting...';
  try {
    await DashboardAPI.saveProfile(profile);
    const { blob, filename } = await DashboardAPI.exportProfile(profile.id);
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  } catch (err) {
    AppUI.reportError('Dashboard export failed', err);
  } finally {
    btn.disabled = false;
    btn.textContent = prevText;
  }
}
