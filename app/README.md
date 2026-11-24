# SplitSpends App

One Flutter codebase for **iOS, Android and the web**. It works in two modes, chosen on the first screen:

| Mode | What it means |
| ---- | ------------- |
| Use on this device | No account. Data stays on the device (IndexedDB in the browser, a file on phones). Backups export to a JSON file. |
| Connect to a server | Email sign-in (a 6-digit code, no password) to a SplitSpends server that you or a friend hosts. Needed only to sync between devices, share with others and get emails. |

The HTTP contract the app talks to is [docs/API.md](../docs/API.md).

## Requirements

- Flutter stable 3.50+ (Dart 3.14+)
- Xcode for iOS, Android Studio / SDK for Android

## Run

```bash
cd app
flutter pub get

# Web (Chrome). When the backend serves the built app, the server defaults
# to the page origin, so nothing needs configuring.
flutter run -d chrome

# iOS simulator
open -a Simulator
flutter run -d ios

# Android emulator or device
flutter run -d android
```

To connect, pick **Connect to a server** and enter the server address (for example `https://splitspends.example.com`). On a phone talking to a backend on your computer, use your computer's LAN address (`http://192.168.1.20:8080`); the Android emulator reaches the host as `http://10.0.2.2:8080`.

## Build

```bash
flutter build web --release        # output: build/web (served by the backend)
flutter build ipa                  # iOS, needs signing
flutter build appbundle            # Android
```

The web build uses clean path URLs (`/groups/<id>`). Whatever serves `build/web` must fall back to `index.html` for unknown non-API paths.

## Checks

```bash
dart format --output=none --set-exit-if-changed lib test
flutter analyze
flutter test
```

`flutter test` includes `test/core/ledger/vectors_test.dart`, which runs `../spec/vectors.json` (shared with the Go tests) against the Dart port of the split and balance rules.

An opt-in end-to-end test imports on-device data into a real server and checks that the server's balances match the device's exactly:

```bash
SPLITSPENDS_E2E_URL=http://localhost:8080 \
SPLITSPENDS_E2E_LOG=/path/to/server.log \
flutter test test/e2e
```

The log path is where the server writes emails when SMTP is not configured (the test reads the sign-in code from it).

## Architecture

Feature-first with clean layering, mirroring the backend's domain design:

```text
lib/
  main.dart                 ProviderScope + app
  app/                      app.dart, router.dart (redirects, deep links), shell.dart
                            (bottom bar on phones, rail + two panes ≥ 700px), theme.dart,
                            routes.dart, refresh.dart, repositories.dart (composition root)
  core/
    api/                    ApiClient (bearer token, envelopes, error envelope), ApiError
    auth/                   Session + AppMode (unset / local / connected), LocalStore
    config/                 AppConfig, server config provider, bundled local catalog
    ledger/                 pure-Dart port of the backend rules: Normalize (every split
                            method), Allocate, Nets, Pairwise, Simplify, sheets, recurrence
    models/                 User, Me, Balance, Debt, JsonMap
    money/                  Money.format / parse / toPlain (integers in minor units only)
    paging/                 PageResult, PagedNotifier (infinite scroll)
    storage/                sembast database, LocalLedger (on-device records), response cache
    sync/                   Outbox (offline writes), OutboxSync, reachability
    utils/                  dates, web download helper
    widgets/                avatars, balance text, amount field, pickers, skeletons, states
  features/<feature>/
    domain/                 models + abstract repository interfaces (no HTTP, no storage)
    data/                   local_* (on-device), remote_* (HTTP), offline_* (outbox wrapper)
    presentation/           screens, widgets, Riverpod providers
```

Features: `auth` (welcome, on-device setup, server sign-in), `sync` (import to an account, backups), `dashboard`, `groups` (list, detail with expenses / balances / stats tabs, settings), `friends`, `expenses` (editor with all split methods, detail with comments and receipt, settle up), `activity`, `account` (profile, preferences, API tokens for MCP), `receipts` (AI scan, AI quick add).

### Modes and Sync

- **On-device**: every repository has a `local_*` implementation over sembast. Records use the API's shapes; balances, debts, stats and recurring expenses are computed with `core/ledger`, so results match the server exactly. Comments, receipts, AI and API tokens need a server and say so.
- **Connecting later**: signing in from on-device mode offers to upload everything (`POST /api/v1/import`), then rewrites local ids with the server's people map. The local copy stays until removed in Account.
- **Connected and offline**: GET responses are cached for offline reading. Creating, editing and deleting expenses, recording payments and creating groups queue in a persisted outbox with client ids and replay in order; the server wins when it rejects a change. A bar shows "N changes waiting to sync".

### Rules

- Screens and providers depend on the **domain interfaces** only. `app/repositories.dart` is the single place that picks the implementation for the current mode.
- Money is always an `int` in minor units plus a currency code. Doubles are used only to display charts and formatted text.
- `domain/split_calculator.dart` mirrors the server's split rules for live previews; the server stays authoritative.
- State: `flutter_riverpod` (no code generation). Forms use a `ChangeNotifier` (`ExpenseForm`) so their rules are unit tested without widgets.
