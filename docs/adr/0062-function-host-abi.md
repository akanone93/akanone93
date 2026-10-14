# 0062. The function host ABI: one JSON-based interface for both placements

- **Status:** Proposed
- **Date:** 2026-10-13
- **Requirements:** `FN-002`, `FN-012`, `FN-015`, `FN-022`, `FN-052`

## Context and problem

`FN-015` requires an ABI between the function hosts (`wazero` on the server, WAMR on the
device) and function modules that is language-neutral, versioned, identical for both
placements and documented in `docs/functions/abi.md`. Rust is the first source language
([ADR-0011](0011-plux-functions-rust-wasm.md)); Go and others come later (`FN-040`), and a
second host in a second language must behave the same. The ABI is the contract the
conformance vectors in `schema/testdata/functions` test on both hosts.

## Decision drivers

- Language-neutral: nothing in it assumes Rust or Go types.
- Identical on both hosts; small enough to implement twice (Go on `wazero`, C on WAMR).
- Deny-by-default capabilities with a defined error for every refusal (`FN-012`), never a
  trap the host cannot classify.
- Debuggable by function authors and Studio: payloads a person can read.
- Cheap per call: the budgets of ADR-0011 leave no room for heavy codecs in the interpreter.
- Evolvable without breaking deployed modules.

## Considered options

1. JSON (UTF-8) for every payload at the boundary.
2. A binary encoding (MessagePack or FlatBuffers) at the boundary.
3. The WebAssembly component model with WIT interfaces.
4. Guest-allocated versus host-allocated buffers (a sub-decision).

## Decision

Chosen option: **1, with host-allocated buffers written through the guest's allocator**.

**Exports.** A module exports `memory`, `plux_abi_version() -> i32` (1),
`plux_alloc(len: i32) -> i32` and `plux_invoke(index: i32, ptr: i32, len: i32) -> i64`.
`index` selects an entry in the module's dispatch table (a module links one or more
functions). The result is packed `(ptr << 32) | len` and points to `tag || UTF-8 JSON`:
**tag 0** is the output, **tag 1** a business error `{code, messageKey, params}` (`FN-022`),
**tag 2** the guest's refusal of its input (`{message}`, `PLX-7008`) and **tag 3** its failure to
encode the output (`{message}`, `PLX-7009`). A trap, a limit or a malformed result is a system
error, classified by the host.

**Imports**, all from the module `plux_v1`. Each returns an `i32` status or an `i64` packed
result, negative for a status:

| Import | Placement |
|---|---|
| `log(level, ptr, len)`, `clock_now() -> i64` (Unix ns), `context() -> i64` (JSON: app, environment, placement, assurance level), `env_get`, `kv_get`, `kv_put`, `kv_delete` | both |
| `http_request` (JSON request and response, body base64), `secret_get`, `claims`, `invoke_function` | server only |

Statuses: `-1` not found, `-2` capability denied, `-3` limit exceeded, `-4` egress refused,
`-5` upstream failed, `-6` invalid argument, `-7` unavailable. A host returns a buffer by
calling the guest's `plux_alloc` and writing into it; the guest owns it. Every invocation gets a
fresh instance (`FN-010`, `FN-053`), so nothing is freed across the boundary: the instance's
memory is discarded with it, and the ABI needs no `plux_free`.

**Refusal rules.** A device host refuses a module that imports a server-only function
(`PLX-7018`); every host refuses an unknown import, an import or export with the wrong
signature, and a missing required export as an invalid module (`PLX-7018`), and an unsupported
`plux_abi_version` as `PLX-7017` (maintainer, 2026-10-14). Refusal happens at load, before any code runs. A capability not granted to a
function returns `-2`, and the SDK maps it to `PLX-7003`.

**WASI shim.** The target is `wasm32-wasip1`, so a module imports a few WASI functions. The
host provides a shim: no preopened directories, no arguments, no environment, no sockets;
`fd_write` to descriptors 1 and 2 becomes `log`, bounded by `function.logBytes`;
`clock_time_get` reads the host clock; `random_get` reads the host's CSPRNG; everything else
returns `ENOSYS` or `EBADF`. The shim is part of the ABI and tested by the same vectors.

**Versioning.** Additions that old modules ignore (new optional fields in JSON payloads, new
statuses a module may treat as generic failure) stay in `plux_v1`. A change that an old
module could misread is a **new major ABI**: it gets a new import module `plux_v2` and a new
`plux_abi_version` value. A host supports a *declared set* of majors and loads each module
under the one it exports; a module is never translated. Hosts and Studio report the set they
support, so a deploy of an unsupported version fails at once.

## Consequences

- **Positive:** two small hosts that can be written against one document and one set of
  vectors; every payload readable in logs and Studio; a module built today runs on later
  hosts, because old majors stay supported for as long as the set declares them.
- **Negative:** JSON costs parsing time and size against a binary format; base64 bodies add
  a third to HTTP payloads. Numbers are decimal text, so functions use decimal types for
  money (`rust_decimal`), which is the right habit anyway.
- **Follow-up:** `docs/functions/abi.md` and the vectors land with the SDK (F1); a measured
  comparison of one binary encoding is worth repeating if a call profile shows JSON at the
  top.

## Options in detail

### JSON at the boundary

Matches the rest of Plux (input and output schemas are JSON Schema, `FN-004`, so the host
validates with the same schemas it publishes). Every language has a parser, and the measured
11 ms p50 call in the interpreter includes it. Chosen.

### MessagePack or FlatBuffers

MessagePack is compact and needs a library in every guest language and both hosts; the
saving is small for the payload sizes involved, and validation against JSON Schema would need
a conversion. FlatBuffers is zero-copy but needs generated code per function type, which the
schema-derivation design (`FN-004`) does not provide, and is hard to read when debugging.
Rejected: more code in two hosts and every SDK for a gain no budget needs.

### Component model and WIT

The right long-term shape for typed interfaces, but tooling is not mature in `wazero`, WAMR's
support is partial, and a component adapter adds size to every module that ADR-0011's budgets
count in kilobytes. Rejected for v1; a future `plux_v2` can adopt it, which the versioning
rule allows.

### Who allocates buffers

If the guest allocated the result and the host returned buffers through a host allocator, the
host would need to manage guest memory and growth. Host-allocated through the guest's
`plux_alloc` keeps one allocator inside the module's linear memory, so the memory limit
(`function.memory`) covers every byte and the host cannot corrupt the guest's heap
bookkeeping.
