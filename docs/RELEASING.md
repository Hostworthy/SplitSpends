# Releasing

Releases are built by `.github/workflows/release.yml` when a version tag is pushed. A tag with a suffix (`v0.1.0-alpha.1`, `v1.2.0-rc.1`) becomes a pre-release; a plain tag (`v1.2.0`) becomes a normal release and also moves the `latest` container tag.

```bash
git checkout main && git pull
git tag v0.1.0-alpha.1
git push origin v0.1.0-alpha.1
```

The workflow first checks that the tag names a version and points to a commit on `main`, then runs the full CI workflow (`ci.yml`) on that commit; nothing is built or published unless both pass. Runs for the same tag queue one behind the other instead of racing.

To stop anyone tagging by mistake, add a tag ruleset for `v*` (Settings, Rules, Rulesets) that restricts creating, updating and deleting matching tags to maintainers.

| Artifact | Built on | Notes |
| --- | --- | --- |
| Server and MCP binaries for Linux (amd64, arm64), macOS (amd64, arm64) and Windows (amd64) | Ubuntu | `main.version` is the tag, so `splitspends version` prints it; `THIRD_PARTY_NOTICES.txt` holds the licences of the Go modules linked in (`scripts/release/notices.sh`) |
| Web app (`splitspends-web-<tag>.tar.gz`) | Ubuntu | Serve with `WEB_DIR` |
| Android APK | Ubuntu | Signed with your upload key when the secrets below exist; otherwise named `-debug-signed.apk` and signed with a key the runner generates for that run |
| iOS app (`-unsigned.ipa`) | macOS | Unsigned until an Apple Developer account is set up; install with Xcode, AltStore or Sideloadly |
| Container image `ghcr.io/hostworthy/splitspends:<tag>` | Ubuntu | linux/amd64; includes the server and the web app, with the third-party notices in `/usr/share/doc/splitspends`; its `HEALTHCHECK` runs `splitspends health`, which exits 1 unless `/health` answers 200 |
| `SHA256SUMS` | | Checksums of every file |

Every file and the image also get a signed build provenance attestation, and the image carries a software bill of materials. The release notes print the image digest and the commands to check them; checksums alone only catch a damaged download, because they are published next to the files.

```bash
gh attestation verify splitspends-server-v0.1.0-linux-amd64.tar.gz -R Hostworthy/SplitSpends
gh attestation verify oci://ghcr.io/hostworthy/splitspends:v0.1.0 -R Hostworthy/SplitSpends
```

App versions use the tag without the `v` and the suffix (`0.1.0`) and the workflow run number as the build number.

## Redoing a Release

If a run fails before the release is published, fix the workflow on `main`, then move the tag to the fixed commit and push it again; the workflow runs from scratch and replaces any files already attached to that release.

```bash
git checkout main && git pull
git tag -f -a v0.1.0-alpha.1 -m "Split Spends v0.1.0-alpha.1"
git push -f origin v0.1.0-alpha.1
```

Never move a tag once its release is published, even if nobody seems to have downloaded it yet: people and their scripts expect a version to name one build. Publish the next version (`v0.1.0-alpha.2`) instead.

## Workflow Actions

Every `uses:` in `.github/workflows` is pinned to a full commit SHA with the version in a trailing comment, because a tag can be moved to other code after review. Dependabot (`.github/dependabot.yml`) proposes updates as pull requests; review the action's changes before merging. When adding an action, pin it the same way, taking the SHA from the action's own repository:

```bash
git ls-remote https://github.com/actions/checkout 'refs/tags/v4*'
```

For an annotated tag use the commit of the `^{}` entry. In the repository settings (Actions, General) you can also require actions to be pinned to a full-length commit SHA.

## Android Signing Secrets (Optional)

Add these repository secrets to sign the APK with your upload key (see [apps/apk/README.md](../apps/apk/README.md)):

| Secret | Value |
| --- | --- |
| `ANDROID_KEYSTORE_BASE64` | `base64 -w0 upload-keystore.jks` |
| `ANDROID_STORE_PASSWORD` | Keystore password |
| `ANDROID_KEY_ALIAS` | Key alias, for example `upload` |
| `ANDROID_KEY_PASSWORD` | Key password |

Keep the same key for every release: Android only installs updates signed with the key of the installed app. Without these secrets the workflow still publishes the APK, as `splitspends-android-<tag>-debug-signed.apk` with a warning in the run and a line in the release notes, but every release is signed with a different debug key, so people must uninstall the previous version (losing the app's local data) before installing the next. Add the secrets before the first release people will keep using.
