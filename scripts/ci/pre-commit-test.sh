#!/usr/bin/env bash
# Runs scripts/hooks/pre-commit in a scratch git repository for staged Go,
# Dart and shell files and checks what it accepts and refuses. Needs git,
# gofmt and dart on PATH.
# Usage: scripts/ci/pre-commit-test.sh
set -euo pipefail

hook=$(cd "$(dirname "$0")/../hooks" && pwd)/pre-commit
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
failures=0

# fresh makes an empty repository and enters it.
fresh() {
  rm -rf "$work/repo"
  mkdir "$work/repo"
  cd "$work/repo"
  git init -q
  git config user.email hook@example.com
  git config user.name Hook
  # A signing setup in the developer's own config would stop the commits.
  git config commit.gpgsign false
}

# expect runs the hook and checks its exit status (pass or fail) and that
# its output contains text, when given.
expect() {
  local want=$1 name=$2 text=${3:-} out status=0
  out=$("$hook" 2>&1) || status=$?
  if { [ "$want" = pass ] && [ "$status" -ne 0 ]; } || { [ "$want" = fail ] && [ "$status" -eq 0 ]; }; then
    echo "FAIL $name: want $want, exit $status: $out"
    failures=$((failures + 1))
  elif [ -n "$text" ] && [[ $out != *"$text"* ]]; then
    echo "FAIL $name: output lacks \"$text\": $out"
    failures=$((failures + 1))
  else
    echo "ok   $name"
  fi
}

formatted_go=$'package main\n\nfunc main() {}\n'
unformatted_go=$'package main\nfunc main(){}\n'
formatted_dart=$'void main() {}\n'
unformatted_dart=$'void main( ){ }\n'

fresh
printf '%s' "$unformatted_go" > main.go
git add main.go
printf '%s' "$formatted_go" > main.go
expect fail "unformatted Go staged, formatted working copy" "main.go"

fresh
printf '%s' "$formatted_go" > main.go
git add main.go
printf '%s' "$unformatted_go" > main.go
expect pass "formatted Go staged, unformatted working copy"

fresh
mkdir "two words"
printf '%s' "$unformatted_go" > "two words/main.go"
git add "two words/main.go"
expect fail "unformatted Go under a name with a space" "two words/main.go"

fresh
printf 'package main\nfunc main() {\n' > main.go
git add main.go
expect fail "Go that does not parse" "main.go"

fresh
printf '%s' "$unformatted_go" > main.go
git add main.go
git commit -q --no-verify -m start
git rm -q main.go
expect pass "deleted Go file"

fresh
printf '%s' "$unformatted_dart" > main.dart
git add main.dart
printf '%s' "$formatted_dart" > main.dart
expect fail "unformatted Dart staged, formatted working copy" "main.dart"

fresh
printf '%s' "$formatted_dart" > main.dart
git add main.dart
expect pass "formatted Dart"

fresh
mkdir -p app/build
printf '%s' "$unformatted_dart" > app/build/generated.dart
git add -f app/build/generated.dart
expect pass "Dart under a build directory"

fresh
printf '#!/bin/sh\necho   hi\n' > run.sh
git add run.sh
expect pass "shell script (shellcheck runs in make lint and CI)"

# Without the formatters on PATH the hook says so and lets the commit
# through; CI runs both checks.
fresh
printf '%s' "$unformatted_go" > main.go
printf '%s' "$unformatted_dart" > main.dart
git add main.go main.dart
mkdir "$work/bin"
for tool in git bash; do
  ln -sf "$(command -v "$tool")" "$work/bin/$tool"
done
PATH="$work/bin" expect pass "no gofmt or dart installed" "gofmt not found"
PATH="$work/bin" expect pass "no dart installed" "dart not found"

if [ "$failures" -ne 0 ]; then
  echo "$failures pre-commit case(s) failed"
  exit 1
fi
echo "pre-commit hook: all cases passed"
