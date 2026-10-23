# Release-build inspection

A release build contains no Plux dev tooling, verbose logging or source maps (`SEC-074`), and
`plux_devtools` is gone from it (`DEV-050`). Compile-time constants remove the code
(`kReleaseMode`, `kDebugMode`, and `dataMocksAllowed` in `plux_flutter`); this directory proves
that they did.

```bash
make release-inspect                    # builds the starter app and Plux Bank for Android, then inspects the APKs
make release-inspect PLATFORM=ios       # the same for iOS (Xcode); inspects Runner.app, the content of an IPA
test/release/inspect.sh app.apk dir/    # inspects built artifacts or already extracted directories
```

`inspect.sh` fails when a build

- has no AOT snapshot (`libapp.so`, `App.framework/App`), which is a debug build or not a build;
- holds a string of [probes.txt](probes.txt) in that snapshot: the overlay UI of `plux_devtools`
  (the `devtools` probes) or diagnostics `plux_flutter` removes from release builds (`logging`);
- holds a P10 dev-session symbol (`p10`: `DevChannel`, `DevOverlayApplier`, `PluxDevSession`,
  `developmentReportOnly`, `/dev/socket`, `Plux-Dev-Ticket`, ...) or the `canary` constant
  `kDevCanary` of `devtools_api/dev_session.dart`, which only code behind `kPluxDevtools`
  references (`DEV-050`);
- ships `kernel_blob.bin`, a `.dill`, a JIT snapshot, a source map (`*.map`) or split-debug-info
  symbols (`*.symbols`).

The starter app wraps itself in `PluxDevtools`, so its release build is the proof that the overlay
is tree-shaken; Plux Bank is built as a store build is, with `--obfuscate --split-debug-info`. The
inspection runs in the `size-android` and `size-ios` CI jobs, which already build release apps.

`PluxDevtools` and its `State` stay in a release build (the widget returns its child), so their
names are not probes. A probe must occur verbatim in its package; a policy test
(`tools/internal/policy/release_test.go`) checks that, and runs the script on small extracted trees
to prove it fails on a planted devtools string, a source map, a debug artifact and a missing
snapshot, and passes a clean tree.

`make release-inspect-selftest` (part of `make repo-check`, no SDK needed) runs the scanner on
[fixtures/leaky.txt](fixtures/leaky.txt) (must fail, naming a symbol and the canary) and
[fixtures/clean.txt](fixtures/clean.txt) (must pass), and plants every probe alone to prove none
is dead.
