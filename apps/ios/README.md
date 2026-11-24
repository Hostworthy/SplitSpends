# SplitSpends for iOS

A thin shell around [`packages/client`](../../packages/client/README.md): this folder holds only the iOS runner and a two-line `lib/main.dart`.

Requires macOS with Xcode.

```bash
flutter pub get                 # once, at the repository root
cd apps/ios
open -a Simulator && flutter run
flutter build ipa               # needs an Apple developer team for signing
```

Set your team and bundle identifier (`dev.splitspends.app` by default) in Xcode under Runner, Signing & Capabilities. Servers on the local network may use plain HTTP; anywhere else use HTTPS.
