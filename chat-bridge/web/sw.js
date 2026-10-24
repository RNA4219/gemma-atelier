// The app runs entirely on this phone. Never cache chat responses or private images.
self.addEventListener('install',()=>self.skipWaiting());
self.addEventListener('activate',event=>event.waitUntil(self.clients.claim()));
