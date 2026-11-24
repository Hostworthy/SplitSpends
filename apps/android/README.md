# SplitSpends for Android

A thin shell around [`packages/client`](../../packages/client/README.md): this folder holds only the Android runner and a two-line `lib/main.dart`.

```bash
flutter pub get                 # once, at the repository root
cd apps/android
flutter run                     # emulator or connected device
flutter build apk --release     # or: make apk (repository root)
flutter build appbundle         # for Google Play
```

The APK lands in `build/app/outputs/flutter-apk/app-release.apk`; install it with `adb install` or by opening it on the phone.

## Release Signing

Without a key, release builds are signed with the debug key: fine for installing and testing, not for Google Play. To sign with your upload key, create `android/key.properties` (it is git-ignored):

```properties
storeFile=/absolute/path/to/upload-keystore.jks
storePassword=...
keyAlias=upload
keyPassword=...
```

Create a keystore with `keytool -genkey -v -keystore upload-keystore.jks -keyalg RSA -keysize 2048 -validity 10000 -alias upload`. Never commit it.

## Network

Servers on your local network may use plain HTTP (`http://192.168.1.20:8080`); the app warns that such a connection is not encrypted. Certificates you install on the phone are trusted, so a homelab with its own certificate authority works over HTTPS. The Android emulator reaches a server on your computer at `http://10.0.2.2:8080`.
