# ADR-009: Push Notifications with Web Push and UnifiedPush

## Status

Accepted

## Context

People want to hear about new expenses and payments right away, not only by email. The usual way to push to phones is Firebase Cloud Messaging (FCM) on Android and the Apple Push Notification service (APNs) on iOS. Both tie every message to an account the app developer owns: with FCM, each self-hoster would need a Firebase project and its service account key, and the Android app would have to be built against that project. That contradicts the promise of Split Spends: one container plus PostgreSQL, no vendor accounts, and an app that works against any server.

The web app needs push as well, and browsers already have a standard for it.

## Decision

- The server speaks standard **Web Push**: RFC 8030 (delivery), RFC 8291 (message encryption, `aes128gcm`) and RFC 8292 (VAPID, which identifies the server with a key pair instead of an account). One code path in `packages/webpush` (on `github.com/SherClockHolmes/webpush-go`, MIT) sends to every device.
- **Browsers** subscribe with the Push API and the server's VAPID public key; their vendor runs the push service and no registration is needed. A small service worker (`apps/web/web/push_sw.js`) shows the notification.
- **Android** uses **UnifiedPush**: the user installs a distributor app of their choice (ntfy, NextPush, or one that relays through FCM if they want), and the app registers with it. Distributors deliver Web Push messages unchanged, so the server does not know or care which one is used. The connector library generates the encryption keys and decrypts messages; the app shows them natively.
- **VAPID keys** come from `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY`, or are generated on first start and stored in the database, so restarts and multiple instances share them without setup.
- **Delivery** is a second channel behind the existing `ports.Notifier`: a fan-out in `packages/bootstrap` sends email and push, so services still call `Notify` once. Each notification carries the recipient's email (for email) and user id (for push); `user.User.Notice` fills them from per-kind, per-channel preferences. Push uses its own bounded queue, drained by a few workers, and is sent inline in serverless mode, like email. A notification goes to all of the recipient's devices at once and each send stops after 5 seconds, so devices that answer slowly or never cost one timeout, not one per device, and do not hold up other people's notifications.
- The domain (`packages/core/domains/push`) owns the rules: subscriptions are unique per endpoint, a 404 or 410 from the push service removes the device, other failures (timeouts included) are counted and a device is removed after 25 in a row (registering it again with the same keys keeps the count), and each user has at most 25 devices, a cap the repository checks and writes atomically.
- **iOS** is out of scope for now: native push on iPhone requires APNs keys from a paid Apple developer account. The web app works on iPhone when added to the Home Screen (iOS 16.4 and later).

## Consequences

### Positive

- No Google or Apple account for self-hosters, no Firebase in the Android app, and no per-server app builds.
- One server implementation for browsers and Android; payloads are end-to-end encrypted, so push services and distributors cannot read them.
- Users choose who delivers their notifications.

### Negative

- Android users must install a distributor app once. The Account screen explains this and links to the list of distributors.
- Endpoints are URLs supplied by devices, so the server makes outgoing requests to them. The sender refuses private, loopback and metadata addresses by default and does not follow redirects; a LAN-only ntfy needs `PUSH_ALLOW_PRIVATE=true`.
- Changing the VAPID key invalidates existing subscriptions; people have to enable push again.

### Neutral

- Payloads repeat what the email says (`{ title, body, url }`), so push shows nothing an email would not.
