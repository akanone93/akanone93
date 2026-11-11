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
# quotes or line breaks, which Windows' flutter.bat drops from arguments, and
# the app then fails at start with a broken key list.
set -eu

keys=${1:?usage: dev-app-defines.sh <keys.json> [<ca.crt>]}
ca=${2:-}

# A file's text as a JSON string body: backslashes and quotes escaped, CR
# dropped, line breaks as \n.
json_text() {
	awk 'BEGIN { ORS = "" } {
		gsub(/\r/, ""); gsub(/\\/, "\\\\"); gsub(/"/, "\\\"")
		if (NR > 1) print "\\n"
		print
	}' "$1"
}

# Everything is checked before anything is written.
[ -r "$keys" ] || { echo "dev-app-defines.sh: cannot read $keys" >&2; exit 1; }
if [ -n "$ca" ]; then
	[ -r "$ca" ] || { echo "dev-app-defines.sh: cannot read $ca" >&2; exit 1; }
	grep -q 'BEGIN CERTIFICATE' "$ca" || { echo "dev-app-defines.sh: $ca is not a PEM certificate" >&2; exit 1; }
fi
printf '{"plux.devtools":"true","PLUX_DEV_ROOT_KEYS":"%s"' "$(json_text "$keys")"
[ -z "$ca" ] || printf ',"PLUX_DEV_CA":"%s"' "$(json_text "$ca")"
printf '}\n'
