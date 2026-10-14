
## Revision (2026-10-14, P7 F5 measurements)

F5 measured both engines on the development host (x86-64; no emulator was available), with
the same linked loan module computing a 360-month schedule, far more than `FN-056`'s typical
calculation of up to 10,000 arithmetic operations:

| | WAMR 2.4.3, fast interpreter | `wasmi` 2.0.0 |
|---|---|---|
| Stripped native library | 200 KB | 2.3 MB |
| 360-month schedule, p50 / p95 | 12.6–14.6 ms / 20.2–24.4 ms | 2.8–2.9 ms / 4.3–4.7 ms |

`wasmi` is about 4.5 times faster on this call and about 11 times larger, and it would bring a
Rust toolchain into every app build (D10). The maintainer keeps WAMR for now (P7 plan §2.6
M8). The measurement on the mid-tier reference device decides: if WAMR misses `FN-056`'s
20 ms p95 for a typical calculation and `wasmi` meets it, this decision is reopened with the
device numbers. `FN-056` stays `WIP` until that run. Numbers and method:
[benchmarks/p7-functions.md](../benchmarks/p7-functions.md).
