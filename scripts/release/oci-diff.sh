#!/bin/bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Explains why two OCI image archives of one build differ (CI-006): prints
# the config and the layers whose digests differ, and for each differing layer
# the entries whose metadata or content differ. Diagnostic only; always exits 0.
set -u
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/a" "$work/b"
tar -xf "$1" -C "$work/a"
tar -xf "$2" -C "$work/b"

python3 - "$work" <<'EOF'
import gzip, hashlib, json, os, subprocess, sys, tarfile

work = sys.argv[1]

def blob(side, digest):
    return os.path.join(work, side, "blobs", *digest.split(":", 1))

def manifest(side):
    doc = json.load(open(os.path.join(work, side, "index.json")))
    while "manifests" in doc:
        doc = json.load(open(blob(side, doc["manifests"][0]["digest"])))
    return doc

def entries(side, digest):
    path = blob(side, digest)
    with open(path, "rb") as f:
        gz = f.read(2) == b"\x1f\x8b"
    out = []
    with (gzip.open if gz else open)(path, "rb") as f, tarfile.open(fileobj=f, mode="r|") as t:
        for m in t:
            h = hashlib.sha256(t.extractfile(m).read()).hexdigest()[:16] if m.isfile() else ""
            out.append(f"{m.name} {m.mode:o} {m.uid}:{m.gid} {m.mtime} {m.type!r} {m.linkname} {h}\n")
    return out

a, b = manifest("a"), manifest("b")
if a["config"]["digest"] != b["config"]["digest"]:
    print("config differs:", a["config"]["digest"], b["config"]["digest"])
for i, (x, y) in enumerate(zip(a["layers"], b["layers"])):
    if x["digest"] == y["digest"]:
        continue
    print(f"── layer {i} differs: {x['digest']} {y['digest']}")
    la, lb = entries("a", x["digest"]), entries("b", y["digest"])
    sa, sb = set(la), set(lb)
    shown = 0
    for line in la:
        if line not in sb and shown < 20:
            print("  - " + line, end="")
            shown += 1
    for line in lb:
        if line not in sa and shown < 40:
            print("  + " + line, end="")
            shown += 1
    if la != lb and sa == sb:
        print("  same entries, different order")
EOF
exit 0
