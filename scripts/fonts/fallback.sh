#!/bin/sh
# Keeps apps/web/web/fonts/fallback in step with the Noto fallback fonts the
# Flutter web engine asks for (emoji and scripts outside the bundled Inter
# subset). The fonts are committed and served by our own server; the web app
# and the build never download them. apps/web/web/flutter_bootstrap.js points
# the engine's fontFallbackBaseUrl at that folder.
#
# Usage, from anywhere:
#   scripts/fonts/fallback.sh check   Offline. Fail if the committed fonts do
#                                     not match the font list of the Flutter
#                                     SDK on PATH (CI and make lint run this)
#   scripts/fonts/fallback.sh update  Maintainers only, after a Flutter
#                                     upgrade changes the list: download the
#                                     missing fonts once from Google Fonts,
#                                     where the engine's exact subsets are
#                                     published, and delete stale ones. Commit
#                                     the result.
set -eu

root=$(cd "$(dirname "$0")/../.." && pwd)
dir=$root/apps/web/web/fonts/fallback
source_url=https://fonts.gstatic.com/s

# Font paths relative to fontFallbackBaseUrl, from the engine's generated list.
expected() {
  command -v flutter >/dev/null || { echo "fallback: flutter is not on PATH" >&2; exit 1; }
  sdk=${FLUTTER_ROOT:-$(flutter --version --machine 2>/dev/null | sed -n 's/.*"flutterRoot": *"\([^"]*\)".*/\1/p')}
  data=$sdk/bin/cache/flutter_web_sdk/lib/_engine/engine/font_fallback_data.dart
  [ -f "$data" ] || flutter precache --web >/dev/null
  [ -f "$data" ] || { echo "fallback: $data not found" >&2; exit 1; }
  grep -o "'[a-z0-9]*/v[0-9]*/[^']*\.\(woff2\|ttf\|otf\)'" "$data" | tr -d "'" | sort -u
}

present() {
  (cd "$dir" && find . -type f ! -name OFL.txt | sed 's|^\./||' | sort)
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
expected > "$tmp/expected"
present > "$tmp/present"
comm -23 "$tmp/expected" "$tmp/present" > "$tmp/missing"
comm -13 "$tmp/expected" "$tmp/present" > "$tmp/stale"

case "${1:-check}" in
check)
  if [ -s "$tmp/missing" ] || [ -s "$tmp/stale" ]; then
    echo "fallback: apps/web/web/fonts/fallback does not match the Flutter SDK. Run scripts/fonts/fallback.sh update and commit." >&2
    sed 's/^/  missing /' "$tmp/missing" >&2
    sed 's/^/  stale   /' "$tmp/stale" >&2
    exit 1
  fi
  echo "fallback: $(wc -l < "$tmp/expected" | tr -d ' ') fonts match the Flutter SDK"
  ;;
update)
  while read -r path; do
    mkdir -p "$dir/$(dirname "$path")"
    curl -fsSL --retry 3 -o "$dir/$path.part" "$source_url/$path"
    mv "$dir/$path.part" "$dir/$path"
  done < "$tmp/missing"
  while read -r path; do rm -f "$dir/$path"; done < "$tmp/stale"
  find "$dir" -type d -empty -delete
  echo "fallback: added $(wc -l < "$tmp/missing" | tr -d ' '), removed $(wc -l < "$tmp/stale" | tr -d ' '). Review and commit apps/web/web/fonts/fallback."
  ;;
*)
  echo "usage: scripts/fonts/fallback.sh [check|update]" >&2
  exit 2
  ;;
esac
