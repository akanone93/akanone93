// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// A [DeviceFunctionHost] for the tests. It is sent to the worker isolate, so
/// it holds a directory (the key-value store) and a port (what the worker
/// observed), not live state.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:isolate';
import 'dart:typed_data';

import 'package:plux_functions/plux_functions.dart';

/// A host whose key-value namespace is a directory of files and which reports
/// what it saw on [events].
final class TestHost implements DeviceFunctionHost {
  /// Creates a host over [directory].
  const TestHost({
    required this.directory,
    this.events,
    this.environment = const {},
    this.clock = 1700000000000000000,
  });

  /// Where the key-value entries live.
  final String directory;

  /// Receives `['log', level, text]` and `['isolate', debugName]`.
  final SendPort? events;

  /// The values `env_get` answers.
  final Map<String, String> environment;

  /// What `clock_now` answers.
  final int clock;

  File _file(Uint8List key) =>
      File('$directory/${key.map((b) => b.toRadixString(16)).join()}');

  @override
  void log(FunctionLogLevel level, Uint8List message) => events?.send([
    'log',
    level.index,
    utf8.decode(message, allowMalformed: true),
  ]);

  @override
  int clockNowNanos() => clock;

  @override
  Uint8List context() {
    events?.send(['isolate', Isolate.current.debugName]);
    return Uint8List.fromList(
      utf8.encode(
        jsonEncode({
          'app': 'app-1',
          'environment': 'production',
          'placement': 'device',
          'assuranceLevel': 'standard',
        }),
      ),
    );
  }

  @override
  Uint8List envGet(Uint8List name) {
    final value = environment[utf8.decode(name)];
    if (value == null) throw HostStatus.notFound;
    return Uint8List.fromList(utf8.encode(value));
  }

  @override
  Uint8List kvGet(Uint8List key) {
    final file = _file(key);
    if (!file.existsSync()) throw HostStatus.notFound;
    return file.readAsBytesSync();
  }

  @override
  void kvPut(Uint8List key, Uint8List value) =>
      _file(key).writeAsBytesSync(value);

  @override
  void kvDelete(Uint8List key) {
    final file = _file(key);
    if (!file.existsSync()) throw HostStatus.notFound;
    file.deleteSync();
  }
}
