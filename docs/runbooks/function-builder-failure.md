# Function Builder Failure Runbook

Function builds fail or never finish because the `builder` role is down, refuses to build, is
full, or rejects the source (`FN-009`, `FN-016`, `FN-019`,
[ADR-0063](../adr/0063-function-builds-provenance.md)). Background: the [builds
guide](../guides/function-builds.md). A build failure never affects functions that are already
deployed: versions are immutable and `fnrunner` does not depend on the `builder`.

## Symptoms

- `plux fn deploy` prints diagnostics and exits with a failure: the build ended in the state
  `failed`. The diagnostic carries a `PLX-72xx` code.
- `plux fn deploy` waits and reports no result: the build stays `queued` or `running`.
- Every build of every function fails with the same infrastructure error, including functions
  that built yesterday.
- `plux publish` fails for a plugin with device functions because "the device functions do not
  link": the same `builder` links a plugin's device functions into one module.
- A build the developer ran locally with `plux fn build` succeeds and the server's fails.

## Signals

There is no builder-specific metric yet. Use:

| Signal | Source |
|---|---|
| Builder reachable from the worker | `plux_rpc_requests_total` for the worker's calls, and the worker's log |
| Build outcomes | The `function build finished` audit entries (`failed` or succeeded) and `plux fn deploy` output |
| Builder activity | The builder's log: `build finished`, `the build failed in the builder`, `link finished` |
| Unsandboxed builder | A warning at start and with every build: `builder.sandbox is none` |

A failure of the role itself, as opposed to a source that does not compile, is an RPC error that
the worker retries, so a long outage shows as builds that stay `running` and a growing queue.
A source that does not compile is recorded as `failed` with diagnostics and is not retried.

## Diagnosis

1. **Read the diagnostic.** For a failed build it names the code. Match it:

| Code | Meaning | What to do |
|---|---|---|
| `PLX-7200` `FUNCTION_BUILD_FAILED` | Cargo or `rustc` failed | Read the compiler message in the diagnostic; reproduce with `plux fn build <name>` |
| `PLX-7201` `FUNCTION_DEPENDENCY_NOT_VENDORED` | A dependency is missing from `vendor/` or `Cargo.lock` | Run `cargo vendor` and commit the lock file; `plux fn deploy` packs both |
| `PLX-7202` `FUNCTION_BUILD_SCRIPT_REFUSED` | A crate with a build script or procedural macro is not on the allowlist | Use another crate, or ask the maintainers to review the crate and its version. The allowlist is a security control; it is not edited to make a build pass |
| `PLX-7203` `FUNCTION_BUILD_LIMIT_EXCEEDED` | Build time, build memory or the test-case count exceeded the registry | Slim the dependency tree; raise `function.buildTimeout` or `function.buildMemory` within their maxima if the build is legitimate |
| `PLX-7204` `FUNCTION_SCHEMA_DERIVATION_FAILED` | The schemas could not be read from the module | Check the input and output types derive `JsonSchema`, and the SDK in use is the pinned one |
| `PLX-7205` `FUNCTION_DECLARATION_MISMATCH` | `function.json` disagrees with the code or the request | Fix the placement or name; a device handler cannot be declared `server` and the reverse |
| `PLX-7206` `FUNCTION_BUILD_NOT_REPRODUCIBLE` | Two builds of the same input differ | See below |

2. **No diagnostic, nothing builds.** Work from the cheapest check.
   - **Is the role up?** Check the `builder` process or pod. A role that cannot start logs why: it
     requires mutual TLS, a listen address, `builder.wasmOpt`, `builder.sdkDir` and a
     concurrency of at least 1. Check offline with `plux-server config validate`.
   - **Can the worker reach it?** `builder.address` on the worker, the network policy, and the
     internal certificates (`SEC-043`). The worker needs `server.internalTLS`; an expired
     certificate fails every build.
   - **Does the sandbox work?** Where the kernel refuses new user, network, mount or PID
     namespaces, the builder refuses to build and says so in its log. Typical causes are a
     container runtime or security profile that forbids namespaces. The Helm chart's builder pod needs a runtime class that
     permits them.
   - **Are the tools there?** The pinned Rust toolchain (1.97.0, target `wasm32-wasip1`),
     `builder.cargo` and `builder.rustc` if set, the pinned `wasm-opt` command, and the SDK directory.

   - **Is it full?** More requests than `builder.concurrency` are refused as unavailable. Builds
     that run long hold slots; check `function.buildTimeout`.
   - **Disk.** The scratch directory (`builder.workDir`) fills with workspaces if cleanup fails.

3. **`PLX-7206` (not reproducible).** Something depends on the machine or the time. Build twice
   with `plux fn build <name> --verify` and compare. The usual causes are a `build.rs` that embeds
   a timestamp, an absolute path or a random value, a dependency resolved to a different
   version (the lock file is the contract), and a toolchain that is not the pinned one. If the
   function and its dependencies are unchanged and only the server disagrees with a local build,
   suspect the builder's toolchain or `wasm-opt`, and compare their versions with the pinned
   ones in the provenance of a good build.

## Mitigation

- **Source error:** fix the source; nothing is needed on the server.
- **Builder down or unreachable:** restart it, fix the address, the certificates or the network
  policy. Queued builds resume; running ones are retried by the worker.
- **Sandbox refused:** fix the runtime so namespaces are allowed. Do **not** set
  `builder.sandbox: none` in a shared or production installation to get builds through: it runs
  uploaded build scripts and procedural macros with the builder's privileges. The setting is for a
  developer's own machine, and it logs a warning at start and with every build.
- **Allowlist refusal:** the maintainers decide. Until then, remove the dependency.
- **Over limits:** reduce the build; raise the app's `function.buildTimeout` or
  `function.buildMemory` only to what the build needs, and no higher than the registry maximum.
- **Bad toolchain or `wasm-opt`:** deploy the builder image with the pinned versions. Never
  change the pins on one host.
- **Urgent fix and the builder is down:** a function cannot be deployed from a module you built
  yourself. There is no bypass, by design (`FN-009`). Restore the builder, or roll an alias back
  to an earlier version with `plux fn alias <name> <alias> <version> --env <env>`.

## Recovery

1. Re-run `plux fn deploy` for the functions that failed during the outage. A deploy that
   failed produced no version, so there is nothing to clean up.
2. For device functions, publish the plugin again.
3. Check one good build end to end: it succeeds, the version is listed by `plux fn versions`, and
   a call works in a test environment.
4. Record the cause. For `PLX-7206`, keep the two differing modules for the investigation.

## Prevention

- Run `plux fn build --verify` and `plux fn test` in CI, so a source error or a
  non-reproducible dependency is found before the server sees it.
- Keep the builder image, the toolchain, `wasm-opt` and the SDK pinned together, and change them
  in one change.
- Keep the allowlist small; a request to add a crate gets a review of the crate and its version.
- Size `builder.concurrency`, `builder.workDir` and the builder's memory for the largest build the
  registry allows.
- Monitor the builder's certificates and the namespace support of the nodes it runs on.
