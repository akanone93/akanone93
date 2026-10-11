// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:math';
import 'dart:typed_data';

import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/key_wrap.dart';
import 'package:plux_flutter/src/store/kv_store.dart';

/// A hardware key in memory: AES-GCM under a key that never leaves this
/// object. [lost] stands for a key the platform has since discarded, and
/// [failing] for a transient platform failure.
final class FakeHardware implements KeyWrapper {
  FakeHardware([int seed = 1])
    : _key = Uint8List.fromList(
        List.generate(32, (i) => (i * 7 + seed) & 0xff),
      );

  Uint8List _key;
  int wraps = 0;
  int unwraps = 0;
  bool lost = false;
  bool failing = false;
  final _random = Random(5);

  /// The platform made a new hardware key: what the old one wrapped is gone.
  void replaceKey() => _key = Uint8List.fromList(_key.reversed.toList());

  @override
  Future<Uint8List> wrap(Uint8List data) async {
    wraps++;
    if (failing) {
      throw const StoreException(StoreFailure.unavailable, 'wrap');
    }
    final nonce = Uint8List.fromList(
      List.generate(12, (_) => _random.nextInt(256)),
    );
    return Uint8List.fromList([
      ...nonce,
      ...const DartAead().seal(_key, nonce, data, Uint8List(0)),
    ]);
  }

  @override
  Future<Uint8List?> unwrap(Uint8List wrapped) async {
    unwraps++;
    if (failing) {
      throw const StoreException(StoreFailure.unavailable, 'unwrap');
    }
    if (lost) return null;
    return const DartAead().open(
      _key,
      wrapped.sublist(0, 12),
      wrapped.sublist(12),
      Uint8List(0),
    );
  }
}
