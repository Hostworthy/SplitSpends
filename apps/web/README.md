# Split Spends for the Web

A thin shell around [`packages/client`](../../packages/client/README.md): this folder holds only the web runner (`web/index.html`, manifest, icons) and a two-line `lib/main.dart`.

```bash
flutter pub get                                   # once, at the repository root
make web                                          # dev server on :5173 against the API on :8080
make web-build                                    # release build into apps/web/build/web
```

The API server (`apps/api`) serves the release build when `WEB_DIR` points at it; the Docker image does this automatically. The web app makes no third-party requests: everything it loads comes from the server that hosts it.

- `--no-web-resources-cdn` bundles the engine (CanvasKit) and its default Roboto font instead of loading them from Google.
- Inter is an asset of `packages/client`.
- The Flutter engine's fallback fonts, Noto Color Emoji and Noto Sans for other scripts (CJK, Devanagari, Arabic, symbols and more), are committed in `web/fonts/fallback/`. `web/flutter_bootstrap.js` sets `fontFallbackBaseUrl` to that folder. They take about 36 MB in 727 files, 26 MB of it CJK, but a browser downloads only the small subsets it needs for the text on screen. They are under the SIL Open Font License 1.1 (`web/fonts/fallback/OFL.txt`).

After upgrading Flutter, run `scripts/fonts/fallback.sh check` (CI and `make lint` run it too). If the engine's font list changed, `scripts/fonts/fallback.sh update` fetches the new subsets once from Google Fonts, the only place they are published, and deletes stale ones; commit the result. Builds and the running app never download fonts.
