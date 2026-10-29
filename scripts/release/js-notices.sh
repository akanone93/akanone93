#!/bin/sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Copies the licence and notice files of the packages in a node_modules
# directory to an output directory, one folder per package, for the image
# to carry under /usr/share/doc (DEP-001). Usage: js-notices.sh NODE_MODULES OUT
# The legal review of the result stays open (O-7). Output is sorted, so it
# is reproducible (CI-006).
set -eu
nm=$1
out=$2
mkdir -p "$out"
find "$nm" -mindepth 2 -maxdepth 3 -type f \
  \( -iname 'LICENSE*' -o -iname 'LICENCE*' -o -iname 'NOTICE*' -o -iname 'COPYING*' \) \
  -not -path '*/node_modules/*/node_modules/*' | LC_ALL=C sort | while read -r f; do
  rel=${f#"$nm"/}
  dir=$(dirname "$rel")
  mkdir -p "$out/$dir"
  cp "$f" "$out/$dir/"
done
