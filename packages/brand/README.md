# Brand

The Split Spends mark is two overlapping coins, a white one and a mint one, on a teal gradient. It matches the in-app logo (`AppLogo` in `packages/client/lib/app/shell.dart`).

| Colour | Hex | Use |
| --- | --- | --- |
| Brand teal | `#0E7C66` | Theme seed, Android icon background, splash |
| Gradient | `#12907A` to `#0A5E4E` | Icon and banner background |
| Coin | `#FFFFFF`, ring `#D6EFE6` | Left coin |
| Mint coin | `#9DEBCF`, ring `#6FD3B2` | Right coin |
| Light background | `#F5FAF7` | Web page and light theme surface |

`icon.svg` is the vector master (also served as the web favicon). Every bitmap comes from `scripts/brand/render.py`, which draws the same geometry with Pillow:

```bash
python3 scripts/brand/render.py
```

It writes the web favicon and PWA icons (`apps/web/web`), Android launcher, adaptive, monochrome and splash images (`apps/apk/android/app/src/main/res`), the iOS app icon and launch image (`apps/ios/ios/Runner/Assets.xcassets`), the social preview banner (`docs/images/banner.png`, also `apps/web/web/social.png`) and the Google Play feature graphic (`docs/images/feature-graphic.png`). Commit the results.
