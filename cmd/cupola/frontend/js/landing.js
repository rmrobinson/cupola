(function () {
  const LAST_DASHBOARD_KEY = 'cupola:last-dashboard-id';

  document.addEventListener('DOMContentLoaded', () => {
    AppUI.registerServiceWorker();
    if (typeof Horizon !== 'undefined') Horizon.start();

    const params = new URLSearchParams(window.location.search);
    const kioskProfileId = params.get('kiosk') === '1' ? params.get('profile') : '';
    if (kioskProfileId) {
      window.location.replace(dashboardURL(kioskProfileId, { kiosk: true }));
      return;
    }

    if (isPWAApp() && params.get('list') !== '1') {
      const lastID = window.localStorage?.getItem(LAST_DASHBOARD_KEY);
      if (lastID) {
        window.location.replace(dashboardURL(lastID));
        return;
      }
    }

    Profile.showLanding(profile => {
      if (!profile?.id) return;
      window.location.href = dashboardURL(profile.id);
    });
  });

  function dashboardURL(id, opts = {}) {
    const params = new URLSearchParams({ id });
    if (opts.kiosk) params.set('kiosk', '1');
    return `/dashboard.html?${params.toString()}`;
  }

  function isPWAApp() {
    return window.matchMedia?.('(display-mode: standalone)').matches ||
      window.matchMedia?.('(display-mode: fullscreen)').matches ||
      window.navigator?.standalone === true;
  }
})();
