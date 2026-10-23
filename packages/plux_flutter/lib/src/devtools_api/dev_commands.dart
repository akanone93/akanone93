// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Carries out the commands of a dev session on the device (`DEV-011`,
/// `DEV-031`): opening a screen with a route, parameters, locale, theme,
/// text scale and mock selection, setting state, switching mocks, faking the
/// clock and simulating lifecycle events. Commands that later slices own
/// are dispatched to the handlers registered for them; the rest are
/// unsupported (`PLX-9232`). Only builds with `kPluxDevtools` create it.
library;

import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:plux_flutter/src/core/app_state.dart';
import 'package:plux_flutter/src/core/runtime.dart';
import 'package:plux_flutter/src/data/mocks.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart';
import 'package:plux_flutter/src/state/providers.dart';
import 'package:plux_flutter/src/state/scope_state.dart';

const _ok = 'DEV_COMMAND_STATUS_OK';
const _failed = 'DEV_COMMAND_STATUS_FAILED';
const _unsupported = 'DEV_COMMAND_STATUS_UNSUPPORTED';
const _refused = 'DEV_COMMAND_STATUS_REFUSED';

/// The command a handler cannot carry out: [status] and the reason.
final class _Rejected implements Exception {
  const _Rejected(this.status, this.message, [this.code]);

  final String status;
  final String message;
  final PluxErrorCode? code;
}

/// Resolves a state scope the runtime does not own by itself, such as
/// `page:<route>` or `component:<node path>`, to its values; null when no
/// such instance is on screen. [state] is the scope's current values.
typedef DevScopeResolver =
    ({ScopeValues values, Map<String, Object?> state})? Function(String scope);

/// Carries out dev session commands on [runtime].
final class RuntimeDevCommands implements DevCommandHandler {
  /// Creates the handler.
  ///
  /// [container] is the one the runtime's providers live in. [extensions]
  /// carry out the commands of later slices, keyed by the name of the
  /// command's oneof field (for example `networkProfile`). [scopes]
  /// resolves the page and component state scopes. [lifecycle] receives the
  /// simulated lifecycle events; it is the runtime unless a caller
  /// replaces it.
  RuntimeDevCommands({
    required this.runtime,
    required this.container,
    this.extensions = const {},
    this.scopes,
    WidgetsBindingObserver? lifecycle,
  }) : lifecycle = lifecycle ?? runtime;

  /// The runtime the commands act on.
  final PluxRuntime runtime;

  /// The container of the runtime's providers.
  final ProviderContainer container;

  /// Handlers of the commands other slices own, by oneof field name.
  final Map<String, DevCommandHandler> extensions;

  /// Resolves page and component state scopes.
  final DevScopeResolver? scopes;

  /// Receives the simulated lifecycle events.
  final WidgetsBindingObserver lifecycle;

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    try {
      final entries = command.entries
          .where((e) => e.value != null)
          .toList(growable: false);
      if (entries.length != 1) {
        throw const _Rejected(
          _failed,
          'a command names exactly one command field',
        );
      }
      final MapEntry(key: name, value: body) = entries.single;
      final extension = extensions[name];
      if (extension != null) return await extension.handle(command);
      if (body is! Map<String, Object?>) {
        throw _Rejected(_failed, '$name is not an object');
      }
      switch (name) {
        case 'openScreen':
          await _openScreen(body);
        case 'setState':
          _setState(body);
        case 'switchMock':
          _switchMock(body);
        case 'fakeClock':
          _fakeClock(body);
        case 'lifecycle':
          _lifecycle(body);
        case 'networkProfile' ||
            'simulateSecurityDetection' ||
            'evaluatePxl' ||
            'captureScreenshot' ||
            'inspectSelect' ||
            'toggleOverlay':
          throw _Rejected(
            _unsupported,
            'this runtime does not carry out $name',
            PluxErrorCode.devCommandUnsupported,
          );
        default:
          throw _Rejected(_failed, 'unknown command $name');
      }
      return {'status': _ok};
    } on _Rejected catch (r) {
      return _result(r.status, r.message, r.code);
    } on Object catch (e) {
      return _result(_failed, 'the command failed: ${e.runtimeType}', null);
    }
  }

  Map<String, Object?> _result(
    String status,
    String message,
    PluxErrorCode? code,
  ) => {
    'status': status,
    'error': {
      if (code != null) 'code': code.id,
      if (code != null) 'reason': code.reason,
      'message': message,
    },
  };

  Future<void> _openScreen(Map<String, Object?> c) async {
    final route = _string(c, 'route');
    if (route.isEmpty) {
      throw const _Rejected(_failed, 'openScreen needs a route');
    }
    final params = <String, Object?>{};
    final rawParams = c['params'];
    if (rawParams != null) {
      if (rawParams is! Map<String, Object?>) {
        throw const _Rejected(_failed, 'params is not an object');
      }
      for (final MapEntry(:key, :value) in rawParams.entries) {
        if (value is! String) {
          throw _Rejected(_failed, 'parameter $key is not text');
        }
        params[key] = value;
      }
    }
    final locale = _locale(_string(c, 'locale'));
    final theme = _theme(c['theme']);
    final scale = _number(c, 'textScale');
    if (scale != null && (scale.isNaN || scale.isNegative)) {
      throw const _Rejected(_failed, 'textScale is not a scale factor');
    }
    final mocks = c['mocks'] == null ? null : _mockList(c['mocks']);

    if (locale != null &&
        !container.read(localeControllerProvider.notifier).request(locale)) {
      throw _Rejected(
        _failed,
        'the release has no locale ${locale.toLanguageTag()}',
        PluxErrorCode.localeNotInRelease,
      );
    }
    if (theme != null) {
      final env = container.read(environmentProvider);
      container
          .read(environmentProvider.notifier)
          .replace(
            PluxEnvironment(
              locale: env.locale,
              themeMode: theme,
              brand: env.brand,
              consent: env.consent,
              user: env.user,
              authDelegate: env.authDelegate,
            ),
          );
    }
    final preview = runtime.devPreview;
    preview.value = preview.value.copyWith(
      locale: locale,
      themeMode: theme,
      textScale: scale != null && scale > 0 ? scale : null,
    );
    if (mocks != null) runtime.dataMocks.replaceAll(mocks);

    if (!await runtime.openRoute(route, params)) {
      throw _Rejected(
        _failed,
        'the route $route could not be opened',
        PluxErrorCode.navigationRefused,
      );
    }
  }

  void _setState(Map<String, Object?> c) {
    final scope = _string(c, 'scope');
    final path = _string(c, 'path');
    if (path.isEmpty) throw const _Rejected(_failed, 'setState needs a path');
    final encoded = c['value'];
    if (encoded is! String) {
      throw const _Rejected(_failed, 'value is not base64 JSON bytes');
    }
    final Object? value;
    try {
      value = jsonDecode(utf8.decode(base64.decode(encoded)));
    } on FormatException {
      throw const _Rejected(_failed, 'value is not base64-encoded JSON');
    }
    final (values, state) = _scopeValues(scope);
    final parts = path.split('.');
    final name = parts.first;
    final decl = values.model?.byName[name];
    if (decl != null && decl.sensitive) {
      throw _Rejected(
        _refused,
        'state $name is sensitive and cannot be set from a session',
        PluxErrorCode.devCommandNotAllowed,
      );
    }
    try {
      if (parts.length == 1) {
        values.writeEntry(name, value);
      } else {
        final current = state[name];
        values.writeEntry(
          name,
          _withPath(current, parts.skip(1).toList(), value),
        );
      }
    } on StateWriteException catch (e) {
      throw _Rejected(_failed, e.message, e.code);
    }
  }

  (ScopeValues, Map<String, Object?>) _scopeValues(String scope) {
    if (scope == 'app') {
      return (
        container.read(appStateProvider.notifier),
        container.read(appStateProvider),
      );
    }
    if (scope.startsWith('plugin:')) {
      final provider = pluginStateProvider(scope.substring('plugin:'.length));
      return (container.read(provider.notifier), container.read(provider));
    }
    final resolved = scopes?.call(scope);
    if (resolved == null) {
      throw _Rejected(_failed, 'no state scope $scope is on screen');
    }
    return (resolved.values, resolved.state);
  }

  Object? _withPath(Object? current, List<String> path, Object? value) {
    if (path.isEmpty) return value;
    final base = current is Map<String, Object?>
        ? {...current}
        : <String, Object?>{};
    base[path.first] = _withPath(
      base[path.first],
      path.skip(1).toList(),
      value,
    );
    return base;
  }

  void _switchMock(Map<String, Object?> c) {
    final source = _string(c, 'source');
    if (source.isEmpty) {
      throw const _Rejected(_failed, 'switchMock needs a source');
    }
    final state = _string(c, 'state');
    runtime.dataMocks.select(source, state.isEmpty ? null : _mockState(state));
  }

  void _fakeClock(Map<String, Object?> c) {
    final clock = runtime.devClock;
    final set = _int(c, 'setUnixMicros');
    final advance = _int(c, 'advanceMicros');
    final reset = c['reset'];
    final given = [set != null, advance != null, reset == true];
    if (given.where((g) => g).length != 1) {
      throw const _Rejected(
        _failed,
        'fakeClock needs exactly one of setUnixMicros, advanceMicros, reset',
      );
    }
    if (set != null) {
      clock.set(Duration(microseconds: set));
    } else if (advance != null) {
      if (!clock.isFaked) {
        throw const _Rejected(_failed, 'the clock is not fixed: set it first');
      }
      if (advance < 0) throw const _Rejected(_failed, 'time cannot go back');
      clock.advance(Duration(microseconds: advance));
    } else {
      clock.reset();
    }
  }

  void _lifecycle(Map<String, Object?> c) {
    switch (_enumName(c['event'], 'DEV_LIFECYCLE_EVENT_', const [
      'UNSPECIFIED',
      'PAUSE',
      'RESUME',
      'INACTIVE',
      'DETACHED',
      'MEMORY_PRESSURE',
    ])) {
      case 'PAUSE':
        lifecycle.didChangeAppLifecycleState(AppLifecycleState.paused);
      case 'RESUME':
        lifecycle.didChangeAppLifecycleState(AppLifecycleState.resumed);
      case 'INACTIVE':
        lifecycle.didChangeAppLifecycleState(AppLifecycleState.inactive);
      case 'DETACHED':
        lifecycle.didChangeAppLifecycleState(AppLifecycleState.detached);
      case 'MEMORY_PRESSURE':
        lifecycle.didHaveMemoryPressure();
      default:
        throw const _Rejected(_failed, 'lifecycle needs an event');
    }
  }

  // Proto3 JSON reading: numbers and enums may arrive as text or numbers,
  // and 64-bit integers as text.

  String _string(Map<String, Object?> c, String key) {
    final v = c[key];
    if (v == null) return '';
    if (v is! String) throw _Rejected(_failed, '$key is not text');
    return v;
  }

  double? _number(Map<String, Object?> c, String key) {
    final v = c[key];
    if (v == null) return null;
    final n = v is num
        ? v.toDouble()
        : (v is String ? double.tryParse(v) : null);
    if (n == null) throw _Rejected(_failed, '$key is not a number');
    return n;
  }

  int? _int(Map<String, Object?> c, String key) {
    final v = c[key];
    if (v == null) return null;
    final n = v is int ? v : (v is String ? int.tryParse(v) : null);
    if (n == null) throw _Rejected(_failed, '$key is not an integer');
    return n;
  }

  String _enumName(Object? v, String prefix, List<String> names) {
    if (v == null) return 'UNSPECIFIED';
    if (v is int && v >= 0 && v < names.length) return names[v];
    if (v is String) {
      final bare = v.startsWith(prefix) ? v.substring(prefix.length) : v;
      if (names.contains(bare)) return bare;
    }
    throw _Rejected(_failed, 'unknown value $v');
  }

  Locale? _locale(String tag) {
    if (tag.isEmpty) return null;
    final parts = tag.split(RegExp('[-_]'));
    final language = parts.first;
    if (!RegExp(r'^[A-Za-z]{2,8}$').hasMatch(language)) {
      throw _Rejected(_failed, '$tag is not a BCP 47 tag');
    }
    String? script;
    String? region;
    for (final p in parts.skip(1)) {
      if (script == null &&
          region == null &&
          RegExp(r'^[A-Za-z]{4}$').hasMatch(p)) {
        script = p;
      } else if (region == null &&
          RegExp(r'^([A-Za-z]{2}|[0-9]{3})$').hasMatch(p)) {
        region = p;
      } else {
        throw _Rejected(_failed, '$tag is not a supported BCP 47 tag');
      }
    }
    return Locale.fromSubtags(
      languageCode: language.toLowerCase(),
      scriptCode: script == null
          ? null
          : script[0].toUpperCase() + script.substring(1).toLowerCase(),
      countryCode: region?.toUpperCase(),
    );
  }

  ThemeMode? _theme(Object? v) => switch (_enumName(
    v,
    'DEV_THEME_MODE_',
    const ['UNSPECIFIED', 'LIGHT', 'DARK', 'SYSTEM'],
  )) {
    'LIGHT' => ThemeMode.light,
    'DARK' => ThemeMode.dark,
    'SYSTEM' => ThemeMode.system,
    _ => null,
  };

  Map<String, DataMockState> _mockList(Object? v) {
    if (v is! List<Object?>) {
      throw const _Rejected(_failed, 'mocks is not a list');
    }
    final out = <String, DataMockState>{};
    for (final m in v) {
      if (m is! Map<String, Object?>) {
        throw const _Rejected(_failed, 'a mock selection is not an object');
      }
      final source = _string(m, 'source');
      if (source.isEmpty) {
        throw const _Rejected(_failed, 'a mock selection needs a source');
      }
      out[source] = _mockState(_string(m, 'state'));
    }
    return out;
  }

  DataMockState _mockState(String state) {
    for (final s in DataMockState.values) {
      if (s.name == state) return s;
    }
    throw _Rejected(_failed, 'unknown mock state $state');
  }
}
