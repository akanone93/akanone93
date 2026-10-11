// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/platform_aead.dart';
import 'package:plux_flutter/src/security/record_cipher.dart';

/// The cost of sealing and opening one record of an encrypted store
/// (NFR-013, SEC-073, ADR-0058), per size, through [RecordCipher]: the
/// number that decides whether the platform's AES-GCM is worth its call
/// overhead against [DartAead], which P5 measured at ten times slower than
/// native for bulk data.
///
/// On a host with no platform implementation only [DartAead] is measured
/// and `platform` is null. A phone's CryptoKit and Conscrypt are measured on
/// the device, with this same loop. Not gated; set `PLUX_BENCH_AEAD=1` to run
/// it.
void main() {
  const warmUp = 200;
  final key = Uint8List.fromList(List.generate(32, (i) => i));

  Map<String, Object?> measure(Aead aead, int size, int iterations) {
    final cipher = RecordCipher(aead);
    final clear = Uint8List.fromList(List.generate(size, (i) => i % 251));
    final nonce = RecordCipher.newNonce();
    final seal = <int>[];
    final open = <int>[];
    final w = Stopwatch();
    for (var i = 0; i < warmUp + iterations; i++) {
      w
        ..reset()
        ..start();
      final record = cipher.seal(key, 'bench', 'k', clear, nonce: nonce);
      final sealed = w.elapsedMicroseconds;
      w
        ..reset()
        ..start();
      final back = cipher.open(key, 'bench', 'k', record);
      final opened = w.elapsedMicroseconds;
      if (back == null) throw StateError('the record did not open');
      if (i >= warmUp) {
        seal.add(sealed);
        open.add(opened);
      }
    }
    double us(List<int> v, double q) {
      final s = [...v]..sort();
      return s[((s.length - 1) * q).round()].toDouble();
    }

    return {
      'bytes': size,
      'iterations': iterations,
      'seal_p50_us': us(seal, 0.5),
      'seal_p95_us': us(seal, 0.95),
      'open_p50_us': us(open, 0.5),
      'open_p95_us': us(open, 0.95),
    };
  }

  test(
    'measures the per-record cost of AES-256-GCM [NFR-013] [SEC-073]',
    () {
      const sizes = {64: 2000, 1024: 2000, 65536: 200};
      final platform = PlatformAead.tryCreate();
      final result = {
        'aead_record': {
          'dart': [
            for (final MapEntry(:key, :value) in sizes.entries)
              measure(const DartAead(), key, value),
          ],
          'platform': platform == null
              ? null
              : [
                  for (final MapEntry(:key, :value) in sizes.entries)
                    measure(platform, key, value),
                ],
        },
      };
      // ignore: avoid_print
      print(jsonEncode(result));
    },
    timeout: const Timeout(Duration(minutes: 5)),
    skip: Platform.environment['PLUX_BENCH_AEAD'] == null
        ? 'set PLUX_BENCH_AEAD=1'
        : false,
  );
}
