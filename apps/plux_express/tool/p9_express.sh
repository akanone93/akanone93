#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# The P9 scenarios of Plux Express (DX-004) the walkthrough calls, one
# step per subcommand, each a thin sequence of `plux` commands against the
# server and app the CLI is configured for (PLUX_SERVER, PLUX_ORGANIZATION
# and the app of plux.json, or --app through PLUX_ARGS). The project is the
# fixture schema/testdata/documents/plux_express.
#
#   campaign      schedule the campaign release (a rollout with a start and an end)
#   experiment    register and start the catalogue banner experiment (ABT-001)
#   courier-off   switch the courier flag off for the fleet (NAV-009, ABT-006)
#   courier-on    switch it back on
#
# Environment: PLUX (the CLI, default plux), PLUX_ENV (default production),
# PLUX_ARGS (extra flags for every command, for example "--app <id>"),
# PLUX_PROJECT (the project directory, default the fixture), and for
# `campaign` CAMPAIGN_RELEASE, CAMPAIGN_START and CAMPAIGN_END (RFC 3339).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
plux="${PLUX:-plux}"
env_key="${PLUX_ENV:-production}"
project="${PLUX_PROJECT:-${here}/../../../schema/testdata/documents/plux_express}"
# shellcheck disable=SC2206 # PLUX_ARGS is a word list by design.
extra=(${PLUX_ARGS:-})

campaign() {
	: "${CAMPAIGN_RELEASE:?set CAMPAIGN_RELEASE to the sequence of the release that carries the campaign}"
	: "${CAMPAIGN_START:?set CAMPAIGN_START (RFC 3339)}"
	: "${CAMPAIGN_END:?set CAMPAIGN_END (RFC 3339); the channel returns to its previous release then}"
	"${plux}" rollout create ${extra[@]+"${extra[@]}"} --env "${env_key}" \
		--release "${CAMPAIGN_RELEASE}" --stages "100%" \
		--start "${CAMPAIGN_START}" --end "${CAMPAIGN_END}"
}

experiment() {
	"${plux}" experiment create ${extra[@]+"${extra[@]}"} --env "${env_key}" -C "${project}" \
		"${project}/experiments/catalogueBanner.experiment.json"
	"${plux}" experiment start ${extra[@]+"${extra[@]}"} --env "${env_key}" -C "${project}" catalogueBanner
}

courier_flag() {
	"${plux}" flags set ${extra[@]+"${extra[@]}"} --env "${env_key}" -C "${project}" \
		--type bool --default "$1" courierEnabled
}

case "${1:-}" in
campaign) campaign ;;
experiment) experiment ;;
courier-off) courier_flag false ;;
courier-on) courier_flag true ;;
*)
	echo "usage: p9_express.sh campaign|experiment|courier-off|courier-on" >&2
	exit 2
	;;
esac
