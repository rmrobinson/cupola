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
      const res = await fetch('/api/v1/profiles');
      if (res.ok) profiles = await res.json();
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
    `;

    el.querySelector('#btn-new-layout').addEventListener('click', () => {
      el.querySelector('#landing-name-form').classList.remove('hidden');
      el.querySelector('#landing-name-input').select();
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
        const res = await fetch(`/api/v1/profiles/${btn.dataset.id}`);
        if (!res.ok) return;
        launch(await res.json(), onProfileLoaded);
      });
    });
  }

  function confirmName(el, defaultLayout, onProfileLoaded) {
    const name = (el.querySelector('#landing-name-input').value || '').trim() || 'My Dashboard';
    const id   = name.toLowerCase().replace(/[^a-z0-9]+/g, '-') + '-' + Date.now();
    const profile = { ...DEFAULTS[defaultLayout], id, name };
    fetch('/api/v1/profiles', {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify(profile),
    }).then(r => {
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
    }).catch(err => {
      AppUI.reportError('Profile save failed', err);
    });
    launch(profile, onProfileLoaded);
  }

  function launch(profile, onProfileLoaded) {
    onProfileLoaded(profile);
  }

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  return { showLanding };
})();
