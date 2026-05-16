(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  // Camera images have Cache-Control: max-age=20 at the source; refresh at the
  // same cadence so the widget shows near-live snapshots between SSE updates.
  const REFRESH_MS = 20_000;

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function sortedCameras(cameras, lat, lon) {
    if (!lat || !lon) return cameras;
    return cameras.slice().sort((a, b) => dist(a, lat, lon) - dist(b, lat, lon));
  }

  function dist(cam, lat, lon) {
    const dlat = cam.lat - lat;
    const dlon = cam.lon - lon;
    return dlat * dlat + dlon * dlon;
  }

  // ── Image auto-refresh ────────────────────────────────────────────────────
  // Store the interval on the container element so each render call replaces
  // the previous timer. Note: if the widget is removed from the grid, the
  // timer will keep firing harmlessly against the detached DOM until page
  // reload — a grid-level destroy() hook would be needed to eliminate this.

  function startRefresh(container) {
    stopRefresh(container);
    container._camRefreshId = setInterval(() => {
      const ts = Date.now();
      container.querySelectorAll('img[data-src]').forEach(img => {
        img.src = img.dataset.src + '?t=' + ts;
      });
    }, REFRESH_MS);
  }

  function stopRefresh(container) {
    if (container._camRefreshId) {
      clearInterval(container._camRefreshId);
      container._camRefreshId = null;
    }
  }

  // ── Render ────────────────────────────────────────────────────────────────

  function render(container, state, config) {
    stopRefresh(container);

    const cameraID = (config?.camera_id || '').trim();
    const allCams = state?.cameras || [];

    // Single-camera mode: show a full-size snapshot when camera_id is configured.
    if (cameraID) {
      const cam = allCams.find(c => c.id === cameraID);
      if (!cam) {
        container.innerHTML = `
          <div class="widget-traffic-camera">
            <p class="camera-no-data">${state ? 'Camera not found' : 'Waiting for data…'}</p>
          </div>`;
        return;
      }
      container.innerHTML = `
        <div class="widget-traffic-camera">
          <img class="camera-snapshot" data-src="${esc(cam.snapshot_url)}"
               src="${esc(cam.snapshot_url)}"
               alt="${esc(cam.name)}" loading="lazy">
          <div class="camera-label">${esc(cam.name)}</div>
        </div>`;
      startRefresh(container);
      return;
    }

    // List mode: show nearest N cameras as thumbnails.
    const maxN = config?.max_cameras > 0 ? Number(config.max_cameras) : 4;
    const cameras = sortedCameras(allCams, window.CupolaConfig?.lat, window.CupolaConfig?.lon).slice(0, maxN);

    if (cameras.length === 0) {
      container.innerHTML = `
        <div class="widget-traffic-cameras">
          <div class="traffic-header">
            <span class="traffic-title">Traffic Cameras</span>
          </div>
          <p class="traffic-empty">${state ? 'No cameras available' : 'Waiting for data…'}</p>
        </div>`;
      return;
    }

    container.innerHTML = `
      <div class="widget-traffic-cameras">
        <div class="traffic-header">
          <span class="traffic-title">Traffic Cameras</span>
        </div>
        <div class="camera-grid">
          ${cameras.map(cameraThumb).join('')}
        </div>
      </div>`;
    startRefresh(container);
  }

  function cameraThumb(cam) {
    return `
      <div class="camera-thumb">
        <img data-src="${esc(cam.snapshot_url)}" src="${esc(cam.snapshot_url)}"
             alt="${esc(cam.name)}" loading="lazy">
        <div class="camera-thumb-label">${esc(cam.name)}</div>
      </div>`;
  }

  window.CupolaWidgets.push({
    type:        'traffic-cameras',
    domain:      'traffic.cameras',
    defaultSize: { w: 7, h: 4 },
    configSchema: [
      { key: 'camera_id',   label: 'Camera ID (leave blank for list)', type: 'text',   default: '' },
      { key: 'max_cameras', label: 'Max cameras (list mode)',          type: 'number', default: 4 },
    ],
    subscriptionParams: () => ({ province: 'ON' }),
    render(container, state, config)  { render(container, state, config); },
    onUpdate(container, data, config) { render(container, data, config); },
  });
})();
