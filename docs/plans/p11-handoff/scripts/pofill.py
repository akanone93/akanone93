# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Fills empty msgstr entries in de/ar catalogues from the given git revisions (first non-empty wins).
import re, subprocess, sys
revs = sys.argv[1:]
for l in ("de", "ar"):
    p = f"studio/apps/web/src/locales/{l}/messages.po"
    s = open(p, encoding="utf-8").read()
    known = {}
    for r in revs:
        old = subprocess.run(["git", "show", f"{r}:{p}"], capture_output=True, text=True).stdout
        for m in re.finditer(r'msgid "((?:[^"\\]|\\.)*)"\nmsgstr "((?:[^"\\]|\\.)+)"\n', old):
            known.setdefault(m.group(1), m.group(2))
    def fill(m):
        k = m.group(1)
        return f'msgid "{k}"\nmsgstr "{known[k]}"\n' if k in known else m.group(0)
    s = re.sub(r'msgid "((?:[^"\\]|\\.)*)"\nmsgstr ""\n(?!")', fill, s)
    open(p, "w", encoding="utf-8").write(s)
