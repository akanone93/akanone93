#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Copies the rules and dashboards, the source of truth in deploy/observability,
# into the Helm chart, which cannot read files outside its directory. The
# Compose stack mounts the source directly. tools/internal/observability fails
# when a copy differs (make helm-lint).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
dest=$here/../helm/plux/files/observability

rm -rf "$dest"
mkdir -p "$dest/dashboards"
cp "$here/rules/plux.rules.yml" "$dest/plux.rules.yml"
cp "$here"/dashboards/*.json "$dest/dashboards/"
echo "copied the rules and dashboards into the chart"
