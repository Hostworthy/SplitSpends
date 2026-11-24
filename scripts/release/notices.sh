#!/bin/sh
# Writes the licence texts of the Go standard library and of every module
# linked into the server and MCP binaries. Their MIT, BSD and Apache licences
# ask for these notices to travel with the binaries, so the release archives
# and the container image include the file.
#
# Usage, from the repository root (GOOS and GOARCH select the platform):
#   scripts/release/notices.sh dist/THIRD_PARTY_NOTICES.txt
set -eu

out=${1:?usage: scripts/release/notices.sh OUTPUT}

# Prints every licence, notice and patent file at the top of a module.
section() {
	files=$(find "$3" -maxdepth 1 -type f \( -iname 'licen[cs]e*' -o -iname 'copying*' \
		-o -iname 'notice*' -o -iname 'patents*' \) | sort)
	if [ -z "$files" ]; then
		echo "notices: no licence file for $1 $2 in $3" >&2
		return 1
	fi
	printf '%s\n%s %s\n%s\n' "================================================================" "$1" "$2" \
		"================================================================"
	while read -r f; do
		printf '\n%s\n\n' "--- $(basename "$f") ---"
		cat "$f"
	done <<FILES
$files
FILES
	printf '\n'
}

modules=$(go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' \
	./apps/api ./apps/mcp | sort -u)

{
	printf 'Split Spends includes the Go standard library and the modules below.\n'
	printf 'Each is distributed under its own licence, reproduced here.\n\n'
	section "Go standard library" "$(go env GOVERSION)" "$(go env GOROOT)"
	while read -r path version dir; do
		section "$path" "$version" "$dir"
	done <<MODULES
$modules
MODULES
} >"$out"
