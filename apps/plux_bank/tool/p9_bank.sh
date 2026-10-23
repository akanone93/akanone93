#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# The P9 scenarios of Plux Bank (DX-004) the walkthrough calls, one step
# per subcommand, each a thin sequence of `plux` commands against the
# server and app the CLI is configured for (PLUX_SERVER, PLUX_ORGANIZATION
# and the app of plux.json, or --app through PLUX_ARGS). The project is the
# fixture schema/testdata/documents/plux_bank, whose security profile is
# `maximum`.
#
#   policies      show the policies in force and write the secure transfer policy
#   release       publish and promote to production; the release touches the
#                 secure transfer page, so it waits for two approvers from
#                 distinct teams
#   approvals     list the open approval requests
#   kill          switch the transfer page off: devices show the fallback page
#   unkill        switch it back on
#   control       show the channel's control document
#   reconstruct   reconstruct a device's moment on the transfer page
#
# Environment: PLUX (the CLI, default plux), PLUX_ENV (default production),
# PLUX_ARGS (extra flags for every command, for example "--app <id>"),
# PLUX_PROJECT (the project directory, default the fixture), and for
# `reconstruct` DEVICE_ID and MOMENT (RFC 3339); `kill` and `unkill` take
# the transfer page's document identifier from TRANSFER_PAGE_ID.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
plux="${PLUX:-plux}"
env_key="${PLUX_ENV:-production}"
project="${PLUX_PROJECT:-${here}/../../../schema/testdata/documents/plux_bank}"
# The transfer page's identifier in the fixture.
transfer_page="${TRANSFER_PAGE_ID:-01d0c450-6c00-7000-8000-000000000212}"
# shellcheck disable=SC2206 # PLUX_ARGS is a word list by design.
extra=(${PLUX_ARGS:-})

policies() {
	"${plux}" policies list ${extra[@]+"${extra[@]}"}
	"${plux}" policies put ${extra[@]+"${extra[@]}"} \
		"${project}/policies/secure-transfer-release.policy.json"
}

release() {
	"${plux}" publish ${extra[@]+"${extra[@]}"} -C "${project}" --env "${env_key}" --release --promote "${env_key}"
}

reconstruct() {
	: "${DEVICE_ID:?set DEVICE_ID to the device whose moment to reconstruct}"
	: "${MOMENT:?set MOMENT (RFC 3339), a time the device was on the transfer page}"
	"${plux}" audit reconstruct ${extra[@]+"${extra[@]}"} --device "${DEVICE_ID}" --at "${MOMENT}"
}

case "${1:-}" in
policies) policies ;;
release) release ;;
approvals) "${plux}" approvals list ${extra[@]+"${extra[@]}"} --status open ;;
kill) "${plux}" control kill ${extra[@]+"${extra[@]}"} --env "${env_key}" page "${transfer_page}" ;;
unkill) "${plux}" control unkill ${extra[@]+"${extra[@]}"} --env "${env_key}" page "${transfer_page}" ;;
control) "${plux}" control get ${extra[@]+"${extra[@]}"} --env "${env_key}" ;;
reconstruct) reconstruct ;;
*)
	echo "usage: p9_bank.sh policies|release|approvals|kill|unkill|control|reconstruct" >&2
	exit 2
	;;
esac
