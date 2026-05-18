/**
 * Profile — landing screen and profile selection.
 *
 * showLanding(onProfileLoaded) renders the landing screen and calls
 * onProfileLoaded(profile) when the user picks or creates a profile.
 * main.js provides the callback so profile.js has no dependency on Widgets.
 */
const Profile = (() => {
  const DEFAULTS = {
    landscape: {
      layout: 'landscape',
      grid_version: 2,
      widgets: [{
        id: 'w-notes',
        type: 'shared-notes',
        pos: { col: 0, row: 0, w: 8, h: 8 },
        config: {},
      }],
    },
    portrait: {
      layout: 'portrait',
      grid_version: 2,
      widgets: [{
        id: 'w-notes',
        type: 'shared-notes',
        pos: { col: 0, row: 0, w: 8, h: 8 },
        config: {},
      }],
    },
  };

  async function showLanding(onProfileLoaded) {
    const landing = document.getElementById('landing');
    landing.classList.remove('hidden');

    let profiles = [];
    try {
      profiles = await DashboardAPI.listProfiles();
    } catch {}

    render(landing, profiles, onProfileLoaded);
  }

  function render(el, profiles, onProfileLoaded) {
    const defaultLayout = window.innerWidth >= 768 ? 'landscape' : 'portrait';

    el.innerHTML = `
      <div class="landing-card">
        <h1 class="landing-heading">
          <img src="/icons/icon-192.png" alt="" class="landing-heading-icon">
          <span>Cupola</span>
        </h1>
        <div class="landing-body">
          <section class="landing-section">
            <button id="btn-new-layout" class="btn-primary">New dashboard</button>
            <button id="btn-import-dashboard" class="btn-secondary">Import dashboard</button>
            <input id="dashboard-import-file" type="file" accept="application/json,.json" hidden>
          </section>
          ${profiles.length ? `
          <section class="landing-section">
            <h2 class="landing-section-title">Saved profiles</h2>
            <ul class="profile-list">
              ${profiles.map(p => `
                <li class="profile-item">
                  <button class="btn-load-profile" data-id="${esc(p.id)}">${esc(p.name)}</button>
                  <span class="profile-layout-tag">${esc(p.layout)}</span>
                </li>
              `).join('')}
            </ul>
          </section>
          ` : ''}
          <div id="landing-name-form" class="landing-section hidden">
            <h2 class="landing-section-title">Name this dashboard</h2>
            <div class="name-form-row">
              <input id="landing-name-input" class="landing-name-input"
                     type="text" placeholder="My Dashboard" value="My Dashboard">
              <button id="btn-confirm-name" class="btn-primary">Open</button>
              <button id="btn-cancel-name"  class="btn-secondary">Cancel</button>
            </div>
          </div>
        </div>
      </div>
      <div id="dashboard-import-modal" class="import-modal hidden"></div>
    `;

    el.querySelector('#btn-new-layout').addEventListener('click', () => {
      el.querySelector('#landing-name-form').classList.remove('hidden');
      el.querySelector('#landing-name-input').select();
    });

    const fileInput = el.querySelector('#dashboard-import-file');
    el.querySelector('#btn-import-dashboard').addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', () => {
      const file = fileInput.files?.[0];
      fileInput.value = '';
      if (file) startImport(el, file, onProfileLoaded);
    });

    el.querySelector('#btn-confirm-name').addEventListener('click', () =>
      confirmName(el, defaultLayout, onProfileLoaded)
    );

    el.querySelector('#landing-name-input').addEventListener('keydown', e => {
      if (e.key === 'Enter') confirmName(el, defaultLayout, onProfileLoaded);
    });

    el.querySelector('#btn-cancel-name').addEventListener('click', () =>
      el.querySelector('#landing-name-form').classList.add('hidden')
    );

    el.querySelectorAll('.btn-load-profile').forEach(btn => {
      btn.addEventListener('click', async () => {
        try {
          launch(await DashboardAPI.getProfile(btn.dataset.id), onProfileLoaded);
        } catch (err) {
          AppUI.reportError('Profile load failed', err);
        }
      });
    });
  }

  function confirmName(el, defaultLayout, onProfileLoaded) {
    const name = (el.querySelector('#landing-name-input').value || '').trim() || 'My Dashboard';
    const id   = name.toLowerCase().replace(/[^a-z0-9]+/g, '-') + '-' + Date.now();
    const profile = { ...DEFAULTS[defaultLayout], id, name };
    DashboardAPI.saveProfile(profile).catch(err => {
      AppUI.reportError('Profile save failed', err);
    });
    launch(profile, onProfileLoaded);
  }

  function launch(profile, onProfileLoaded) {
    onProfileLoaded(profile);
  }

  async function startImport(root, file, onProfileLoaded) {
    let exportData;
    try {
      exportData = JSON.parse(await file.text());
    } catch (err) {
      AppUI.reportError('Dashboard import file is not valid JSON', err);
      return;
    }
    try {
      renderImportPreview(root, exportData, await DashboardAPI.validateImport(exportData), onProfileLoaded);
    } catch (err) {
      AppUI.reportError('Dashboard import validation failed', err);
    }
  }

  function renderImportPreview(root, exportData, validation, onProfileLoaded) {
    const modal = root.querySelector('#dashboard-import-modal');
    const defaultName = `${validation.profile_name || 'Imported dashboard'} (Imported)`;
    const rows = (validation.widgets || []).map(w => {
      const hardFail = w.status === 'missing_widget_type' || w.status === 'missing_domain';
      const checked = hardFail ? '' : ' checked';
      const disabled = hardFail ? ' disabled' : '';
      const statusLabel = w.status === 'ok' ? 'Ready' : w.status === 'config_warning' ? 'Warning' : 'Unavailable';
      const warnings = (w.warnings || []).concat((w.missing_domains || []).map(d => `Missing ${d}`));
      return `
        <label class="import-widget-row import-widget-${esc(w.status)}">
          <input type="checkbox" name="widget" value="${esc(w.id)}"${checked}${disabled}>
          <span class="import-widget-main">
            <span class="import-widget-title">${esc(w.label || w.type)}</span>
            <span class="import-widget-meta">${esc(statusLabel)}${warnings.length ? ` - ${esc(warnings.join('; '))}` : ''}</span>
          </span>
        </label>
      `;
    }).join('');

    modal.innerHTML = `
      <div class="import-card" role="dialog" aria-modal="true" aria-labelledby="import-title">
        <button class="import-close" aria-label="Close">&times;</button>
        <h2 id="import-title" class="import-title">Import dashboard</h2>
        <label class="import-name-row">
          <span>Name</span>
          <input id="import-dashboard-name" type="text" value="${esc(defaultName)}">
        </label>
        <div class="import-summary">
          ${esc(validation.layout || '')} dashboard - ${(validation.widgets || []).length} widgets
        </div>
        <div class="import-widget-list">${rows || '<p class="import-empty">No widgets found in this export.</p>'}</div>
        <div class="import-actions">
          <button class="btn-secondary import-cancel">Cancel</button>
          <button class="btn-primary import-confirm"${validation.can_import ? '' : ' disabled'}>Import</button>
        </div>
      </div>
    `;
    modal.classList.remove('hidden');
    modal.querySelector('.import-close').addEventListener('click', () => closeImport(modal));
    modal.querySelector('.import-cancel').addEventListener('click', () => closeImport(modal));
    modal.addEventListener('click', e => {
      if (e.target === modal) closeImport(modal);
    });
    modal.querySelector('.import-confirm').addEventListener('click', () =>
      confirmImport(modal, exportData, onProfileLoaded)
    );
    modal.querySelector('#import-dashboard-name')?.focus();
  }

  async function confirmImport(modal, exportData, onProfileLoaded) {
    const btn = modal.querySelector('.import-confirm');
    const selected = [...modal.querySelectorAll('input[name="widget"]:checked')].map(el => el.value);
    const name = modal.querySelector('#import-dashboard-name')?.value || '';
    btn.disabled = true;
    try {
      const data = await DashboardAPI.importProfile({ exportData, name, widgetIDs: selected });
      closeImport(modal);
      launch(data.profile, onProfileLoaded);
    } catch (err) {
      btn.disabled = false;
      AppUI.reportError('Dashboard import failed', err);
    }
  }

  function closeImport(modal) {
    modal.classList.add('hidden');
    modal.innerHTML = '';
  }

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  return { showLanding };
})();
