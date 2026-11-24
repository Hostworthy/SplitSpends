# SplitSpends App

One Flutter codebase for **iOS, Android and the web**. It works in two modes, chosen on the first screen:

| Mode | What it means |
| ---- | ------------- |
| Use on this device | No account. Data stays on the device. |
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
    models/                 User, Me, Balance, Debt, JsonMap
    money/                  Money.format / parse / toPlain (integers in minor units only)
    paging/                 PageResult, PagedNotifier (infinite scroll)
    utils/                  dates, web download helper
    widgets/                avatars, balance text, amount field, pickers, skeletons, states
  features/<feature>/
    domain/                 models + abstract repository interfaces (no HTTP, no storage)
    data/                   implementations (remote_* = HTTP; local ones come next)
    presentation/           screens, widgets, Riverpod providers
```

Features: `auth` (welcome, on-device setup, server sign-in), `dashboard`, `groups` (list, detail with expenses / balances / stats tabs, settings), `friends`, `expenses` (editor with all split methods, detail with comments and receipt, settle up), `activity`, `account` (profile, preferences, API tokens for MCP), `receipts` (AI scan, AI quick add).

### Rules

- Screens and providers depend on the **domain interfaces** only. `app/repositories.dart` is the single place that picks the implementation for the current mode.
- Money is always an `int` in minor units plus a currency code. Doubles are used only to display charts and formatted text.
- `domain/split_calculator.dart` mirrors the server's split rules for live previews; the server stays authoritative.
- State: `flutter_riverpod` (no code generation). Forms use a `ChangeNotifier` (`ExpenseForm`) so their rules are unit tested without widgets.
