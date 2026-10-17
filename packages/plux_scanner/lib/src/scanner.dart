// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_scanner/src/engine.dart';
import 'package:plux_scanner/src/scan_view.dart';
import 'package:plux_scanner/src/session.dart';

export 'package:plux_scanner/src/formats.dart' show formatName, scannerFormats;

/// What a scan page reports. It reports once; later calls are ignored.
abstract interface class ScanSink {
  /// A code was read.
  void found(PluxScanResult result);

  /// The scan cannot go on: the camera is denied or missing.
  void fail(PluxDeviceException error);

  /// The user gave up.
  void cancel();
}

/// Builds the page that scans codes of [formats], `BarcodeFormat` member
/// names (every format when empty), and reports to [sink].
typedef ScanPageBuilder = Widget Function(
  BuildContext context,
  List<String> formats,
  ScanSink sink,
);

/// The `scanCode` action for Plux plugins (RT-060, SEC-080): a full-screen
/// camera page that returns the first code it reads, on the same engines as
/// the scanner widgets (ML Kit through Play services with a ZXing-C++
/// fallback on Android, Vision on iOS).
///
/// Register it in `PluxConfig.devicePackages` with the key of the host's
/// root navigator: `PluxScanner(navigatorKey: navigatorKey)`. Plugins must
/// declare the `camera` device API, and the app must approve it.
final class PluxScanner implements PluxDevicePackage {
  /// Creates the package. The scan page opens on the navigator of
  /// [navigatorKey]; [pageBuilder] replaces the camera page.
  PluxScanner({
    required GlobalKey<NavigatorState> navigatorKey,
    PluxScannerConfig config = const PluxScannerConfig(),
    @visibleForTesting ScanPageBuilder? pageBuilder,
  }) : _scanner = _Scanner(
         navigatorKey,
         pageBuilder ??
             (context, formats, sink) =>
                 _CameraPage(formats: formats, sink: sink, config: config),
       );

  final _Scanner _scanner;

  @override
  String get name => 'plux_scanner';

  @override
  Map<Type, Object> get services => {PluxCodeScanner: _scanner};
}

final class _Scanner implements PluxCodeScanner {
  _Scanner(this._navigator, this._page);

  final GlobalKey<NavigatorState> _navigator;
  final ScanPageBuilder _page;

  @override
  Future<PluxScanResult?> scan(List<String> formats) {
    final nav = _navigator.currentState;
    if (nav == null) {
      return Future.error(
        const PluxDeviceException.unavailable(
          'there is no navigator to show the scanner on',
        ),
      );
    }
    final done = Completer<PluxScanResult?>();
    late final MaterialPageRoute<void> route;
    void finish(PluxScanResult? result, [PluxDeviceException? error]) {
      if (done.isCompleted) return;
      if (error != null) {
        done.completeError(error);
      } else {
        done.complete(result);
      }
      if (route.isCurrent) {
        nav.pop();
      } else if (route.isActive) {
        nav.removeRoute(route);
      }
    }

    final sink = _Sink(finish);
    route = MaterialPageRoute<void>(
      fullscreenDialog: true,
      builder: (context) => _page(context, formats, sink),
    );
    // Leaving the page by the back button or a swipe is a cancel.
    unawaited(route.popped.then((_) => finish(null)));
    unawaited(nav.push(route));
    return done.future;
  }
}

final class _Sink implements ScanSink {
  const _Sink(this._finish);

  final void Function(PluxScanResult?, [PluxDeviceException?]) _finish;

  @override
  void found(PluxScanResult result) => _finish(result);

  @override
  void fail(PluxDeviceException error) => _finish(null, error);

  @override
  void cancel() => _finish(null);
}

final class _CameraPage extends StatelessWidget {
  const _CameraPage({
    required this.formats,
    required this.sink,
    required this.config,
  });

  final List<String> formats;
  final ScanSink sink;
  final PluxScannerConfig config;

  void _failed(String code) {
    // The message holds the error code only (SEC-092).
    sink.fail(
      code == ScanErrorCodes.permissionDenied
          ? const PluxDeviceException.denied('the camera permission was denied')
          : PluxDeviceException.unavailable('the scanner failed: $code'),
    );
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: Colors.black,
    appBar: AppBar(
      backgroundColor: Colors.transparent,
      foregroundColor: Colors.white,
      leading: IconButton(
        icon: const Icon(Icons.close),
        tooltip: MaterialLocalizations.of(context).closeButtonTooltip,
        onPressed: sink.cancel,
      ),
    ),
    extendBodyBehindAppBar: true,
    body: PluxScannerView(
      config: config,
      formats: formats,
      onDetected: (hit) =>
          sink.found(PluxScanResult(value: hit.value, format: hit.format)),
      onError: _failed,
    ),
  );
}
