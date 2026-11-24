# SplitSpends for the Web

A thin shell around [`packages/client`](../../packages/client/README.md): this folder holds only the web runner (`web/index.html`, manifest, icons) and a two-line `lib/main.dart`.

```bash
flutter pub get                                   # once, at the repository root
make web                                          # dev server on :5173 against the API on :8080
make web-build                                    # release build into apps/web/build/web
```

The API server (`apps/api`) serves the release build when `WEB_DIR` points at it; the Docker image does this automatically. The build bundles its engine and fonts, so it makes no third-party requests except the emoji font noted in [docs/HOSTING.md](../../docs/HOSTING.md).
