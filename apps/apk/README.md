# Split Spends for Android

A thin shell around [`packages/client`](../../packages/client/README.md): this folder holds only the Android runner and a two-line `lib/main.dart`.

```bash
flutter pub get                 # once, at the repository root
cd apps/apk
flutter run                     # emulator or connected device
flutter build apk --release     # or: make apk (repository root)
flutter build appbundle         # for Google Play
```

The APK lands in `build/app/outputs/flutter-apk/app-release.apk`; install it with `adb install` or by opening it on the phone.

The CI workflow, weekly or started by hand from the Actions tab, builds the release APK and runs the app on an Android emulator; download the APK from the `splitspends-apk` artifact of the CI run. See [Testing](../../docs/DEV.md#testing) to run the device test yourself.

## Release Signing

Without a key, release builds are signed with the debug key: fine for installing and testing, not for Google Play. A build signed with a different key cannot update the installed app, so publish only APKs signed with one upload key (see [Releasing](../../docs/RELEASING.md#android-signing-secrets-optional)). To sign with your upload key, create `android/key.properties` (it is git-ignored):

```properties
storeFile=/absolute/path/to/upload-keystore.jks
storePassword=...
keyAlias=upload
keyPassword=...
```

Create a keystore with `keytool -genkey -v -keystore upload-keystore.jks -keyalg RSA -keysize 2048 -validity 10000 -alias upload`. Never commit it.

## Network

Servers on your local network may use plain HTTP (`http://192.168.1.20:8080`, `http://nas.local:8080`); the app warns that such a connection is not encrypted. For a plain HTTP address outside the local network the app asks before sending your sign-in code, because the code and the session token would travel unencrypted. The app checks this itself: its HTTP client is Dart's, which Android's network security config does not govern. For the same reason it trusts only the certificate authorities that ship with Android, not ones you install on the phone, so a server reached over HTTPS needs a certificate from a public authority (for example Let's Encrypt). The Android emulator reaches a server on your computer at `http://10.0.2.2:8080`.

## Backup

Android Auto Backup is on, so the on-device ledger moves to a new phone with your Google backup. The backup also holds the app's offline copy of server data from connected mode. The session token is encrypted with an Android Keystore key, which is not backed up, so a restored app cannot read it and asks you to sign in again.
