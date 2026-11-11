#!/usr/bin/env sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Writes the build settings of the Plux Dev app (DEV-001) as a JSON file for
# `flutter build|run --dart-define-from-file`:
#
#   scripts/dev-app-defines.sh <keys.json> [<ca.crt>] > dev-defines.json
#
# <keys.json> is the installation's development root keys as `plux pull`
# writes them (PLUX_DEV_ROOT_KEYS); <ca.crt> is its development certificate
# authority in PEM (PLUX_DEV_CA; `make dev`: deploy/compose/.secrets/ca/ca.crt).
#
# A file and not --dart-define on the command line: both values hold double
# quotes, which Windows' flutter.bat drops from arguments. Each value is one
# line: Flutter hands the defines to the Dart compiler on a command line that
# a line break cuts on Windows ("Scheme not starting with alphabetic
# character"). The key list is JSON, which needs no line breaks, and the app
# restores the certificate's PEM lines itself.
set -eu

keys=${1:?usage: dev-app-defines.sh <keys.json> [<ca.crt>]}
ca=${2:-}

# A file's text as one line of a JSON string body: each line trimmed and
# joined to the next without a break, backslashes and quotes escaped. With
# "compact", every space and tab goes too, so that no value holds whitespace:
# the key list's fields (identifiers, names and hex) hold none, so only its
# JSON layout goes, and the app reads PEM markers without their space.
json_text() {
	awk -v compact="${2:-}" 'BEGIN { ORS = "" } {
		gsub(/\r/, ""); gsub(/^[ \t]+|[ \t]+$/, "")
		if (compact != "") gsub(/[ \t]/, "")
		gsub(/\\/, "\\\\"); gsub(/"/, "\\\"")
		print
	}' "$1"
}

# Everything is checked before anything is written.
[ -r "$keys" ] || { echo "dev-app-defines.sh: cannot read $keys" >&2; exit 1; }
if [ -n "$ca" ]; then
	[ -r "$ca" ] || { echo "dev-app-defines.sh: cannot read $ca" >&2; exit 1; }
	grep -q 'BEGIN CERTIFICATE' "$ca" || { echo "dev-app-defines.sh: $ca is not a PEM certificate" >&2; exit 1; }
fi
printf '{"plux.devtools":"true","PLUX_DEV_ROOT_KEYS":"%s"' "$(json_text "$keys" compact)"
[ -z "$ca" ] || printf ',"PLUX_DEV_CA":"%s"' "$(json_text "$ca" compact)"
printf '}\n'
