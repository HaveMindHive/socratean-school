/* Socratean service worker, app-shell cache, API always network */

var CACHE_NAME = 'socratean-shell-v10';

var SHELL = [
    '/',
    '/index.html',
    '/professor.html',
    '/teacher.html',
    '/app.js',
    '/styles.css',
    '/manifest.json',
    '/icon-180.png',
    '/icon-192.png',
    '/icon-512.png'
];

self.addEventListener('install', function (event) {
    event.waitUntil(
        caches.open(CACHE_NAME).then(function (cache) {
            // Add each shell asset individually so one missing icon
            // does not abort the whole install.
            return Promise.all(SHELL.map(function (url) {
                return cache.add(url).catch(function () { /* asset unavailable at install time */ });
            }));
        }).then(function () {
            return self.skipWaiting();
        })
    );
});

self.addEventListener('activate', function (event) {
    event.waitUntil(
        caches.keys().then(function (keys) {
            return Promise.all(keys.map(function (key) {
                if (key !== CACHE_NAME) return caches.delete(key);
            }));
        }).then(function () {
            return self.clients.claim();
        })
    );
});

self.addEventListener('fetch', function (event) {
    var req = event.request;
    if (req.method !== 'GET') return;

    var url = new URL(req.url);
    if (url.origin !== self.location.origin) return;

    // NEVER cache or serve API responses from cache, always network.
    if (url.pathname.indexOf('/api/') === 0) return;

    // Navigations: network first, fall back to cached shell for offline load.
    if (req.mode === 'navigate') {
        event.respondWith(
            fetch(req).then(function (res) {
                var copy = res.clone();
                caches.open(CACHE_NAME).then(function (cache) { cache.put(req, copy); });
                return res;
            }).catch(function () {
                return caches.match(req).then(function (cached) {
                    if (cached) return cached;
                    var fallback = url.pathname.indexOf('professor') !== -1 ? '/professor.html' : '/index.html';
                    return caches.match(fallback);
                });
            })
        );
        return;
    }

    // Static assets: cache first, then network (and refresh the cache).
    event.respondWith(
        caches.match(req).then(function (cached) {
            if (cached) return cached;
            return fetch(req).then(function (res) {
                if (res && res.ok) {
                    var copy = res.clone();
                    caches.open(CACHE_NAME).then(function (cache) { cache.put(req, copy); });
                }
                return res;
            });
        })
    );
});
