// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/core/app_state.dart';
import 'package:plux_flutter/src/data/mocks.dart';
import 'package:plux_flutter/src/devtools_api/dev_commands.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart';
import 'package:plux_flutter/src/pxl/types.dart';
import 'package:plux_flutter/src/render/scope.dart';
import 'package:plux_flutter/src/state/providers.dart';
import 'package:plux_flutter/src/state/scope_state.dart';

import '../support/harness.dart';
import '../support/locale_bundle.dart';

const _ok = 'DEV_COMMAND_STATUS_OK';
const _failed = 'DEV_COMMAND_STATUS_FAILED';
const _unsupported = 'DEV_COMMAND_STATUS_UNSUPPORTED';
const _refused = 'DEV_COMMAND_STATUS_REFUSED';
const _count = '01c0c450-6c00-7000-8000-000000000051';

final class _Lifecycle with WidgetsBindingObserver {
  final List<Object> seen = [];

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) => seen.add(state);

  @override
  void didHaveMemoryPressure() => seen.add('memory');
}

final class _Recorder implements DevCommandHandler {
  final List<Map<String, Object?>> commands = [];

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    commands.add(command);
    return {'status': _ok, 'result': base64.encode(utf8.encode('42'))};
  }
}

String _json(Object? v) => base64.encode(utf8.encode(jsonEncode(v)));

/// The dev session commands on the device (`DEV-011`, `DEV-031`) over the
/// routing, state and widgets conformance projects.
void main() {
  final g = Harness.goldens;
  late Harness h;
  late GlobalKey<NavigatorState> navigator;
  late RuntimeDevCommands commands;

  setUp(() async {
    h = await Harness.create();
    navigator = GlobalKey();
  });
  tearDown(() => h.close());

  Future<void> start(
    WidgetTester tester, {
    String project = 'routing',
    Widget home = const SizedBox(),
    Map<String, DevCommandHandler> extensions = const {},
    WidgetsBindingObserver? lifecycle,
    DevScopeResolver? scopes,
  }) async {
    tester.view
      ..physicalSize = const Size(800, 1400)
      ..devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(
      () => switch (project) {
        'state' => h.startFrom(g.bundles['state/state.pxb']!, {
          'notes': g.bundles['state/notes.pxb']!,
        }, navigatorKey: navigator),
        'widgets' => h.startFrom(
          withLocales(g.bundles['widgets/widgets.pxb']!, {
            'de': {_count: '{count, plural, one {# Widget} other {# Widgets}}'},
          }),
          {'gallery': g.bundles['widgets/gallery.pxb']!},
          navigatorKey: navigator,
        ),
        _ => h.startFrom(g.bundles['routing/routing.pxb']!, {
          'nav': g.bundles['routing/nav.pxb']!,
        }, navigatorKey: navigator),
      },
    );
    commands = RuntimeDevCommands(
      runtime: h.runtime,
      container: Plux.container,
      extensions: extensions,
      lifecycle: lifecycle,
      scopes: scopes,
    );
    await tester.pumpWidget(
      MaterialApp(
        navigatorKey: navigator,
        home: PluxScope(child: home),
      ),
    );
    await settle(tester);
  }

  Future<Map<String, Object?>> run(
    WidgetTester tester,
    Map<String, Object?> command,
  ) async {
    final result = await tester.runAsync(() => commands.handle(command));
    await settle(tester);
    return result!;
  }

  String code(Map<String, Object?> result) =>
      (result['error']! as Map<String, Object?>)['code']! as String;

  testWidgets(
    'openScreen opens the route with its parameters, theme and text scale [DEV-011] [A11Y-005]',
    (tester) async {
      await start(tester);
      final result = await run(tester, {
        'openScreen': {
          'route': 'detail',
          'params': {'itemId': '7'},
          'theme': 'DEV_THEME_MODE_DARK',
          'textScale': 1.3,
        },
      });
      expect(result, {'status': _ok});
      expect(find.text('Item 7'), findsOneWidget);
      expect(
        Plux.container.read(environmentProvider).themeMode,
        ThemeMode.dark,
      );
      final scaled = MediaQuery.textScalerOf(
        tester.element(find.text('Item 7')),
      );
      expect(scaled.scale(10), closeTo(13, 1e-9));
      expect(h.runtime.devPreview.value.textScale, 1.3);
    },
  );

  testWidgets(
    'an openScreen without a route is a failed command, not an exception [DEV-011]',
    (tester) async {
      await start(tester);
      final missing = await run(tester, {'openScreen': <String, Object?>{}});
      expect(missing['status'], _failed);
    },
  );

  testWidgets(
    'openScreen switches to a locale and to a pseudo-locale, and refuses one the release lacks [I18N-007] [I18N-010]',
    (tester) async {
      await start(tester, project: 'widgets');
      final de = await run(tester, {
        'openScreen': {'route': 'text', 'locale': 'de'},
      });
      expect(de['status'], _ok);
      await tester.runAsync(
        () => Plux.container.read(localeControllerProvider.notifier).settled(),
      );
      expect(
        Plux.container.read(environmentProvider).locale,
        const Locale('de'),
      );
      final pseudo = await run(tester, {
        'openScreen': {'route': 'text', 'locale': 'en-XA'},
      });
      expect(pseudo['status'], _ok);
      expect(
        Plux.container.read(environmentProvider).locale?.toLanguageTag(),
        'en-XA',
      );
      final bad = await run(tester, {
        'openScreen': {'route': 'text', 'locale': 'not a tag'},
      });
      expect(bad['status'], _failed);
    },
  );

  testWidgets('openScreen selects the mocks it names [DAT-080] [DEV-011]', (
    tester,
  ) async {
    await start(tester);
    final result = await run(tester, {
      'openScreen': {
        'route': 'home',
        'mocks': [
          {'source': 'tasks', 'state': 'loading'},
          {'source': 'shop/orders', 'state': 'error'},
        ],
      },
    });
    expect(result['status'], _ok);
    expect(h.runtime.dataMocks.selected, {
      'tasks': DataMockState.loading,
      'shop/orders': DataMockState.error,
    });
    final bad = await run(tester, {
      'openScreen': {
        'route': 'home',
        'mocks': [
          {'source': 'tasks', 'state': 'nonsense'},
        ],
      },
    });
    expect(bad['status'], _failed);
  });

  testWidgets(
    'switchMock selects and clears a source\'s mock state [DAT-080]',
    (tester) async {
      await start(tester);
      expect(
        await run(tester, {
          'switchMock': {'source': 'tasks', 'state': 'empty'},
        }),
        {'status': _ok},
      );
      expect(h.runtime.dataMocks.of('', 'tasks'), DataMockState.empty);
      await run(tester, {
        'switchMock': {'source': 'tasks', 'state': 'success'},
      });
      expect(h.runtime.dataMocks.of('', 'tasks'), DataMockState.success);
      await run(tester, {
        'switchMock': {'source': 'tasks'},
      });
      expect(h.runtime.dataMocks.of('', 'tasks'), isNull);
      final bad = await run(tester, {
        'switchMock': {'source': 'tasks', 'state': 'bogus'},
      });
      expect(bad['status'], _failed);
    },
  );

  testWidgets(
    'setState writes a typed value into the app scope and refuses what cannot be written [DEV-031]',
    (tester) async {
      await start(tester, project: 'state');
      Map<String, Object?> app() => Plux.container.read(appStateProvider);
      expect(
        await run(tester, {
          'setState': {'scope': 'app', 'path': 'counter', 'value': _json(5)},
        }),
        {'status': _ok},
      );
      expect(app()['counter'], 5);
      final wrongType = await run(tester, {
        'setState': {'scope': 'app', 'path': 'counter', 'value': _json('x')},
      });
      expect(wrongType['status'], _failed);
      expect(code(wrongType), PluxErrorCode.stateWriteTypeMismatch.id);
      final computed = await run(tester, {
        'setState': {'scope': 'app', 'path': 'doubled', 'value': _json(1)},
      });
      expect(computed['status'], _failed);
      final unknown = await run(tester, {
        'setState': {'scope': 'app', 'path': 'nothing', 'value': _json(1)},
      });
      expect(unknown['status'], _failed);
      final noScope = await run(tester, {
        'setState': {'scope': 'page:/x', 'path': 'a', 'value': _json(1)},
      });
      expect(noScope['status'], _failed);
      final notJson = await run(tester, {
        'setState': {'scope': 'app', 'path': 'counter', 'value': '!!'},
      });
      expect(notJson['status'], _failed);
      expect(app()['counter'], 5);
    },
  );

  testWidgets(
    'setState refuses a sensitive entry and writes into a resolved page scope [DEV-031]',
    (tester) async {
      final page = PageInstance(
        const {},
        model: ScopeModel(
          kind: StateScopeKind.page,
          owner: 'p',
          decls: const [
            StateDecl(
              name: 'secret',
              id: 's',
              typeExpr: 'string',
              type: PxlType(PxlKind.string),
              defaultValue: '',
              sensitive: true,
            ),
            StateDecl(
              name: 'count',
              id: 'c',
              typeExpr: 'int',
              type: PxlType(PxlKind.int),
              defaultValue: 0,
            ),
          ],
          evaluate: (_, _) => null,
          types: const {},
        ),
      );
      await start(
        tester,
        scopes: (scope) => scope == 'page:/x'
            ? (
                values: Plux.container.read(pageStateProvider(page).notifier),
                state: Plux.container.read(pageStateProvider(page)),
              )
            : null,
      );
      Plux.container.listen(pageStateProvider(page), (_, _) {});
      final refused = await run(tester, {
        'setState': {
          'scope': 'page:/x',
          'path': 'secret',
          'value': _json('hunter2'),
        },
      });
      expect(refused['status'], _refused);
      expect(code(refused), PluxErrorCode.devCommandNotAllowed.id);
      expect(jsonEncode(refused), isNot(contains('hunter2')));
      expect(
        await run(tester, {
          'setState': {'scope': 'page:/x', 'path': 'count', 'value': _json(3)},
        }),
        {'status': _ok},
      );
      expect(Plux.container.read(pageStateProvider(page))['count'], 3);
    },
  );

  testWidgets(
    'fakeClock sets, advances and resets the clock of the actions [DEV-031]',
    (tester) async {
      await start(tester);
      final clock = h.runtime.actionClock;
      expect(
        await run(tester, {
          'fakeClock': {'setUnixMicros': '1000000'},
        }),
        {'status': _ok},
      );
      expect(clock.now, const Duration(seconds: 1));
      final fired = <String>[];
      final slept = clock
          .sleep(const Duration(seconds: 10))
          .then((_) => fired.add('sleep'));
      clock.timer(const Duration(seconds: 3), () => fired.add('timer'));
      final ticks = clock.periodic(
        const Duration(seconds: 4),
        () => fired.add('tick'),
      );
      expect(
        await run(tester, {
          'fakeClock': {'advanceMicros': 5000000},
        }),
        {'status': _ok},
      );
      expect(clock.now, const Duration(seconds: 6));
      expect(fired, ['timer', 'tick']);
      await run(tester, {
        'fakeClock': {'advanceMicros': 5000000},
      });
      await slept;
      expect(fired, ['timer', 'tick', 'tick', 'sleep']);
      ticks.cancel();
      expect(
        await run(tester, {
          'fakeClock': {'reset': true},
        }),
        {'status': _ok},
      );
      expect(h.runtime.devClock.isFaked, isFalse);
      expect(clock.now, isNot(const Duration(seconds: 11)));
      final unfixed = await run(tester, {
        'fakeClock': {'advanceMicros': 1},
      });
      expect(unfixed['status'], _failed);
      final both = await run(tester, {
        'fakeClock': {'reset': true, 'setUnixMicros': 1},
      });
      expect(both['status'], _failed);
      final text = await run(tester, {
        'fakeClock': {'setUnixMicros': 'abc'},
      });
      expect(text['status'], _failed);
    },
  );

  testWidgets('lifecycle commands reach the observer [DEV-031]', (
    tester,
  ) async {
    final observer = _Lifecycle();
    await start(tester, lifecycle: observer);
    for (final event in [
      'PAUSE',
      'RESUME',
      'INACTIVE',
      'DETACHED',
      'MEMORY_PRESSURE',
    ]) {
      expect(
        await run(tester, {
          'lifecycle': {'event': 'DEV_LIFECYCLE_EVENT_$event'},
        }),
        {'status': _ok},
      );
    }
    expect(observer.seen, [
      AppLifecycleState.paused,
      AppLifecycleState.resumed,
      AppLifecycleState.inactive,
      AppLifecycleState.detached,
      'memory',
    ]);
    final unspecified = await run(tester, {
      'lifecycle': {'event': 'DEV_LIFECYCLE_EVENT_UNSPECIFIED'},
    });
    expect(unspecified['status'], _failed);
    expect(observer.seen, hasLength(5));
  });

  testWidgets('lifecycle commands drive the runtime by default [DEV-031]', (
    tester,
  ) async {
    await start(tester);
    expect(
      await run(tester, {
        'lifecycle': {'event': 2},
      }),
      {'status': _ok},
    );
    expect(
      await run(tester, {
        'lifecycle': {'event': 1},
      }),
      {'status': _ok},
    );
    expect(
      await run(tester, {
        'lifecycle': {'event': 5},
      }),
      {'status': _ok},
    );
  });

  testWidgets(
    'commands of later slices go to their registered handler, the others are unsupported [DEV-031]',
    (tester) async {
      final recorder = _Recorder();
      await start(tester, extensions: {'networkProfile': recorder});
      final command = {
        'networkProfile': {'profileId': 'offline'},
      };
      final result = await run(tester, command);
      expect(result['status'], _ok);
      expect(utf8.decode(base64.decode(result['result']! as String)), '42');
      expect(recorder.commands, [command]);
      for (final name in [
        'simulateSecurityDetection',
        'evaluatePxl',
        'captureScreenshot',
        'inspectSelect',
        'toggleOverlay',
      ]) {
        final r = await run(tester, {name: <String, Object?>{}});
        expect(r['status'], _unsupported, reason: name);
        expect(code(r), 'PLX-9232', reason: name);
      }
    },
  );

  testWidgets(
    'a malformed or unknown command fails and never throws [DEV-011]',
    (tester) async {
      await start(tester);
      for (final bad in <Map<String, Object?>>[
        {},
        {'openScreen': 'home'},
        {'bogus': <String, Object?>{}},
        {
          'openScreen': <String, Object?>{'route': 'home'},
          'setState': <String, Object?>{},
        },
        {
          'openScreen': {
            'route': 'home',
            'params': {'a': 1},
          },
        },
        {
          'openScreen': {'route': 'home', 'textScale': 'wide'},
        },
        {
          'openScreen': {'route': 'home', 'theme': 'DEV_THEME_MODE_PINK'},
        },
        {
          'openScreen': {'route': 'home', 'mocks': 'all'},
        },
        {'switchMock': <String, Object?>{}},
        {'lifecycle': <String, Object?>{}},
      ]) {
        final r = await run(tester, bad);
        expect(r['status'], _failed, reason: '$bad');
        expect(
          (r['error']! as Map<String, Object?>)['message'],
          isNotEmpty,
          reason: '$bad',
        );
      }
    },
  );
}
