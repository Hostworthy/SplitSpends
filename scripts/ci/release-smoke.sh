#!/usr/bin/env bash
# Installs a release APK on the connected emulator, starts it and checks it
# is still running a while later. The integration test runs a debug build,
# so this is the only run of what R8 shrinking and release signing produce.
# Usage: scripts/ci/release-smoke.sh path/to/app-release.apk
set -u
apk=${1:?usage: release-smoke.sh path/to/app-release.apk}
app=dev.splitspends.app

adb logcat -c || true
# The integration test leaves a debug build signed with another key.
adb uninstall "$app" > /dev/null 2>&1 || true
adb install "$apk" || exit 1
adb shell am start -W -n "$app/.MainActivity" || exit 1

# A crash on start (a class R8 removed, a broken plugin) ends the process
# within seconds, so it has to stay up for the whole window.
alive=0
for _ in $(seq 1 20); do
  if adb shell pidof "$app" > /dev/null; then
    alive=$((alive + 1))
  else
    alive=0
    break
  fi
  sleep 1
done

adb logcat -d > release-logcat.txt 2>/dev/null || true
if [ "$alive" -lt 20 ] || grep -q "FATAL EXCEPTION" release-logcat.txt; then
  echo "release-smoke: $app did not keep running"
  grep -E "flutter|AndroidRuntime|FATAL|$app" release-logcat.txt | tail -200
  exit 1
fi
echo "release-smoke: $app started and kept running"
