<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Function Host ABI v1

The contract between a function host and a function module (`FN-015`). A host is the program
that loads a module and runs it: the Go host on `wazero` (`backend/internal/fnhost`) on the
server, and the WAMR host in `plux_functions` on the device. A module is WebAssembly built for
`wasm32-wasip1`, usually from Rust with the [`plux-fn`](../../sdk/fn/plux-fn) crate. The ABI is
the same on both placements and names no type of any source language. The reasoning is
[ADR-0062](../adr/0062-function-host-abi.md); how to write a function is the [functions
guide](../guides/functions.md).

If you only write functions in Rust you never meet this document: `plux-fn` implements the guest
side. Read it to write a second host, to build a module in another language (`FN-040`), or to
understand a `PLX-7017` or `PLX-7018` refusal.

The conformance vectors in [`schema/testdata/functions`](../../schema/testdata/functions/README.md)
test this document. A host that passes them implements the same ABI as every other host.

## 1. Overview

- Every payload at the boundary is **UTF-8 JSON**. Input, output, the invocation context, HTTP
  requests and responses, claims and errors are all JSON a person can read in a log.
- Every invocation runs in a **fresh instance** with its own linear memory (`FN-010`). Nothing
  survives a call, so nothing is freed across the boundary (section 5).
- Buffers cross the boundary as one `i64`: `(ptr << 32) | len`, with the pointer in the high 32
  bits. A negative `i64` is never a buffer; it is a status (section 4).
- The host's work is bounded per call by the limits of `FN-011` (`function.*` in the [limits
  registry](../reference/limits.md)). A limit is reported as its own error, never as an
  unclassified trap.

## 2. What a module exports

| Export | Signature | Meaning |
|---|---|---|
| `memory` | linear memory | Required. A module that imports its memory is invalid. |
| `plux_abi_version` | `() -> i32` | Required. Returns `1`. |
| `plux_alloc` | `(len: i32) -> i32` | Required. Returns the address of `len` writable bytes in the module's memory, or `0` when it cannot. |
| `plux_invoke` | `(index: i32, ptr: i32, len: i32) -> i64` | Required. Runs the function at `index` of the module's dispatch table with the JSON input at `ptr`, `len`. Returns a tagged result (section 3). |
| `plux_schemas` | `(index: i32) -> i64` | Optional. Present in a module built with the feature `plux-schema`: returns the untagged JSON `{"input": …, "output": …}` (JSON Schema, `FN-004`) of the function at `index`. The builder reads it ([ADR-0063](../adr/0063-function-builds-provenance.md)); a deployed module does not export it. |
| `_initialize` | `() -> ()` | Optional. The WASI reactor constructor. When present the host calls it once per instance, before `plux_alloc` and `plux_invoke`. |

A module links one or more functions. `index` selects among them by position in the module's
dispatch table. A server function version is a module of one function. A plugin's device
functions are linked into one module (`FN-051`). An `index` outside the table traps.

A module must **not** have a WebAssembly start section: it would run guest code during
instantiation, before the host controls the call. Use `_initialize`.

## 3. The tagged result of `plux_invoke`

`plux_invoke` returns `(ptr << 32) | len`. The `len` bytes at `ptr` are one **tag byte** followed
by a UTF-8 JSON payload. An empty result, a range outside the module's memory or an unknown tag
is `PLX-7007` (`FUNCTION_TRAPPED`).

| Tag | Meaning | Payload | Host reports |
|---|---|---|---|
| `0` | Output | The function's JSON output. | The output. |
| `1` | Business error (`FN-022`) | `{"code": "…", "messageKey": "…", "params": {…}}`. `code` matches `^[A-Z][A-Z0-9_]*$`; `params` is an object when present. | A business error, `PLX-7100`, carrying the function's own code. Not a system error. |
| `2` | The guest refused its input | `{"message": "…"}` | `PLX-7008` (`FUNCTION_INPUT_INVALID`) |
| `3` | The guest failed to encode its output | `{"message": "…"}` | `PLX-7009` (`FUNCTION_OUTPUT_INVALID`) |

A tag `1` payload without a `code` or a `messageKey`, or with `params` that is not an object, is
`PLX-7009`. A payload longer than `function.outputSize` is `PLX-7004`; the host does not copy it.
The input is limited the same way before an instance exists: larger than `function.inputSize` is
`PLX-7022`.

The tag byte is not part of the payload size that the limits count.

## 4. What a module imports

All imports come from the module **`plux_v1`**. A pointer and a length describe bytes in the
guest's memory. A range outside that memory ends the call with `PLX-7007`.

| Import | Signature | Placement |
|---|---|---|
| `log` | `(level: i32, ptr: i32, len: i32) -> ()` | both |
| `clock_now` | `() -> i64` | both |
| `context` | `() -> i64` | both |
| `env_get` | `(ptr: i32, len: i32) -> i64` | both |
| `kv_get` | `(ptr: i32, len: i32) -> i64` | both |
| `kv_put` | `(key_ptr: i32, key_len: i32, value_ptr: i32, value_len: i32) -> i32` | both |
| `kv_delete` | `(ptr: i32, len: i32) -> i32` | both |
| `http_request` | `(ptr: i32, len: i32) -> i64` | server only |
| `secret_get` | `(ptr: i32, len: i32) -> i64` | server only |
| `claims` | `() -> i64` | server only |
| `invoke_function` | `(name_ptr: i32, name_len: i32, in_ptr: i32, in_len: i32) -> i64` | server only |

A module imports only what it uses, so a device module never mentions a server-only function
(`FN-008`, `FN-052`).

### 4.1 Statuses

A host call that returns an `i64` returns either a packed buffer or a negative status. A call
that returns an `i32` (`kv_put`, `kv_delete`) returns `0` or a negative status. A host may add
statuses in later versions; a guest treats an unknown negative value as `-7`.

| Status | Name | Meaning |
|---|---|---|
| `-1` | not found | No such key, secret, environment variable, claim set or function. |
| `-2` | capability denied | The capability is not granted to this function (`FN-012`). The Rust SDK maps it to `PLX-7003`. |
| `-3` | limit exceeded | A limit of the capability was exceeded: HTTP calls (`function.httpCalls`), call depth (`function.callDepth`), key-value limits (`function.kv*`), or the guest could not allocate the reply. |
| `-4` | egress refused | The destination is not in the function's allowlist or the egress proxy refused it (`PLX-7010`). |
| `-5` | upstream failed | The remote side of an HTTP call or a nested call failed. |
| `-6` | invalid argument | The argument is malformed, for example an HTTP request that is not valid JSON or has a URL other than `http` or `https`. |
| `-7` | unavailable | The capability cannot be served now. |

An empty successful reply, such as a key whose value is empty, is returned as `0`: a zero
`i64` is an empty buffer, not an error. A guest that reads `len == 0` treats the buffer as empty.

### 4.2 The functions

**`log(level, ptr, len)`.** Appends a line to the invocation's log. `level` is `0` debug, `1`
info, `2` warn, `3` error; a larger value is clamped to `3`. The lines of one invocation share
the budget `function.logBytes`. The first line that does not fit is replaced by one record,
`log output truncated`, at warn level, and every later line is dropped. Logging never fails.

**`clock_now() -> i64`.** The host clock in Unix nanoseconds.

**`context() -> i64`.** A JSON object with the fields `app`, `environment`, `placement`
(`"server"` or `"device"`) and `assuranceLevel` (such as `"AL2"`).

**`env_get(name)`.** The value of an environment variable the function's document lists in
`env`, as raw bytes. `-1` when it is not set. These are values of the app's environment, not of
the process.

**`kv_get(key)`, `kv_put(key, value)`, `kv_delete(key)`.** The function's key-value namespace
(`FN-018`): PostgreSQL per app, environment and function on the server, the plugin's namespace on
the device. Needs the `kv` capability. `kv_get` and `kv_delete` return `-1` for a missing key (the Rust SDK turns that into success for
a delete). Sizes are bounded
by `function.kvBytes`, `function.kvKeys` and `function.kvValueBytes` (`-3`, `PLX-7016`).

**`http_request(request)`** (server only). `request` is JSON:

```json
{"method": "POST", "url": "https://api.example.com/v1/rates",
 "headers": {"content-type": "application/json"}, "body": "eyJhIjoxfQ=="}
```

`method` is letters only and defaults to `GET`. `url` is `http` or `https` with a host. `body`
is base64. A request that does not parse or breaks these rules is `-6`. The host then admits
the request in this order: no allowed domain at all is `-2`; a host outside the allowlist is
`-4`; more calls than `function.httpCalls` is `-3`. The allowlist holds exact host names or
`*.example.com` wildcards; a wildcard matches any host with at least one label before
`.example.com`, not `example.com` itself, and matching ignores case and a trailing dot. An
admitted request leaves through the `egress` proxy, which applies the allowlist again and
refuses private addresses (`-4`). The response is JSON:

```json
{"status": 200, "headers": {"content-type": "application/json"}, "body": "eyJyYXRlIjoxLjF9"}
```

`body` is base64. The host does not follow redirects; a `3xx` answer is the function's to read.
A response body over the runner's bound is `-3`, never silently truncated.

**`secret_get(name)`** (server only). The value of a secret granted to the function by name:
`-2` if the name is not granted, `-1` if it has no value. A function cannot list names.

**`claims()`** (server only). The caller's verified claims as a JSON object (`SEC-026`). `-2`
when the function does not declare `claims`, `-1` when the call carries none.

**`invoke_function(name, input)`** (server only). Calls another server function. `-2` if the
function is not granted by name, `-3` if the call would exceed `function.callDepth` (3). The
reply has the form of a `plux_invoke` result: a tag byte, then JSON, where tag `0` is the
callee's output and tag `1` its business error. A callee's system error is a status, not a
buffer.

## 5. Memory rules

The host never allocates in the module's memory by itself. To hand a buffer to the guest, the
host:

1. calls the guest's `plux_alloc(len)`;
2. writes the bytes at the returned address;
3. returns `(ptr << 32) | len`.

The guest owns the buffer from then on. The same applies to the input of `plux_invoke`: the host
calls `plux_alloc(len)`, writes the JSON, and passes `ptr` and `len` (`ptr` is `0` for an empty
input).

- `plux_alloc` returning `0` is a failed allocation. The host answers the import with `-3`.
- An address from `plux_alloc` at or above 2 GiB, or one whose range lies outside the memory, is
  `PLX-7007`: a packed buffer addresses at most 2 GiB.
- There is **no `plux_free`**. The instance is discarded after one call, so a guest leaks on
  purpose. This is why the ABI stays small, and why `function.memory` caps every byte the call
  uses: the memory limit covers the host's buffers too, because they sit in the guest's memory.
- Growing the memory beyond `function.memory` fails `memory.grow`, and the call ends with
  `PLX-7002`. A module whose initial memory already exceeds the limit is refused at load
  (`PLX-7018`) or on the call (`PLX-7002`).
- The host reads guest memory only through bounds-checked copies. A bad pointer never panics the
  host.

## 6. The WASI shim

The target is `wasm32-wasip1`, so a module imports a few functions from
`wasi_snapshot_preview1`. The host provides them as a **locked-down shim**, not as an operating
system:

| Capability | Behaviour |
|---|---|
| File system | None. No directory is preopened; every descriptor call other than the two below fails with a WASI error such as `EBADF`; the conformance vectors check that opening a file fails. |
| Arguments and environment | Empty. The process environment is never visible; use `env_get`. |
| Sockets | None. |
| `fd_write` to descriptor 1 or 2 | Becomes `log` (1 at info, 2 at error), counted against `function.logBytes`. |
| `clock_time_get` | Reads the host clock, the same one as `clock_now`. |
| `random_get` | Reads the host's cryptographically secure random source. |
| Everything else | A WASI error; no host resource is reached. |

The shim is part of the ABI. A host on the device must behave the same, which the vectors check.
A module that imports a WASI function the host does not provide, or from any module other than
`plux_v1` and `wasi_snapshot_preview1`, is refused at load.

## 7. Versioning: `plux_abi_version`

`plux_abi_version()` returns the major version of the ABI the module was built for. This
document describes `1`, and imports live in `plux_v1`.

- Changes that an old module ignores stay in v1: a new optional field in a JSON payload, a new
  status a module may treat as `-7`.
- A change that an old module could misread is a **new major**: the imports move to `plux_v2`
  and `plux_abi_version` returns `2`.
- A host supports a declared set of majors and loads each module under the one it exports. A
  module is never translated. Hosts report the set they support, so a deploy of an unsupported
  version fails at once.

The host calls `plux_abi_version` in a throwaway instance at load, after the structural checks
and before any other guest code runs.

## 8. When a host refuses a module

Refusal happens at load, before any function runs.

| Condition | Code |
|---|---|
| The module does not compile or validate as WebAssembly. | `PLX-7018` `FUNCTION_MODULE_INVALID` |
| `memory` is missing or imported; a required export is missing or has the wrong signature. | `PLX-7018` |
| The module has a start section. | `PLX-7018` |
| An import outside `plux_v1` and the WASI set, an unknown `plux_v1` import, or an import with the wrong signature. | `PLX-7018` |
| A device host loads a module that imports a server-only function. | `PLX-7018` |
| The initial memory exceeds the host's maximum. | `PLX-7018` |
| `plux_abi_version` traps. | `PLX-7018` |
| `plux_abi_version` returns a major the host does not support. | `PLX-7017` `FUNCTION_ABI_UNSUPPORTED` |

On the server, `fnrunner` also verifies the artifact's signature and SHA-256 before it compiles a
module (`FN-009`, `PLX-7019`).

A capability that was not granted is not a load error. The call returns `-2` and the host never
serves it. A guest that traps right after a `-2` is reported as `PLX-7003` rather than as a
generic trap.

## 9. Writing a second host

A host that implements this document, and passes the vectors, can run every module built for ABI
v1. In order:

1. Compile the module and check its exports, imports and start section (section 8).
2. Create an instance with the shim of section 6, a memory capped at the call's limit, and a
   deadline that closes the instance from outside.
3. Call `_initialize` if present, then `plux_alloc` and `plux_invoke`, with the input.
4. Serve each import under the call's grants, counting HTTP calls, log bytes and call depth.
5. Decode the tagged result (section 3) and classify any trap.
6. Discard the instance.

The vectors state each expected outcome by case. The runner's environment, including the fixed
clock and the counting random source, is described in the vectors' README.
