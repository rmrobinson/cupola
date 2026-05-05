const CACHE_VERSION = '2026-05-05';
const STATIC_CACHE = `cupola-static-${CACHE_VERSION}`;
const API_CACHE    = `cupola-api-${CACHE_VERSION}`;

const PRECACHE = [
  '/',
  '/manifest.json',
  '/icons/icon.svg',
  '/css/main.css',
  '/css/grid.css',
  '/css/horizon.css',
  '/js/stream.js',
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
        keys.filter(k => k !== STATIC_CACHE && k !== API_CACHE)
            .map(k => caches.delete(k))
      ))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', evt => {
  const { request } = evt;
  if (request.method !== 'GET') return;

  const { pathname } = new URL(request.url);

  // SSE stream: network-only — cannot be cached or replayed
  if (pathname === '/api/v1/stream') return;

  if (pathname.startsWith('/api/')) {
    evt.respondWith(networkFirst(request));
  } else {
    evt.respondWith(cacheFirst(request));
  }
});

async function networkFirst(request) {
  const cache = await caches.open(API_CACHE);
  try {
    const response = await fetch(request);
    if (response.ok) cache.put(request, response.clone());
    return response;
  } catch {
    return (await cache.match(request))
      ?? new Response('{}', { headers: { 'Content-Type': 'application/json' } });
  }
}

async function cacheFirst(request) {
  const cached = await caches.match(request);
  if (cached) return cached;
  const response = await fetch(request);
  if (response.ok) {
    const cache = await caches.open(STATIC_CACHE);
    cache.put(request, response.clone());
  }
  return response;
}
