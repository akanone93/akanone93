// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/security/confidential.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

import '../sync/confidential_support.dart';

/// The cost of decrypting a confidential bundle (NFR-013, SEC-053): the
/// signed hash and every section hash checked, every section opened with
/// AES-256-GCM, the plain container laid out — [decryptBundle] end to end,
/// per MiB of plaintext, p95 over repeated runs of a 4 MiB bundle.
///
/// On the host this is pure Dart ([DartAead]), an upper bound; the platform
/// cipher (ADR-0058) on the reference device is measured by the device
/// benchmark. Not gated; `make bench-confidential` runs it.
void main() {
  const sectionBytes = 256 * 1024;
  const sections = 16; // 4 MiB of plaintext.
  const warmUp = 2;
  const iterations = 20;

  test(
    'measures decrypting a confidential bundle [NFR-013] [SEC-053]',
    () {
      final key = Uint8List.fromList([for (var i = 0; i < 32; i++) i * 7]);
      final plain = encodeBundle(BundleKinds.plugin, [
        for (var i = 0; i < sections; i++)
          Section(
            SectionKind.page,
            Uint8List(16)..[15] = i,
            Uint8List(0),
            Uint8List.fromList([
              for (var j = 0; j < sectionBytes; j++) (i + j * 31) & 0xff,
            ]),
          ),
      ]);
      final sealed = sealBundle(plain, key, 0);
      final hash = Uint8List.fromList(sealed.sublist(16, 48));
      final mib = sections * sectionBytes / (1024 * 1024);
      for (var i = 0; i < warmUp; i++) {
        decryptBundle(sealed, hash, key);
      }
      final perMiB = <double>[];
      final w = Stopwatch();
      for (var i = 0; i < iterations; i++) {
        w
          ..reset()
          ..start();
        final out = decryptBundle(sealed, hash, key);
        w.stop();
        expect(out.length, plain.length);
        perMiB.add(w.elapsedMicroseconds / 1000 / mib);
      }
      perMiB.sort();
      double q(double p) => double.parse(
        perMiB[((perMiB.length - 1) * p).round()].toStringAsFixed(3),
      );
      // ignore: avoid_print
      print(
        jsonEncode({
          'confidential_decrypt': {
            'implementation': 'pure_dart',
            'plaintext_mib': mib,
            'iterations': iterations,
            'p50_ms_per_mib': q(0.5),
            'p95_ms_per_mib': q(0.95),
            'p99_ms_per_mib': q(0.99),
            'budget_ms_per_mib': 10,
          },
        }),
      );
      expect(hexEncode(hash), hasLength(64));
    },
    timeout: const Timeout(Duration(minutes: 5)),
    skip: Platform.environment['PLUX_BENCH_CONFIDENTIAL'] == null
        ? 'set PLUX_BENCH_CONFIDENTIAL=1, or run make bench-confidential'
        : false,
  );
}
