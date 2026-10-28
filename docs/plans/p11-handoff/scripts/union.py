# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
# Resolves purely additive merge conflicts by keeping both sides (HEAD first); two
# `export interface UpstreamData {` openings are joined. Check the result with tsc: a conflict
# that splits a function body needs a manual fix. Usage: python3 union.py <file>...
import re
import sys

pat = re.compile(r'<<<<<<< HEAD\n(.*?)=======\n(.*?)>>>>>>> [^\n]*\n', re.S)
key = 'export interface UpstreamData {\n'


def fix(m):
    a, b = m.group(1), m.group(2)
    if key in a and key in b:
        a1, a2 = a.split(key, 1)
        b1, b2 = b.split(key, 1)
        return a1 + b1 + key + a2 + b2
    return a + b


for p in sys.argv[1:]:
    s = open(p).read()
    n = len(pat.findall(s))
    s = pat.sub(fix, s)
    open(p, 'w').write(s)
    print(p, n)
