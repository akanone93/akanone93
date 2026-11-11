#!/usr/bin/env sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Keeps `adb reverse tcp:<port> tcp:<port>` applied to every Android device
# adb sees, for as long as it runs: a phone plugged in, plugged in again, or
# connected over Android's wireless debugging reaches the dev stack at
# https://localhost:<port> without anyone typing the command (ADR-0104).
# `make dev` starts it unless DEV_ADB_REVERSE=0.
#
#   scripts/adb-reverse-watch.sh [port]      (default 8080)
#
# It uses `adb`, or under WSL Windows' `adb.exe`, which is the adb a USB
# phone is attached to there (ADB=<path> chooses another). Without either it
# says so once and exits: the phone then needs `adb reverse` by hand. It only
# forwards the device's own localhost port to this machine's; it opens no port
# on any network.
set -eu

port=${1:-8080}
case $port in '' | *[!0-9]*)
	echo "adb-reverse-watch: the port must be a number, not '$port'" >&2
	exit 2
	;;
esac
interval=${ADB_REVERSE_INTERVAL:-2}

adb=${ADB:-}
if [ -z "$adb" ]; then
	if command -v adb >/dev/null 2>&1; then
		adb=adb
	elif command -v adb.exe >/dev/null 2>&1; then
		adb=adb.exe
	else
		echo "· adb not found: run 'adb reverse tcp:$port tcp:$port' for a phone yourself (DEV_ADB_REVERSE=0 hides this)."
		exit 0
	fi
fi

# The serials of the devices adb lists as ready, one per line.
ready() {
	"$adb" devices 2>/dev/null | tr -d '\r' | awk 'NR > 1 && $2 == "device" { print $1 }'
}

# Whether the device already forwards the port.
forwarded() {
	"$adb" -s "$1" reverse --list 2>/dev/null | tr -d '\r' | grep -q "tcp:$port tcp:$port\$"
}

echo "· adb reverse kept on tcp:$port for every phone adb sees (DEV_ADB_REVERSE=0 turns this off)."
known=
while :; do
	now=$(ready || true)
	for serial in $now; do
		if ! forwarded "$serial"; then
			if "$adb" -s "$serial" reverse "tcp:$port" "tcp:$port" >/dev/null 2>&1; then
				echo "✓ $serial reaches the dev stack at localhost:$port"
			fi
		fi
	done
	for serial in $known; do
		# $now splits on purpose: one serial per word.
		# shellcheck disable=SC2086
		case " $(printf '%s ' $now)" in *" $serial "*) ;; *) echo "· $serial disconnected; its reverse comes back when it reconnects" ;; esac
	done
	known=$now
	sleep "$interval"
done
