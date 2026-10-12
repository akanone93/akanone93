#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# The starter app's end-to-end flows on an Android emulator (QA-006), and
# the Kotlin add-to-app host's (HST-033), run by `make e2e-android` in CI.
# See test/e2e/README.md.
#
#   android.sh [api]   API level of the system image (default 35; 26 or
#                      later: the API 24 kernel panics on the emulator,
#                      ADR-0035)
#
# Installs the emulator and a Google APIs x86_64 system image with the SDK's
# own tools, boots a headless emulator (hardware acceleration needs KVM),
# forwards the device's loopback ports of the test server and the reference
# backend to this machine (adb reverse), and runs the Go driver with PLUX_E2E_DEVICE, which builds
# the app for the emulator and runs integration_test/app_test.dart there.
# While the emulator boots, it builds the Go driver, the app's Android
# project and the add-to-app host, so the builds the flows start recompile
# only the Dart code with their defines (ADR-0043).
# Cloud development sessions do not start emulators (docs/WORKLOG.md,
# device testing); this runs on GitHub's runners.
set -euo pipefail

api=${1:-35}
root=$(git rev-parse --show-toplevel)
out=${E2E_OUT:-$root/build/e2e}
sdk=${ANDROID_HOME:-${ANDROID_SDK_ROOT:?set ANDROID_HOME to the Android SDK}}
tools=$sdk/cmdline-tools/latest/bin
image="system-images;android-$api;google_apis;x86_64"
avd=plux_e2e_$api
# The test server's address (backend/internal/server/starter_e2e_integration_test.go).
port=18094
# The reference apps' servers (Plux Bank 18097, Plux Express 18098) and their
# reference backend (18096; test/refapi), which the device reaches over
# reverse forwards of their own
# (backend/internal/server/reference_e2e_integration_test.go).
refapi_port=18096
reference_ports="18097 18098"
mkdir -p "$out"

yes | "$tools/sdkmanager" --licenses >/dev/null || true
# A download that arrives broken ("Error on ZipFile unknown archive", CI
# runs 36743012409 and 37644772933, where ten emulator jobs downloaded the
# same packages at once) is fetched again after 15 and 45 seconds before
# the job gives up. CI restores the packages from its cache first, so a
# download is the exception.
: >"$out/sdkmanager.log"
for wait in 0 15 45; do
	sleep "$wait"
	if "$tools/sdkmanager" --install emulator platform-tools "$image" >>"$out/sdkmanager.log" 2>&1; then
		installed=true; break
	fi
	installed=false
done
$installed || { tail -n 20 "$out/sdkmanager.log"; echo "✗ the Android SDK packages did not install"; exit 1; }
# One AVD home for avdmanager and the emulator, whatever the runner sets.
export ANDROID_AVD_HOME=${ANDROID_AVD_HOME:-$HOME/.android/avd}
mkdir -p "$ANDROID_AVD_HOME"
echo no | "$tools/avdmanager" create avd --force --name "$avd" --package "$image" --device pixel_6 >/dev/null
[ -r /dev/kvm ] && [ -w /dev/kvm ] || echo "⚠ /dev/kvm is not usable: the emulator runs without acceleration"

adb=$sdk/platform-tools/adb
log=$out/emulator-$api.log
# The adb server, and its key in ~/.android, exist before the emulator
# starts, so the emulator's registration with the server cannot race the
# server's first start. Suspected in CI run 36966274717, where the
# emulator reported its boot complete and adb never answered for it; fail
# now prints the devices adb sees.
"$adb" start-server >/dev/null
"$sdk/emulator/emulator" -avd "$avd" -no-window -no-audio -no-boot-anim -no-snapshot \
	-gpu swiftshader_indirect -port 5554 -memory 4096 -no-metrics >"$log" 2>&1 &
emulator=$!
serial=emulator-5554
trap '"$adb" -s "$serial" emu kill >/dev/null 2>&1 || kill "$emulator" 2>/dev/null || true' EXIT

# The Go modules first, with retries: a runner's connection to the module
# proxy can time out (runs 38238159276, 38260897809), and the build below
# would fail the job on it before any flow ran.
for attempt in 1 2 3; do
	(cd "$root/backend" && go mod download && cd "$root/test/refapi" && go mod download) && break
	[ "$attempt" = 3 ] && { echo "✗ the Go modules could not be downloaded"; exit 1; }
	sleep $((attempt * 15))
done
prebuild_log=$out/prebuild-$api.log
# Only what E2E_SHARD's flows build (mk/device.mk).
shard=${E2E_SHARD:-all}
(
	cd "$root/backend" && go test -count=1 -run '^$' ./internal/server
	if [ "$shard" = all ] || [ "$shard" = starter ]; then
		cd "$root/apps/starter" && flutter build apk --debug
	fi
	if [ "$shard" = all ] || [ "$shard" = hosts ]; then
		# The add-to-app host with the Gradle wrapper flutter pub get writes
		# into the module (TestAddToAppAgainstTheServer builds it again, with
		# the baseline it pulls).
		cd "$root/apps/add_to_app/plux_module" && flutter pub get
		cd "$root/apps/add_to_app/android_host" && ../plux_module/.android/gradlew --no-daemon --console=plain \
			-Ptarget-platform=android-x64 :app:assembleDebug :app:assembleDebugAndroidTest
	fi
) >"$prebuild_log" 2>&1 &
prebuild=$!

# fail <message>: the emulator's log, the devices adb sees, then the
# message.
fail() {
	echo "--- $log (last 80 lines)"; tail -n 80 "$log" || true
	echo "--- adb devices"; timeout 10 "$adb" devices -l || true
	echo "✗ $1"; exit 1
}

# connections: how many entries adb's server lists for the emulator.
connections() {
	timeout 10 "$adb" devices 2>/dev/null | awk -v s="$serial" '$1 == s' | wc -l | tr -d ' '
}

# single: restarts adb's server and waits, at most a minute, until it lists
# the emulator once in eight polls in a row.
single() {
	echo "adb lists $serial more than once; restarting adb's server"
	timeout 10 "$adb" kill-server >/dev/null 2>&1 || true
	timeout 10 "$adb" start-server >/dev/null 2>&1 || true
	local n=0 until=$((SECONDS + 60))
	while [ "$n" -lt 8 ] && [ "$SECONDS" -lt "$until" ]; do
		if [ "$(connections)" = 1 ]; then n=$((n + 1)); else n=0; fi
		sleep 2
	done
}

# adbs <args>: adb for the emulator. A second registration can still
# appear after the boot loop has seen one (run 38238286380); adb then
# refuses the serial, so the server restarts and the command runs again,
# at most three times.
adbs() {
	local attempt out
	for attempt in 1 2 3; do
		if out=$(timeout 60 "$adb" -s "$serial" "$@" 2>&1); then
			[ -z "$out" ] || printf '%s\n' "$out"
			return 0
		fi
		case $out in
		*"more than one device"*) single ;;
		*) printf '%s\n' "$out" >&2; return 1 ;;
		esac
	done
	printf '%s\n' "$out" >&2
	return 1
}

# Booted when sys.boot_completed is 1; never waits on a dead emulator or
# longer than ten minutes of wall time (adb wait-for-device would wait
# forever). The emulator can report its boot complete while adb's shell
# never answers on a stale connection (CI runs 36966274717, 37630248033:
# listed as a device, no answer for ten minutes); 30 seconds after the
# emulator's own report, and every 30 seconds after that, adb's server
# restarts, dropping every connection, and finds the emulator again.
# adb reconnect is not used: it can leave two connections under one serial,
# which every later "adb -s" refuses (run 37633642167).
booted=false
deadline=$((SECONDS + 600))
answer=
restart_at=
restarts=0
# Consecutive polls that saw the boot complete under a single adb entry.
stable=0
while [ "$SECONDS" -lt "$deadline" ]; do
	kill -0 "$emulator" 2>/dev/null || fail "the emulator exited while booting"
	answer=$(timeout 10 "$adb" -s "$serial" shell getprop sys.boot_completed 2>&1 | tr -d '\r') || true
	if [ "$answer" = 1 ]; then
		# A restart of adb's server can register the emulator twice under
		# one serial, a moment after it is first listed again, and every
		# later "adb -s" refuses it with "more than one device/emulator"
		# (runs 38127865288, 38151817486): booted only once eight polls in
		# a row, sixteen seconds, list a single entry.
		if [ "$(connections)" = 1 ]; then
			stable=$((stable + 1))
			if [ "$stable" -ge 8 ]; then
				booted=true; break
			fi
		else
			stable=0
			single
		fi
		sleep 2
		continue
	fi
	stable=0
	if grep -q 'Boot completed' "$log"; then
		if [ -z "$restart_at" ]; then
			# adbd can answer well after the emulator reports its boot on
			# API 26 with a software GPU; a restart is the last resort, as it
			# can register the emulator twice (above).
			restart_at=$((SECONDS + 90))
		elif [ "$SECONDS" -ge "$restart_at" ]; then
			restarts=$((restarts + 1))
			echo "adb has no answer from the booted emulator (last: ${answer:-nothing}); restarting adb's server ($restarts)"
			timeout 10 "$adb" kill-server >/dev/null 2>&1 || true
			timeout 10 "$adb" start-server >/dev/null 2>&1 || true
			restart_at=$((SECONDS + 30))
		fi
	fi
	sleep 2
done
$booted || fail "the emulator did not boot in 10 minutes (adb's last answer: ${answer:-nothing})"
if ! wait "$prebuild"; then
	echo "--- $prebuild_log (last 80 lines)"; tail -n 80 "$prebuild_log" || true
	echo "✗ building the driver or the app failed"; exit 1
fi
for s in window_animation_scale transition_animation_scale animator_duration_scale; do
	adbs shell settings put global "$s" 0
done
adbs reverse "tcp:$port" "tcp:$port"
for p in $refapi_port $reference_ports; do
	adbs reverse "tcp:$p" "tcp:$p"
done

# The device log of a failed run is kept beside the flows' log (uploaded
# with build/e2e/*.log), and its crashes are printed: a flow that ends
# with "did not complete" means the app died on the device, which only
# the device log explains.
"$adb" -s "$serial" logcat -G 16M >/dev/null 2>&1 || true
"$adb" -s "$serial" logcat -c >/dev/null 2>&1 || true
status=0
PLUX_E2E_REFAPI_ADDR=127.0.0.1:$refapi_port PLUX_E2E_DEVICE=$serial make -C "$root" --no-print-directory e2e-starter 2>&1 | tee "$out/android-$api-$shard.log" || status=$?
if [ "$status" != 0 ]; then
	device_log=$out/logcat-$api-$shard.log
	"$adb" -s "$serial" logcat -d -v threadtime >"$device_log" 2>&1 || true
	echo "--- $device_log: crashes and Flutter errors (last 80)"
	grep -E 'FATAL EXCEPTION|AndroidRuntime|F DEBUG|Abort message|E flutter|Fatal signal' "$device_log" | tail -n 80 || true
	exit "$status"
fi
