#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Self-test of test/release/inspect.sh (SEC-074, DEV-050), run by
# `make release-inspect-selftest`; it needs no SDK. The scanner runs on
#   - fixtures/leaky.txt as the AOT snapshot of a build: it must fail and name
#     a P10 symbol and the devtools canary;
#   - fixtures/clean.txt: it must pass;
#   - every probe of probes.txt planted alone in a clean snapshot: each must
#     fail, so that no probe is dead.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# snapshot <name> <text-file>: lays out the tree of an unpacked release APK
# whose libapp.so is the file; prints the tree's path.
snapshot() {
	local dir=$tmp/$1
	mkdir -p "$dir/lib/arm64-v8a"
	cp "$2" "$dir/lib/arm64-v8a/libapp.so"
	echo "$dir"
}

failures=0
expect() { # expect <description> <condition status>
	if [ "$2" -ne 0 ]; then
		echo "FAIL selftest: $1"
		failures=$((failures + 1))
	fi
}

out=$(bash "$here/inspect.sh" "$(snapshot leaky "$here/fixtures/leaky.txt")" 2>&1) && rc=0 || rc=$?
expect "a snapshot with dev symbols must fail (exit $rc)" "$((rc != 1))"
grep -qF 'p10 string "DevChannel"' <<<"$out" && rc=0 || rc=$?
expect "the leaky snapshot must name DevChannel" "$rc"
grep -qF 'canary string "plux-devtools-canary-7f3a"' <<<"$out" && rc=0 || rc=$?
expect "the leaky snapshot must name the devtools canary" "$rc"

out=$(bash "$here/inspect.sh" "$(snapshot clean "$here/fixtures/clean.txt")" 2>&1) && rc=0 || rc=$?
expect "a clean snapshot must pass (exit $rc)" "$rc"

n=0
while IFS=$'\t' read -r kind probe; do
	case "$kind" in '' | '#'*) continue ;; esac
	n=$((n + 1))
	printf 'PluxView %s more\n' "$probe" >"$tmp/plant.txt"
	rc=0
	bash "$here/inspect.sh" "$(snapshot "plant$n" "$tmp/plant.txt")" >/dev/null 2>&1 || rc=$?
	expect "$kind probe \"$probe\" planted alone must fail (exit $rc)" "$((rc != 1))"
done <"$here/probes.txt"

if [ "$failures" -ne 0 ]; then
	exit 1
fi
echo "ok   inspect.sh: leaky fixture fails, clean fixture passes, all $n probes fire"
