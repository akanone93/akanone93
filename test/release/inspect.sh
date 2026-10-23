#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Release-build inspection (SEC-074, DEV-050), run by `make release-inspect`.
# See test/release/README.md.
#
#   inspect.sh <apk|aab|ipa|zip|directory>...   inspect built artifacts
#   inspect.sh --build android|ios              build the starter app (which
#                                               wraps itself in PluxDevtools)
#                                               and Plux Bank for release,
#                                               then inspect them
#
# A release build must contain
#   - an AOT snapshot (libapp.so, or App.framework/App): a build without
#     one is a debug or JIT build, or not a build at all, and fails;
#   - none of the probe strings of test/release/probes.txt in that snapshot
#     (plux_devtools code, dev-only Plux diagnostics, the P10 dev session's
#     class names, socket path and ticket header, and the devtools canary
#     constant that only a build with kPluxDevtools contains);
#   - no kernel or JIT snapshot (kernel_blob.bin, *.dill, vm_snapshot_data,
#     isolate_snapshot_data), no source maps (*.map) and no split-debug-info
#     symbol files (*.symbols).
# Every finding is printed; the exit status is 1 when there is one, 2 for a
# usage error.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
probes=${PLUX_RELEASE_PROBES:-$here/probes.txt}

# inspect <label> <directory>: prints the findings for one extracted build.
inspect() {
	local label=$1 dir=$2 failed=0 snapshots=0 f kind probe
	while IFS= read -r f; do
		snapshots=$((snapshots + 1))
		while IFS=$'\t' read -r kind probe; do
			case "$kind" in '' | '#'*) continue ;; esac
			if grep -aqF -- "$probe" "$f"; then
				echo "FAIL $label: $kind string \"$probe\" in ${f#"$dir"/}"
				failed=1
			fi
		done <"$probes"
	done < <(find "$dir" -type f \( -name libapp.so -o -path '*/App.framework/App' \) | sort)
	if [ "$snapshots" -eq 0 ]; then
		echo "FAIL $label: no AOT snapshot (libapp.so or App.framework/App); not a release build"
		failed=1
	fi
	while IFS= read -r f; do
		echo "FAIL $label: ${f#"$dir"/} must not ship in a release build"
		failed=1
	done < <(find "$dir" -type f \( -name kernel_blob.bin -o -name '*.dill' -o -name vm_snapshot_data \
		-o -name isolate_snapshot_data -o -name '*.map' -o -name '*.symbols' \) | sort)
	if [ "$failed" -eq 0 ]; then
		echo "ok   $label: $snapshots AOT snapshot(s), no probe string, no debug or map file"
	fi
	return "$failed"
}

# artifact <path>: inspects a directory, or an archive extracted first.
artifact() {
	local path=$1 tmp rc=0
	if [ -d "$path" ]; then
		inspect "$path" "$path" || rc=1
		return "$rc"
	fi
	[ -f "$path" ] || { echo "no such file or directory: $path" >&2; return 2; }
	tmp=$(mktemp -d)
	unzip -q -o "$path" -d "$tmp" || { rm -rf "$tmp"; echo "FAIL $path: not a zip archive" ; return 1; }
	inspect "$path" "$tmp" || rc=1
	rm -rf "$tmp"
	return "$rc"
}

build() {
	local root out symbols
	root=$(git rev-parse --show-toplevel)
	out=${RELEASE_OUT:-$root/build/release}
	mkdir -p "$out"
	symbols=$out/symbols
	mkdir -p "$symbols"
	case "$1" in
	android)
		# The starter keeps its class names, so the probes that are class
		# names mean something; Plux Bank is built as a store build is,
		# obfuscated, with its symbols kept outside the package.
		(cd "$root/apps/starter" && flutter build apk --release --target-platform android-arm64)
		(cd "$root/apps/plux_bank" && flutter build apk --release --target-platform android-arm64 \
			--obfuscate --split-debug-info="$symbols")
		artifacts=(
			"$root/apps/starter/build/app/outputs/flutter-apk/app-release.apk"
			"$root/apps/plux_bank/build/app/outputs/flutter-apk/app-release.apk"
		)
		;;
	ios)
		# --no-codesign leaves Runner.app, the content of an IPA's Payload.
		(cd "$root/apps/starter" && flutter build ios --release --no-codesign)
		(cd "$root/apps/plux_bank" && flutter build ios --release --no-codesign \
			--obfuscate --split-debug-info="$symbols")
		artifacts=(
			"$root/apps/starter/build/ios/iphoneos/Runner.app"
			"$root/apps/plux_bank/build/ios/iphoneos/Runner.app"
		)
		;;
	*)
		echo "usage: inspect.sh --build android|ios" >&2
		exit 2
		;;
	esac
}

if [ "$#" -eq 0 ]; then
	echo "usage: inspect.sh <artifact>... | --build android|ios" >&2
	exit 2
fi
artifacts=("$@")
if [ "$1" = --build ]; then
	[ "$#" -eq 2 ] || { echo "usage: inspect.sh --build android|ios" >&2; exit 2; }
	build "$2"
fi

status=0
for a in "${artifacts[@]}"; do
	rc=0
	artifact "$a" || rc=$?
	if [ "$rc" -eq 2 ]; then exit 2; fi
	if [ "$rc" -ne 0 ]; then status=1; fi
done
exit "$status"
