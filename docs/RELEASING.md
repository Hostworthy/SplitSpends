# Releasing

Releases are built by `.github/workflows/release.yml` when a version tag is pushed. A tag with a suffix (`v0.1.0-alpha.1`, `v1.2.0-rc.1`) becomes a pre-release; a plain tag (`v1.2.0`) becomes a normal release and also moves the `latest` container tag.

```bash
git checkout main && git pull
git tag v0.1.0-alpha.1
git push origin v0.1.0-alpha.1
```

| Artifact | Built on | Notes |
| --- | --- | --- |
| Server and MCP binaries for Linux (amd64, arm64), macOS (amd64, arm64) and Windows (amd64) | Ubuntu | `main.version` is the tag, so `splitspends version` prints it |
| Web app (`splitspends-web-<tag>.tar.gz`) | Ubuntu | Serve with `WEB_DIR` |
| Android APK | Ubuntu | Signed with your upload key when the secrets below exist, otherwise with the debug key |
| iOS app (`-unsigned.ipa`) | macOS | Unsigned until an Apple Developer account is set up; install with Xcode, AltStore or Sideloadly |
| Container image `ghcr.io/hostworthy/splitspends:<tag>` | Ubuntu | linux/amd64; includes the server and the web app |
| `SHA256SUMS` | | Checksums of every file |

App versions use the tag without the `v` and the suffix (`0.1.0`) and the workflow run number as the build number.

## Android Signing Secrets (Optional)

Add these repository secrets to sign the APK with your upload key (see [apps/apk/README.md](../apps/apk/README.md)):

| Secret | Value |
| --- | --- |
| `ANDROID_KEYSTORE_BASE64` | `base64 -w0 upload-keystore.jks` |
| `ANDROID_STORE_PASSWORD` | Keystore password |
| `ANDROID_KEY_ALIAS` | Key alias, for example `upload` |
| `ANDROID_KEY_PASSWORD` | Key password |

Keep the same key for every release: Android only installs updates signed with the key of the installed app.
