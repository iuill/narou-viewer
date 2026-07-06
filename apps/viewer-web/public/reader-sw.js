const APP_SHELL_CACHE_PREFIX = "narou-viewer-shell-";
const APP_SHELL_CACHE = `${APP_SHELL_CACHE_PREFIX}v2`;
const APP_SHELL_ASSETS = ["/", "/manifest.webmanifest"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(APP_SHELL_CACHE).then((cache) => {
      return cache.addAll(APP_SHELL_ASSETS);
    }),
  );
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((cacheNames) =>
        Promise.all(
          cacheNames.map((cacheName) => {
            if (!cacheName.startsWith(APP_SHELL_CACHE_PREFIX) || cacheName === APP_SHELL_CACHE) {
              return Promise.resolve(false);
            }

            return caches.delete(cacheName);
          }),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const { request } = event;

  if (request.method !== "GET") {
    return;
  }

  if (request.mode !== "navigate") {
    return;
  }

  event.respondWith(
    fetch(request).catch(async () => {
      const cached = await caches.match("/");
      return cached ?? Response.error();
    }),
  );
});
