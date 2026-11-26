// A versão é calculada pelo servidor a partir dos arquivos embutidos.
const CACHE_PREFIX = 'reembolso-pwa-';
const CACHE_NAME = CACHE_PREFIX + '__CACHE_VERSION__';
const OFFLINE_URL = '/offline.html';
const PUBLIC_FILES = [OFFLINE_URL, '/icon-192.png', '/icon-512.png', '/icon-maskable-512.png', '/apple-touch-icon.png'];

self.addEventListener('install', event => {
  event.waitUntil((async () => {
    const cache = await caches.open(CACHE_NAME);
    await cache.addAll(PUBLIC_FILES.map(url => new Request(url, {cache: 'reload', credentials: 'omit'})));
  })());
  // Uma atualização espera pelo consentimento na interface ou pelo fechamento das abas.
});
self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    const names = await caches.keys();
    await Promise.all(names.filter(name => name.startsWith(CACHE_PREFIX) && name !== CACHE_NAME).map(name => caches.delete(name)));
    await self.clients.claim();
  })());
});
self.addEventListener('message', event => {
  if (event.data?.type === 'ACTIVATE_UPDATE') event.waitUntil(self.skipWaiting());
});
self.addEventListener('fetch', event => {
  const request = event.request;
  const url = new URL(request.url);
  // APIs, notas e escritas passam sempre pela rede; não são armazenadas ou reenviadas.
  if (request.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api/')) return;
  if (request.mode === 'navigate') {
    event.respondWith(fetch(request).catch(() => caches.open(CACHE_NAME).then(cache => cache.match(OFFLINE_URL))));
  } else if (PUBLIC_FILES.includes(url.pathname)) {
    event.respondWith(fetch(request).catch(() => caches.open(CACHE_NAME).then(cache => cache.match(url.pathname))));
  }
});
