# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Prints, as hex, the tiny hand-assembled modules that test/refusals_test.dart
# embeds: a valid module, and one each with an unsupported plux_abi_version, an
# http_request import, an unknown import and no plux_alloc export.
#   python3 refusals.py
import sys
def leb(n):
    o=bytearray()
    while True:
        b=n&0x7f;n>>=7
        if n: o.append(b|0x80)
        else: o.append(b);return bytes(o)
def vec(items): return leb(len(items))+b''.join(items)
def name(s): return leb(len(s))+s.encode()
def section(i,body): return bytes([i])+leb(len(body))+body
def module(imports, version, drop_alloc=False):
    types=[b'\x60\x00\x01\x7f', b'\x60\x01\x7f\x01\x7f', b'\x60\x03\x7f\x7f\x7f\x01\x7e', b'\x60\x02\x7f\x7f\x01\x7e']
    imps=[name(m)+name(n)+b'\x00'+leb(3) for m,n in imports]
    funcs=[leb(0),leb(1),leb(2)]
    exports=[name('memory')+b'\x02\x00', name('plux_abi_version')+b'\x00'+leb(len(imports)), name('plux_alloc')+b'\x00'+leb(len(imports)+1), name('plux_invoke')+b'\x00'+leb(len(imports)+2)]
    if drop_alloc: exports.pop(2)
    ver=b'\x41'+leb(version)+b'\x0b'
    bodies=[leb(len(b'\x00'+ver))+b'\x00'+ver,
            (lambda b: leb(len(b))+b)(b'\x00\x41\x00\x0b'),
            (lambda b: leb(len(b))+b)(b'\x00\x42\x00\x0b')]
    out=b'\x00asm\x01\x00\x00\x00'
    out+=section(1,vec(types))
    if imps: out+=section(2,vec(imps))
    out+=section(3,vec(funcs))
    out+=section(5,vec([b'\x00\x01']))
    out+=section(7,vec(exports))
    out+=section(10,vec(bodies))
    return out
cases={
 'valid': module([],1),
 'abi2': module([],2),
 'http': module([('plux_v1','http_request')],1),
 'unknown': module([('env','mystery')],1),
 'noalloc': module([],1,True),
}
for k,v in cases.items(): print(k, v.hex())
