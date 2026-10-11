// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// Identifiers that name the hardware or the user across apps. A device
/// is known to the server only by a random UUIDv7 the server mints at
/// registration (SEC-160), so none of these is ever read.
final _hardwareIdentifiers = <String, RegExp>{
  'IMEI': RegExp(r'\bIMEI\b', caseSensitive: false),
  'MEID': RegExp(r'\bMEID\b'),
  'serial number': RegExp(r'Build\.(getSerial|SERIAL)|\bgetSerial\b'),
  'ANDROID_ID': RegExp(r'ANDROID_ID|Settings\.Secure\.getString'),
  'IDFV': RegExp(r'\bIDFV\b|identifierForVendor'),
  'IDFA': RegExp(r'\bIDFA\b|ASIdentifierManager|advertisingIdentifier'),
  'advertising id': RegExp(r'AdvertisingIdClient|advertisingId'),
  'MAC address': RegExp(r'getMacAddress|\bmacAddress\b|WifiInfo'),
  'telephony id': RegExp(r'getDeviceId|getSubscriberId|getSimSerialNumber'),
};

const _sourceExtensions = {'.dart', '.kt', '.java', '.swift', '.m', '.h'};

Iterable<File> _sources(Directory root) => root
    .listSync(recursive: true)
    .whereType<File>()
    .where(
      (f) => _sourceExtensions.any(f.path.endsWith) && !_generated(f.path),
    );

bool _generated(String path) =>
    path.contains('/build/') || path.contains('/.dart_tool/');

List<String> _hits(Directory root) => [
  for (final f in _sources(root))
    for (final MapEntry(:key, :value) in _hardwareIdentifiers.entries)
      if (value.hasMatch(f.readAsStringSync())) '${f.path}: $key',
];

void main() {
  test('plux_flutter reads no hardware identifier, in Dart or in the native '
      'plugins [SEC-160]', () {
    final package = Directory.current;
    for (final dir in ['lib', 'android', 'ios']) {
      expect(
        Directory('${package.path}/$dir').existsSync(),
        isTrue,
        reason: dir,
      );
      expect(_hits(Directory('${package.path}/$dir')), isEmpty, reason: dir);
    }
  });

  test('the scan finds an identifier that is planted [SEC-160]', () {
    final dir = Directory.systemTemp.createTempSync('plux_privacy_');
    addTearDown(() => dir.deleteSync(recursive: true));
    File('${dir.path}/Ids.kt').writeAsStringSync(
      'val id = Settings.Secure.getString(r, Settings.Secure.ANDROID_ID)\n',
    );
    File('${dir.path}/ids.dart').writeAsStringSync('const runtimeInfo = 1;\n');
    expect(_hits(dir), hasLength(1));
    expect(_hits(dir).single, contains('ANDROID_ID'));
  });
}
