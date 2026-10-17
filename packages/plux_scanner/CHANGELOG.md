<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Changelog

## Unreleased

- `QrScanner`, `BarcodeScanner` and `QrCodeView` (`PluxScannerWidgets`): an inline camera preview with a viewfinder, torch, debounced continuous detection, a live-region announcement and an error state with retry; a QR code painted from the module matrix of `qr`.
- Decision B21: on Android the scanners use ML Kit's unbundled barcode model through Google Play services (about 200 KB instead of about 2.4 MB), requested in the background at the first app open; where Play services is missing or the model is not there, ZXing-C++ 2.3.0 reads frames through FFI, offline (`PluxScannerConfig.engine`); the fallback's default camera is CameraX in the package's Android plugin (`AndroidFrameSource`: preview in a Flutter texture, luminance frames at up to 10 a second), replaceable through `PluxScannerConfig.frameSource`. The host adds `dev.steenbakker.mobile_scanner.useUnbundled=true` to `android/gradle.properties`. iOS keeps Vision.
- `scanCode` now runs on the same scanner view.

## 0.1.0

The first release: `PluxScanner`, the device package behind the `scanCode` action, on `mobile_scanner` (ML Kit on Android).
