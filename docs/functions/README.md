# Plux Functions

Business logic that declarative actions cannot express runs as Plux Functions: Rust compiled to
WebAssembly, on the server or on the device (spec §16). This directory holds the host contract;
the guides explain how to use it.

| Document | Covers |
|---|---|
| [Functions guide](../guides/functions.md) | Writing, testing, deploying and promoting a function with `plux-fn` and `plux fn` |
| [Host ABI v1](abi.md) | The language-neutral contract between a function host and a module (`FN-015`, [ADR-0062](../adr/0062-function-host-abi.md)) |
| [Builds and provenance](../guides/function-builds.md) | The `builder` role, reproducibility, the allowlist and the signed statement ([ADR-0063](../adr/0063-function-builds-provenance.md)) |
| [Security and limits](../guides/function-security.md) | Isolation, egress, capabilities, per-request checks and the `function.*` limits ([ADR-0064](../adr/0064-function-isolation.md)) |
| Runbooks | [Function outage](../runbooks/function-outage.md), [egress refusals](../runbooks/function-egress-refusals.md), [builder failure](../runbooks/function-builder-failure.md) |
