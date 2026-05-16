/**
 * main.js — orchestrator (loaded last).
 *
 * Initialises: SSE → Horizon → Profile landing or kiosk auto-load → Grid
 * Exposes: Widgets registry (consumed by grid.js)
 */

// ── Config ────────────────────────────────────────────────────────────────────
// Fetch home lat/lon at script-load time so the promise is already in flight
// before DOMContentLoaded. launchCanvas awaits it, guaranteeing distance-sort
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

// ── Horizon Gradient ─────────────────────────────────────────────────────────

const Horizon = (() => {
  const STOPS = [
    [ 0,  '#0a0a1e', '#1a1a3a'],
    [ 5,  '#0d1b3a', '#1e2f5a'],
    [ 6,  '#3a1c71', '#d76d77'],
    [ 7,  '#fc4a1a', '#f7b733'],
    [ 9,  '#1a78c2', '#74b9ff'],
    [12,  '#0c75d8', '#a8d8f0'],
    [16,  '#1a78c2', '#74b9ff'],
    [18,  '#e8a33a', '#ff7043'],
    [19,  '#bf360c', '#7b1fa2'],
    [21,  '#2c2c6c', '#1a1a3a'],
    [24,  '#0a0a1e', '#1a1a3a'],
  ];

  let _astro = null;

  function hexRgb(h) {
    return [parseInt(h.slice(1,3),16), parseInt(h.slice(3,5),16), parseInt(h.slice(5,7),16)];
  }
  function lerp(a, b, t) { return Math.round(a + (b - a) * t); }
  function mix(c1, c2, t) {
    const [r1,g1,b1] = hexRgb(c1), [r2,g2,b2] = hexRgb(c2);
    return `rgb(${lerp(r1,r2,t)},${lerp(g1,g2,t)},${lerp(b1,b2,t)})`;
  }

  function colorsFromAstro(now, astro) {
    const ms = now.getTime();
    const dawn  = new Date(astro.civil_dawn).getTime();
    const rise  = new Date(astro.sunrise).getTime();
    const noon  = new Date(astro.solar_noon).getTime();
    const set   = new Date(astro.sunset).getTime();
    const dusk  = new Date(astro.civil_dusk).getTime();
    const h90   = 90 * 60 * 1000;

    const periods = [
      { s: 0,         e: dawn,       top: '#0a0a1e', bot: '#1a1a3a' },
      { s: dawn,      e: rise,       top: '#3a1c71', bot: '#d76d77' },
      { s: rise,      e: rise+h90,   top: '#fc4a1a', bot: '#f7b733' },
      { s: rise+h90,  e: noon,       top: '#1a78c2', bot: '#74b9ff' },
      { s: noon,      e: set-h90,    top: '#0c75d8', bot: '#a8d8f0' },
      { s: set-h90,   e: set,        top: '#e8a33a', bot: '#ff7043' },
      { s: set,       e: dusk,       top: '#bf360c', bot: '#7b1fa2' },
      { s: dusk,      e: Infinity,   top: '#0a0a1e', bot: '#1a1a3a' },
    ];

    for (let i = 0; i < periods.length - 1; i++) {
      const p = periods[i], q = periods[i + 1];
      if (ms >= p.s && ms < p.e) {
        const t = Math.max(0, Math.min(1, (ms - p.s) / (p.e - p.s)));
        return [mix(p.top, q.top, t), mix(p.bot, q.bot, t)];
      }
    }
    const last = periods[periods.length - 1];
    return [last.top, last.bot];
  }

  function colorsFromClock(now) {
    const h = now.getHours() + now.getMinutes() / 60;
    let i = 0;
    while (i < STOPS.length - 2 && STOPS[i + 1][0] <= h) i++;
    const [h0, t0, b0] = STOPS[i], [h1, t1, b1] = STOPS[i + 1];
    const t = Math.max(0, Math.min(1, (h - h0) / (h1 - h0)));
    return [mix(t0, t1, t), mix(b0, b1, t)];
  }

  function update() {
    const el = document.getElementById('horizon-bg');
    if (!el) return;
    const now = new Date();
    const [top, bot] = _astro ? colorsFromAstro(now, _astro) : colorsFromClock(now);
    el.style.background = `linear-gradient(180deg, ${top} 0%, ${bot} 100%)`;
  }

  function setAstro(data) {
    _astro = data;
    update();
  }

  function start() {
    update();
    setInterval(update, 30_000);
  }

  return { start, setAstro };
})();

// ── Profile save helper ───────────────────────────────────────────────────────

function saveProfile(profile) {
  return DashboardAPI.saveProfile(profile).catch(err => {
    AppUI.reportError('Dashboard save failed', err);
  });
}

// ── Init ──────────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
  const params = new URLSearchParams(window.location.search);
  const isKiosk = params.get('kiosk') === '1';
  const kioskProfileId = params.get('profile');

  Stream.connect();
  AppUI.registerServiceWorker();
  Horizon.start();

  // Wire horizon to astro domain
  Stream.on('astro', data => Horizon.setAstro(data));

  if (isKiosk && kioskProfileId) {
    // Kiosk mode: auto-load the named profile, suppress chrome
    document.getElementById('canvas').classList.add('kiosk');
    DashboardAPI.getProfile(kioskProfileId)
      .then(profile => {
        launchCanvas(profile);
      })
      .catch(err => {
        console.error('[cupola] kiosk profile not found:', kioskProfileId, err);
      });
  } else {
    Profile.showLanding(profile => launchCanvas(profile));
  }
});

async function launchCanvas(profile) {
  await _configReady;

  const canvas = document.getElementById('canvas');
  document.getElementById('landing').classList.add('hidden');
  canvas.classList.remove('hidden');

  Grid.init(profile, saveProfile);
  window.CupolaActiveProfile = profile;

  // "+ Widget" button
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
