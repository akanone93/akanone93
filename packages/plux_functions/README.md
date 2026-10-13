<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# plux_functions

The device function engine of [Plux](https://nightcode42.github.io/plux3/), as an optional package of
[`plux_flutter`](https://pub.dev/packages/plux_flutter): apps that run no device-placed Plux
Functions do not pay for the interpreter.

A Plux Function is Rust compiled to WebAssembly. A function placed on the device runs here, in
[WAMR](https://github.com/bytecodealliance/wasm-micro-runtime)'s fast interpreter, built from vendored
source by `hook/build.dart`. Modules are never compiled to native code (`SEC-054`, `FN-050`).

```dart
final engine = await FunctionEngine.start();
final module = await engine.load(key: bundleModuleSha256, bytes: wasm);
final outcome = await engine.invoke(
  module: module,
  index: 0,
  input: utf8.encode('{"principal":"1000","annualRate":"0.12","months":3}'),
  limits: limits, // from the limits registry; the engine has no defaults
  host: host,
);
```

- **Off the UI isolate.** Every call into the interpreter runs on one long-lived worker isolate
  (`FN-053`). Invocations queue; each gets a fresh instance, so nothing survives one.
- **Limits.** Time (a watchdog thread terminates the instance), memory (every byte the invocation
  requests from the runtime), input, output and log bytes. Exceeding one throws a `PluxException`
  (`PLX-7001`, `7002`, `7022`, `7004`).
- **Cancellation.** A `FunctionCancellation` token ends a running or queued invocation with `PLX-7021`.
- **Refusal at load.** A module that imports a server-only function (`http_request`, `secret_get`,
  `claims`, `invoke_function`) is refused with `PLX-7018`; an unknown import or a wrong signature with `PLX-7018`, an unsupported
  `plux_abi_version` with `PLX-7017` (`FN-052`, ADR-0062).
- **Host.** The guest reaches `log`, the clock, the invocation context, `env_get` and the plugin's
  key-value namespace through a `DeviceFunctionHost`. The engine calls it on the worker isolate, so
  an implementation is sendable and keeps its state in a place the worker can reach.

`native/wamr/` holds the vendored WAMR 2.4.3 subset with a SHA-256 per file; `native/wamr/import.sh`
re-creates it from the upstream tag.
