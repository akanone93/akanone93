// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Scanning the pairing QR code with `plux_scanner` (DEV-003).
library;

import 'package:flutter/material.dart';
import 'package:plux_scanner/plux_scanner.dart';

/// Opens the camera on [navigator] and returns the text of the first QR
/// code read, or null when the user backs out or the camera fails. The text
/// is only offered to the pairing, which checks it (`DevPairingLink`).
Future<String?> scanPairingCode(NavigatorState navigator) => navigator
    .push<String>(MaterialPageRoute<String>(builder: (_) => const _ScanPage()));

final class _ScanPage extends StatelessWidget {
  const _ScanPage();

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Scan the pairing code')),
    body: PluxScannerView(
      config: const PluxScannerConfig(),
      formats: const ['qrCode'],
      overlayHint: 'Point the camera at the QR code in Studio or the CLI',
      semanticLabel: 'Pairing code scanner',
      onDetected: (hit) => Navigator.of(context).pop(hit.value),
      onError: (_) => Navigator.of(context).pop(),
    ),
  );
}
