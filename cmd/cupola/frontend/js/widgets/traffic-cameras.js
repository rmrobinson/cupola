(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // Returns cameras sorted by distance from a lat/lon, or all cameras if no location set.
  function sortedCameras(cameras, lat, lon) {
    if (!lat || !lon) return cameras;
    return cameras.slice().sort((a, b) => dist(a, lat, lon) - dist(b, lat, lon));
  }

  function dist(cam, lat, lon) {
    const dlat = cam.lat - lat;
    const dlon = cam.lon - lon;
    return dlat * dlat + dlon * dlon;
  }

  function render(container, state, config) {
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
          <img class="camera-snapshot" src="${esc(cam.snapshot_url)}"
               alt="${esc(cam.name)}" loading="lazy">
          <div class="camera-label">${esc(cam.name)}</div>
        </div>`;
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
  }

  function cameraThumb(cam) {
    return `
      <div class="camera-thumb">
        <img src="${esc(cam.snapshot_url)}" alt="${esc(cam.name)}" loading="lazy">
        <div class="camera-thumb-label">${esc(cam.name)}</div>
      </div>`;
  }

  window.CupolaWidgets.push({
    type:        'traffic-cameras',
    domain:      'traffic.cameras',
    defaultSize: { w: 4, h: 4 },
    configSchema: [
      { key: 'camera_id',   label: 'Camera ID (leave blank for list)', type: 'text',   default: '' },
      { key: 'max_cameras', label: 'Max cameras (list mode)',          type: 'number', default: 4 },
    ],
    subscriptionParams: () => ({ province: 'ON' }),
    render(container, state, config)  { render(container, state, config); },
    onUpdate(container, data, config) { render(container, data, config); },
  });
})();
