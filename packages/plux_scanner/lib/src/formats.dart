// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:mobile_scanner/mobile_scanner.dart' as ms;

/// The `BarcodeFormat` members of the schema and the scanner's formats.
const Map<String, List<ms.BarcodeFormat>> _formats = {
  'qrCode': [ms.BarcodeFormat.qrCode],
  'aztec': [ms.BarcodeFormat.aztec],
  'dataMatrix': [ms.BarcodeFormat.dataMatrix],
  'pdf417': [ms.BarcodeFormat.pdf417],
  'code39': [ms.BarcodeFormat.code39],
  'code93': [ms.BarcodeFormat.code93],
  'code128': [ms.BarcodeFormat.code128],
  'codabar': [ms.BarcodeFormat.codabar],
  'ean8': [ms.BarcodeFormat.ean8],
  'ean13': [ms.BarcodeFormat.ean13],
  'itf': [ms.BarcodeFormat.itf14, ms.BarcodeFormat.itf2of5],
  'upcA': [ms.BarcodeFormat.upcA],
  'upcE': [ms.BarcodeFormat.upcE],
};

/// The schema's `BarcodeFormat` member for a scanner format, or null for
/// a format the schema does not know.
String? formatName(ms.BarcodeFormat format) {
  for (final e in _formats.entries) {
    if (e.value.contains(format)) return e.key;
  }
  return null;
}

/// The scanner formats for schema members [names]; empty for every format.
List<ms.BarcodeFormat> scannerFormats(List<String> names) => [
  for (final n in names) ...?_formats[n],
];
