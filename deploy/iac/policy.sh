#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Checks the secure defaults of OpenTofu plans for the infrastructure modules
# under deploy/iac: no public bucket or endpoint, encryption at rest on,
# private networking and deletion protection (DEP-050). The rules are in
# policy.jq. Verifies: DEP-050.
#
# Usage: deploy/iac/policy.sh <plan.json>...
#   plan.json is the output of `tofu show -json <planfile>`. Exit status 0
#   means every plan satisfies every rule; 1 means at least one violation.

set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)

if [ "$#" -eq 0 ]; then
	echo "usage: $0 <plan.json>..." >&2
	exit 2
fi

status=0
for plan in "$@"; do
	violations=$(jq -r -f "$here/policy.jq" "$plan")
	if [ -n "$violations" ]; then
		printf '%s\n' "$violations" | sed "s|^|$plan: |" >&2
		status=1
	fi
done
exit "$status"
