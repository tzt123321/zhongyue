const CACHE_NAME = 'zhongyue-v2';
const STATIC_ASSETS = [
  '/',
  '/index.html',
  '/assets/',
];

// Install: pre-cache static assets
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(STATIC_ASSETS);
    })
  );
  self.skipWaiting();
});

// Activate: clean old caches
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k))
      )
    )
  );
  self.clients.claim();
});

// Fetch strategy
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip non-GET and cross-origin requests
  if (request.method !== 'GET') return;
  if (url.origin !== self.location.origin) return;

  // API /stream → NetworkFirst (always fresh)
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/stream/')) {
    event.respondWith(
      fetch(request).catch(() => caches.match(request))
    );
    return;
  }

  // Covers/artists/lyrics → CacheFirst (stable assets)
  if (url.pathname.startsWith('/data/covers') ||
      url.pathname.startsWith('/data/artists') ||
      url.pathname.startsWith('/data/lyrics')) {
    event.respondWith(
      caches.match(request).then((cached) => {
        if (cached) return cached;
        return fetch(request).then((resp) => {
          if (resp.ok) {
            const clone = resp.clone();
            caches.open(CACHE_NAME).then((c) => c.put(request, clone));
          }
          return resp;
        });
      })
    );
    return;
  }

  // Static assets (JS/CSS/images) → CacheFirst
  if (request.destination === 'script' ||
      request.destination === 'style' ||
      request.destination === 'image') {
    event.respondWith(
      caches.match(request).then((cached) => {
        if (cached) return cached;
        return fetch(request).then((resp) => {
          if (resp.ok) {
            const clone = resp.clone();
            caches.open(CACHE_NAME).then((c) => c.put(request, clone));
          }
          return resp;
        });
      })
    );
    return;
  }

  // HTML (SPA) → NetworkFirst
  event.respondWith(
    fetch(request).then((resp) => {
      if (resp.ok) {
        const clone = resp.clone();
        caches.open(CACHE_NAME).then((c) => c.put(request, clone));
      }
      return resp;
    }).catch(() => caches.match(request) || caches.match('/index.html'))
  );
});
