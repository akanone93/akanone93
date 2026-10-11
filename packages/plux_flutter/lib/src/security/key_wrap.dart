// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Data keys wrapped by secure hardware (SEC-073, ADR-0058).
///
/// Each local store has one 256-bit data key, made on the device. It is
/// never stored in the clear: a hardware key — an Android Keystore AES key,
/// an iOS Secure Enclave key used with ECIES — wraps it, and the wrapped
/// bytes sit in the platform's secret storage. The runtime unwraps a key
/// once per launch and keeps it in memory; a read or write of a record
/// never calls the hardware.
library;

import 'dart:convert';

import 'package:flutter/services.dart';
import 'package:plux_flutter/src/platform/runtime_channel.dart';
import 'package:plux_flutter/src/store/kv_store.dart';

/// A hardware key that wraps and unwraps small secrets.
abstract interface class KeyWrapper {
  /// [data] wrapped by the hardware key, which is made when there is none.
  /// Throws [StoreException] when the platform cannot wrap.
  Future<Uint8List> wrap(Uint8List data);

  /// What [wrapped] holds, or null when the hardware key that wrapped it
  /// is gone or is not the one that did: a backup restored onto another
  /// device, a reinstall, a key the platform invalidated. Throws
  /// [StoreException] when the platform fails for another reason, so a
  /// transient failure never reads as a lost key.
  Future<Uint8List?> unwrap(Uint8List wrapped);
}

const _channel = RuntimeChannel();

/// The `dev.plux/runtime` channel's `dataKeyWrap` and `dataKeyUnwrap`
/// (`PluxDataKeys` on Android and iOS). The channel carries the data key to
/// the platform and back, as it does the secrets of `PlatformSecretStore`;
/// it never carries the hardware key, which cannot leave the platform.
///
/// The platform answers `PLUX_KEY_MISSING` when no hardware key can open the
/// bytes, and `PLUX_PLATFORM` for any other failure.
final class PlatformKeyWrapper implements KeyWrapper {
  /// Creates the wrapper.
  const PlatformKeyWrapper();

  @override
  Future<Uint8List> wrap(Uint8List data) async {
    final Object? reply;
    try {
      reply = await _channel.invokeMethod<Object?>('dataKeyWrap', {
        'data': data,
      });
    } on PlatformException {
      throw const StoreException(
        StoreFailure.unavailable,
        'the platform could not wrap a data key',
      );
    } on MissingPluginException {
      throw const StoreException(
        StoreFailure.unavailable,
        'the platform has no key wrapping',
      );
    }
    if (reply is! Uint8List || reply.isEmpty) {
      throw const StoreException(
        StoreFailure.unavailable,
        'the platform answered a data key wrap with a malformed reply',
      );
    }
    return reply;
  }

  @override
  Future<Uint8List?> unwrap(Uint8List wrapped) async {
    final Object? reply;
    try {
      reply = await _channel.invokeMethod<Object?>('dataKeyUnwrap', {
        'wrapped': wrapped,
      });
    } on PlatformException catch (e) {
      if (e.code == 'PLUX_KEY_MISSING') return null;
      throw const StoreException(
        StoreFailure.unavailable,
        'the platform could not unwrap a data key',
      );
    } on MissingPluginException {
      throw const StoreException(
        StoreFailure.unavailable,
        'the platform has no key wrapping',
      );
    }
    if (reply is! Uint8List) {
      throw const StoreException(
        StoreFailure.unavailable,
        'the platform answered a data key unwrap with a malformed reply',
      );
    }
    return reply;
  }
}

/// A [SecretStore] whose values are wrapped by a hardware key before they
/// reach [inner]: the data keys of the encrypted stores live in it
/// (`installationKey` reads and creates them), and `plux_db_drift` takes
/// its database key from it.
///
/// A value is unwrapped once and kept in memory for the life of this
/// object. The wrapped form binds the secret's name, so a blob copied
/// under another name does not unwrap. A value that [inner] holds unwrapped
/// — a key an earlier launch made before the stores were encrypted — is
/// wrapped on its first read, in place, so the key survives the change. A
/// value whose hardware key is gone is removed and reads as absent: the
/// caller makes a new key, and what the old one sealed is discarded as
/// unreadable (ADR-0049).
final class HardwareWrappedSecretStore implements SecretStore {
  /// Creates the store over [inner], wrapping with [wrapper].
  HardwareWrappedSecretStore(this.inner, this.wrapper);

  /// Where the wrapped values are kept.
  final SecretStore inner;

  /// The hardware key.
  final KeyWrapper wrapper;

  /// What a wrapped value starts with in [inner]. Base64 has no `.`, so a
  /// value written before wrapping never starts like this.
  static const String prefix = 'hw1.';

  final Map<String, String> _unwrapped = {};

  @override
  Future<String?> read(String name) async {
    final known = _unwrapped[name];
    if (known != null) return known;
    final stored = await inner.read(name);
    if (stored == null) return null;
    if (!stored.startsWith(prefix)) {
      await write(name, stored);
      return stored;
    }
    final String? value = await _open(name, stored.substring(prefix.length));
    if (value == null) {
      await inner.delete(name);
      return null;
    }
    return _unwrapped[name] = value;
  }

  @override
  Future<void> write(String name, String value) async {
    final wrapped = await wrapper.wrap(_payload(name, value));
    await inner.write(name, '$prefix${base64Encode(wrapped)}');
    _unwrapped[name] = value;
  }

  @override
  Future<void> delete(String name) async {
    _unwrapped.remove(name);
    await inner.delete(name);
  }

  Future<String?> _open(String name, String encoded) async {
    final Uint8List blob;
    try {
      blob = base64Decode(encoded);
    } on FormatException {
      return null;
    }
    final payload = await wrapper.unwrap(blob);
    if (payload == null || payload.length < 2) return null;
    final bound = (payload[0] << 8) | payload[1];
    if (payload.length < 2 + bound) return null;
    final String boundName;
    final String value;
    try {
      boundName = utf8.decode(payload.sublist(2, 2 + bound));
      value = utf8.decode(payload.sublist(2 + bound));
    } on FormatException {
      return null;
    }
    return boundName == name ? value : null;
  }

  /// The wrapped bytes: the name's length and the name, then the value.
  static Uint8List _payload(String name, String value) {
    final n = utf8.encode(name);
    if (n.length > 0xffff) throw ArgumentError.value(n.length, 'name');
    return Uint8List.fromList([
      n.length >> 8,
      n.length & 0xff,
      ...n,
      ...utf8.encode(value),
    ]);
  }
}
