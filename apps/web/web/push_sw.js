// Push notifications for the SplitSpends web app.
//
// The app registers this worker with the scope "push/" when you tap
// "Enable push notifications" in Account, so it never controls pages and
// stays out of the way of Flutter's own service worker. It shows each push
// message the server sends ({ title, body, url }) as a notification, focuses
// the app (or opens it at url) when you click one, and subscribes again when
// the browser replaces or drops the subscription. It has no sign-in token,
// so the app sends the new subscription to the server when it next starts.

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));

self.addEventListener('push', (event) => {
  let message = {};
  try {
    message = event.data ? event.data.json() : {};
  } catch (_) {
    message = { body: event.data ? event.data.text() : '' };
  }
  const title = message.title || 'Split Spends';
  event.waitUntil(
    self.registration.showNotification(title, {
      body: message.body || '',
      icon: new URL('../icons/Icon-192.png', self.registration.scope).href,
      badge: new URL('../icons/badge-96.png', self.registration.scope).href,
      data: { url: message.url || new URL('..', self.registration.scope).href },
    }),
  );
});

self.addEventListener('pushsubscriptionchange', (event) => {
  // Some browsers hand over the new subscription; others only the old one,
  // whose options (the server's key) subscribe again.
  if (event.newSubscription || !event.oldSubscription) return;
  event.waitUntil(
    self.registration.pushManager
      .subscribe(event.oldSubscription.options)
      .catch(() => {
        // Push stays off until it is enabled again in Account.
      }),
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const app = new URL('..', self.registration.scope);
  let target = app.href;
  try {
    const url = new URL(event.notification.data && event.notification.data.url, app);
    // Only ever open this app, never another site named in a message.
    if (url.origin === app.origin) target = url.href;
  } catch (_) {
    // Keep the app's start page.
  }
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windows) => {
      // An open app window is focused as it is: this worker does not control
      // pages, so it cannot navigate them.
      for (const client of windows) {
        if (new URL(client.url).origin === app.origin && 'focus' in client) {
          return client.focus();
        }
      }
      return self.clients.openWindow(target);
    }),
  );
});
