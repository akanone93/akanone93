// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Scanning and QR display for Plux plugins.
///
/// * The `scanCode` action (RT-060, SEC-080): add [PluxScanner] to
///   `PluxConfig.devicePackages`. A plugin that scans declares the `camera`
///   device API in its capabilities and the app document approves it;
///   without this package the action fails with `PLX-5401`.
/// * The `QrScanner`, `BarcodeScanner` and `QrCodeView` widgets: add
///   [PluxScannerWidgets] to `PluxConfig.widgetPackages`.
///
/// The host declares the camera usage description (`NSCameraUsageDescription`)
/// and, on Android, the camera permission.
///
/// ## Engines (decision B21)
///
/// On Android the scanners use ML Kit's unbundled barcode model through
/// Google Play services (about 200 KB instead of about 2.4 MB). The model is
/// requested in the background at the first app open. Where Play services is
/// missing, or the model is not there yet, they fall back to ZXing-C++
/// through FFI, which works offline; set [PluxScannerConfig.engine] to
/// [ScannerEngine.zxing] to force it. iOS uses Vision.
///
/// The host app opts in to the unbundled model with one line in
/// `android/gradle.properties`, which `mobile_scanner` reads:
///
/// ```
/// dev.steenbakker.mobile_scanner.useUnbundled=true
/// ```
///
/// The fallback needs frames, and `mobile_scanner` hands out none: on
/// Android it opens its own CameraX camera ([AndroidFrameSource]), and
/// [PluxScannerConfig.frameSource] replaces it.
library;

export 'src/android_frames.dart' show AndroidFrameSource, maxFrameSide;
export 'src/engine.dart'
    show
        ChannelScannerProbe,
        MlKitState,
        PluxScannerConfig,
        ScannerEngine,
        ScannerProbe,
        ScannerTexts,
        resolveScannerEngine;
export 'src/package.dart' show PluxScannerIds, PluxScannerWidgets;
export 'src/qr_view.dart' show PluxQrCodeView, QrPainter, qrModules;
export 'src/scan_view.dart' show PluxScannerView;
export 'src/scanner.dart' show PluxScanner, ScanPageBuilder, ScanSink;
export 'src/session.dart'
    show
        MobileScannerSession,
        ScanErrorCodes,
        ScanFrameSource,
        ScanFrameSourceFactory,
        ScanHit,
        ScanRequest,
        ScanSession,
        ScanSessionFactory,
        ScanSourceException,
        ZxingSession,
        cropFrame,
        windowInSensor;
export 'src/zxing_ffi.dart'
    show ScanFrame, ZxingDecoder, ZxingResult, formatMask, scanFormatNames;
