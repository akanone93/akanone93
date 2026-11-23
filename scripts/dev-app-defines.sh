#!/usr/bin/env sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Writes the build settings of the Plux Dev app (DEV-001) as a JSON file for
# `flutter build|run --dart-define-from-file`:
#
#   scripts/dev-app-defines.sh [<ca.crt>] > dev-defines.json
#
# <ca.crt> is the installation's development certificate authority in PEM
# (PLUX_DEV_CA; `make dev`: deploy/compose/.secrets/ca/ca.crt). The app is
# built with no key: the phone pins its organisation's development root when
# it is connected in Studio, and each app opens with the development key that
# root certifies (ADR-0109).
#
# A file and not --dart-define on the command line: the value holds double
# quotes, which Windows' flutter.bat drops from arguments. It is one line:
# Flutter hands the defines to the Dart compiler on a command line that a line
# break cuts on Windows ("Scheme not starting with alphabetic character"), and
# the app restores the certificate's PEM lines itself.
set -eu

ca=${1:-}

# A file's text as one line of a JSON string body: each line trimmed and
# joined to the next without a break, backslashes and quotes escaped. With
# "compact", every space and tab goes too, so that no value holds whitespace:
# the app reads PEM markers without their space.
json_text() {
	awk -v compact="${2:-}" 'BEGIN { ORS = "" } {
		gsub(/\r/, ""); gsub(/^[ \t]+|[ \t]+$/, "")
		if (compact != "") gsub(/[ \t]/, "")
		gsub(/\\/, "\\\\"); gsub(/"/, "\\\"")
		print
	}' "$1"
}

# Everything is checked before anything is written.
if [ -n "$ca" ]; then
	[ -r "$ca" ] || { echo "dev-app-defines.sh: cannot read $ca" >&2; exit 1; }
	grep -q 'BEGIN CERTIFICATE' "$ca" || { echo "dev-app-defines.sh: $ca is not a PEM certificate" >&2; exit 1; }
fi
printf '{"plux.devtools":"true"'
[ -z "$ca" ] || printf ',"PLUX_DEV_CA":"%s"' "$(json_text "$ca" compact)"
printf '}\n'
