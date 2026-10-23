<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12h — the Android size budget (`RT-061`)

CI run 39721074609: `make size-android` fails: `sizegate: plux_flutter adds 10537174 bytes to the android-armeabi-v7a-apk, over the 10.0 MiB of RT-061` (arm64 +8.63 MiB, x86_64 +8.91 MiB pass); almost all of it is `libapp.so` (+9.7 MB). P8 passed, so P9 added about 0.5 MiB or more to the Dart AOT code. Find what (`make size-android` writes a code-size breakdown under `build/size/code-size/`; you may not have the Android SDK here — then compare with `flutter build aot`-free means: `dart compile exe`-style size analysis is not equivalent, so instead diff the P9 runtime for large tables, generated code or dependencies added since `main`: `git diff --stat main...feat/phase-9 -- packages/plux_flutter/lib packages/plux_flutter/pubspec.yaml`, and look for big generated tables, eager initialisers, or packages pulled into the release build that only tests need). Reduce it below the budget without removing features (deferred/lazy tables, tree-shakeable code, moving test-only code out of `lib`). Never raise the budget. Report the before/after estimate and what you changed.
Done when: `cd packages/plux_flutter && dart analyze lib test && flutter test` pass, and your report states the bytes saved and how you measured them.
