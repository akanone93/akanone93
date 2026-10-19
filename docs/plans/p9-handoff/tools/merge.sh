#!/bin/bash
# merge.sh <branch>: merge, auto-resolve generated (ours) and list-like files (union), list the rest.
# After merging, run `make gen` (and the -update test flags) to regenerate the files kept as "ours".
S=$(cd "$(dirname "$0")" && pwd)/union.py
cd "$(git rev-parse --show-toplevel)"
git merge --no-edit "$1" >/dev/null 2>&1
for f in $(git diff --name-only --diff-filter=U); do
  case $f in
    *plugin.json|*renderer.dart|*/widgets/app.json) python3 $S "$f" && git add "$f";;
    *_gen.go|*.g.dart|*_generated.dart|*.gen.ts|*.pb.go|*ids.lock.json|*COVERAGE.md|*flutter-api.json|*.pxb|*.ttf|*.png|*errors.json|docs/reference/*|*/dbgen/*|*vectors.json|*openapi*) git checkout --ours "$f" 2>/dev/null && git add "$f" || git rm -q "$f";;
    *) echo "MANUAL $f";;
  esac
done
U=$(git diff --name-only --diff-filter=U); if [ -z "$U" ] && [ -f .git/MERGE_HEAD ]; then git commit -q --no-edit 2>&1 | grep -i failed; fi; echo "$U" | sed "s/^/STILL /" | grep -v "^STILL $"; git log --oneline -1
