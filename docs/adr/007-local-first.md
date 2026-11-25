# ADR-007: Local-First App with Optional Server

## Status

Accepted

## Context

Many people want to split costs without creating an account or running a server. An account should add sync, sharing and email, not be the price of entry.

## Decision

- The app has two modes. **Local-only**: data lives on the device (a pure-Dart store backed by IndexedDB on the web and files on mobile), people are plain contacts, and the split and balance rules run in Dart. **Connected**: the server is the source of truth; the app caches data for offline reading and queues changes in an outbox.
- The Dart split and balance code mirrors `packages/core` and is tested against the same examples, so both modes agree to the minor unit.
- The on-device database carries a schema version (`schemaVersion` in `packages/client/lib/core/storage/local_db.dart`). A release that changes a stored record shape raises it and adds a migration step there, so existing installs are upgraded when the database opens. A database written by a newer release is refused with a message, and if the data cannot be opened the app says so and offers Try again and Save backup instead of staying on the splash screen.
- Every object the device creates gets a UUID on the device. The server accepts these ids, and repeating a create with the same id returns the existing object, so the outbox can retry safely.
- Moving from local-only to an account uses `POST /api/v1/import`: one transaction, no emails. Contacts with an email resolve to existing or invited users; contacts without one become `placeholder` people who keep their device id and can be given an email later.
- Onboarding presents both modes as equal choices and says plainly what an account is for. No dark patterns: no nagging, no pre-selected sign-up, no hidden skip.

## Consequences

### Positive

- Works with zero infrastructure; a server becomes an upgrade, not a requirement.

### Negative

- Split and balance rules exist in Go and Dart; shared test vectors keep them aligned.
- Conflict handling is simple: the server wins, and offline edits are replayed in order.
- The outbox belongs to one account on one server. Writes of another account stay on the device, unsent, until that account signs in again. A sign-out the user asks for discards the account's writes after a warning; a session the server ends (401) keeps them. Only the API's own verdicts (validation, conflict, not found, forbidden) drop a write; proxy answers, 408, 429 and outages keep it, and a write the server keeps failing on is set aside for Retry or Discard so later writes still sync.
