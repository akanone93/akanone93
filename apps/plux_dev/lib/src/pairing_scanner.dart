// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Scanning the pairing QR code with `plux_scanner` (DEV-003).
library;

import 'package:flutter/material.dart';
import 'package:plux_scanner/plux_scanner.dart';

/// Opens the camera on [navigator] and returns the text of the first QR
/// code read, or null when the user backs out or the camera fails. The text
/// is only offered to the pairing, which checks it (`DevPairingLink`).
/// [scannerBuilder] replaces the camera view in tests.
Future<String?> scanPairingCode(
  NavigatorState navigator, {
  PairingScannerBuilder scannerBuilder = cameraScanner,
}) => navigator.push<String>(
  MaterialPageRoute<String>(builder: (_) => _ScanPage(builder: scannerBuilder)),
);

/// Builds the scanner view; [onValue] gets the text of a code read and
/// [onGiveUp] is called when the camera fails.
typedef PairingScannerBuilder = Widget Function(
  void Function(String value) onValue,
  VoidCallback onGiveUp,
);

/// The camera view: `plux_scanner` reading QR codes. Needs a device.
Widget cameraScanner(
  void Function(String value) onValue,
  VoidCallback onGiveUp,
) => PluxScannerView(
  config: const PluxScannerConfig(),
  formats: const ['qrCode'],
  overlayHint: 'Point the camera at the QR code in Studio or the CLI',
  semanticLabel: 'Pairing code scanner',
  onDetected: (hit) => onValue(hit.value),
  onError: (_) => onGiveUp(),
);

final class _ScanPage extends StatelessWidget {
  const _ScanPage({required this.builder});

  final PairingScannerBuilder builder;

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Scan the pairing code')),
    body: builder(
      (value) => Navigator.of(context).pop(value),
      () => Navigator.of(context).pop(),
    ),
  );
}
