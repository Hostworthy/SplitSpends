#!/usr/bin/env bash
# Runs the Android integration test on the connected emulator or device and
# prints the app's log when it fails, so a crash or hang is visible in CI.
# Usage, from apps/android: ../../scripts/ci/device-test.sh [flutter test args]
set -u
adb logcat -c || true
timeout 1200 flutter test integration_test --reporter expanded "$@"
code=$?
if [ "$code" -ne 0 ]; then
  [ "$code" -eq 124 ] && echo "device-test: timed out after 20 minutes"
  adb logcat -d > ../../device-logcat.txt 2>/dev/null || true
  echo "---- logcat (flutter, crashes, app) ----"
  grep -E "flutter|device-test|AndroidRuntime|FATAL|dev\.splitspends" ../../device-logcat.txt | tail -400
fi
exit "$code"
