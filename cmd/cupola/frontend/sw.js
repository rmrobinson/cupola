const CACHE_VERSION = 'runtime-v2';
const STATIC_CACHE = `cupola-static-${CACHE_VERSION}`;

const PRECACHE = [
  '/',
  '/admin',
  '/admin.html',
  '/manifest.json',
  '/favicon.ico',
  '/icons/apple-touch-icon.png',
  '/icons/favicon-32.png',
  '/icons/icon-192.png',
  '/icons/icon-512.png',
  '/css/main.css',
  '/css/admin.css',
  '/css/grid.css',
  '/css/horizon.css',
  '/js/stream.js',
  '/js/app-ui.js',
  '/js/details.js',
  '/js/admin.js',
  '/js/subscriptions.js',
  '/js/overlays.js',
  '/js/profile.js',
  '/js/widget-picker.js',
  '/js/grid.js',
  '/js/main.js',
  '/js/widgets/aircraft.js',
  '/js/widgets/alerts.js',
  '/js/widgets/clock.js',
  '/js/widgets/flag-status.js',
  '/js/widgets/moon-phase.js',
  '/js/widgets/municipal-events.js',
  '/js/widgets/radar-map.js',
  '/js/widgets/shared-notes.js',
  '/js/widgets/solar-activity.js',
  '/js/widgets/solar-forecast.js',
  '/js/widgets/sunrise-sunset.js',
  '/js/widgets/traffic-cameras.js',
  '/js/widgets/traffic-incidents.js',
  '/js/widgets/traffic-road-conditions.js',
  '/js/widgets/transit.js',
  '/js/widgets/waste-collection.js',
  '/js/widgets/waterway.js',
  '/js/widgets/weather-current.js',
  '/js/widgets/weather-forecast.js',
  '/js/widgets/weather-rainfall.js',
  '/js/vendor/leaflet.js',
  '/js/vendor/leaflet.css',
  '/js/vendor/protomaps-leaflet.js',
];

self.addEventListener('install', evt => {
  evt.waitUntil(
    caches.open(STATIC_CACHE)
      .then(c => c.addAll(PRECACHE))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', evt => {
  evt.waitUntil(
    caches.keys()
      .then(keys => Promise.all(
        keys.filter(k => k !== STATIC_CACHE)
            .map(k => caches.delete(k))
      ))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', evt => {
  const { request } = evt;
  if (request.method !== 'GET') return;

  const { pathname } = new URL(request.url);

  // Live API responses must not be replayed from cache on a situational dashboard.
  if (pathname.startsWith('/api/')) {
    return;
  }
  if (pathname.startsWith('/tiles/')) {
    return;
  }

  evt.respondWith(networkFirstStatic(request));
});

async function networkFirstStatic(request) {
  const cache = await caches.open(STATIC_CACHE);
  try {
    const response = await fetch(request);
    if (response.ok) cache.put(request, response.clone());
    return response;
  } catch {
    return (await cache.match(request))
      ?? new Response('Offline', { status: 503, headers: { 'Content-Type': 'text/plain' } });
  }
}
