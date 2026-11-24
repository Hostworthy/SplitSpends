#!/bin/sh
# Downloads the Noto fallback fonts the Flutter web engine loads for emoji
# and scripts outside the bundled Inter subset, so the web app can serve them
# itself instead of the browser fetching them from fonts.gstatic.com.
# web/flutter_bootstrap.js points the engine's fontFallbackBaseUrl here.
#
# Usage, from app/:
#   tool/fallback_fonts.sh          Download and verify into web/fonts/fallback
#   tool/fallback_fonts.sh --check  Fail if the lock file does not match the
#                                   font list of the Flutter SDK on PATH
#   tool/fallback_fonts.sh --lock   Rewrite the lock file from that SDK
#
# FALLBACK_FONTS picks what to download: "all" (default, about 36 MB, every
# script the engine knows) or space-separated font directories from the lock
# file, for example "notocoloremoji" (about 2 MB, emoji only). Characters with
# no downloaded font render as empty boxes.
set -eu

self=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
cd "$(dirname "$0")/.."
lock=tool/fallback_fonts.lock
out=web/fonts/fallback
source_url=https://fonts.gstatic.com/s
fonts=${FALLBACK_FONTS:-all}

if command -v sha256sum >/dev/null 2>&1; then
  sha() { sha256sum "$1" | cut -d' ' -f1; }
else
  sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi

# Font paths, relative to the base URL, from the engine's generated list.
sdk_paths() {
  command -v flutter >/dev/null || { echo "fallback_fonts: flutter is not on PATH" >&2; exit 1; }
  root=${FLUTTER_ROOT:-$(flutter --version --machine | sed -n 's/.*"flutterRoot": *"\([^"]*\)".*/\1/p')}
  data=$root/bin/cache/flutter_web_sdk/lib/_engine/engine/font_fallback_data.dart
  if [ ! -f "$data" ]; then
    flutter precache --web >/dev/null
  fi
  [ -f "$data" ] || { echo "fallback_fonts: $data not found" >&2; exit 1; }
  grep -o "'[a-z0-9]*/v[0-9]*/[^']*\.\(woff2\|ttf\|otf\)'" "$data" | tr -d "'" | sort -u
}

fetch() {
  mkdir -p "$(dirname "$2")"
  curl -fsSL --retry 3 --retry-delay 2 -o "$2.part" "$source_url/$1"
  mv "$2.part" "$2"
}

case "${1:-}" in
--check)
  missing=$(sdk_paths | while read -r path; do
    grep -q " $path\$" "$lock" || echo "$path"
  done)
  if [ -n "$missing" ]; then
    echo "fallback_fonts: $lock does not match the Flutter SDK. Run tool/fallback_fonts.sh --lock. Missing:" >&2
    echo "$missing" >&2
    exit 1
  fi
  echo "fallback_fonts: $lock matches the Flutter SDK"
  ;;
--lock)
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  sdk_paths > "$tmp/paths"
  echo "fallback_fonts: hashing $(wc -l < "$tmp/paths" | tr -d ' ') fonts"
  while read -r path; do
    fetch "$path" "$tmp/font"
    echo "$(sha "$tmp/font") $path"
  done < "$tmp/paths" > "$tmp/lock"
  mv "$tmp/lock" "$lock"
  echo "fallback_fonts: wrote $lock"
  ;;
"")
  if [ "$fonts" = all ]; then
    entries=$(cat "$lock")
  else
    entries=$(for dir in $fonts; do
      grep " $dir/" "$lock" || { echo "fallback_fonts: no fonts named $dir in $lock" >&2; exit 1; }
    done)
  fi
  count=$(echo "$entries" | wc -l | tr -d ' ')
  echo "$entries" | xargs -n 2 -P 8 "$self" --one
  # Remove fonts that are no longer selected or no longer in the lock file.
  wanted=$(echo "$entries" | cut -d' ' -f2)
  (cd "$out" && find . -type f ! -name OFL.txt | sed 's|^\./||') | while read -r path; do
    echo "$wanted" | grep -qxF "$path" || rm -f "$out/$path"
  done
  find "$out" -type d -empty -delete
  echo "fallback_fonts: $count fonts ready in $out"
  ;;
--one)
  # Internal: download and verify one font ($2 hash, $3 path).
  file=$out/$3
  if [ -f "$file" ] && [ "$(sha "$file")" = "$2" ]; then
    exit 0
  fi
  fetch "$3" "$file"
  actual=$(sha "$file")
  if [ "$actual" != "$2" ]; then
    rm -f "$file"
    echo "fallback_fonts: checksum mismatch for $3 (got $actual, want $2)" >&2
    exit 1
  fi
  ;;
*)
  echo "usage: tool/fallback_fonts.sh [--check|--lock]" >&2
  exit 2
  ;;
esac
