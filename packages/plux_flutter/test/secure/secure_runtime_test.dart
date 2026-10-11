// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/engine.dart';
import 'package:plux_flutter/src/actions/graph.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/actions/run.dart';
import 'package:plux_flutter/src/actions/trace.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/core/features.dart';
import 'package:plux_flutter/src/device/guard.dart';
import 'package:plux_flutter/src/device/services.dart';
import 'package:plux_flutter/src/forms/form_state.dart';
import 'package:plux_flutter/src/pxl/regex.dart';
import 'package:plux_flutter/src/render/builders/secure.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/node_context.dart';
import 'package:plux_flutter/src/render/renderer.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';
import 'package:plux_flutter/src/secure/masking.dart';
import 'package:plux_flutter/src/secure/platform.dart';
import 'package:plux_flutter/src/secure/secure_ui.dart';

import '../actions/engine_test.dart' show FakeNavigator, id, input, lit;
import '../support/harness.dart';
import 'secure_pages_test.dart' show FakeBiometrics, FakeClock, FakeScreen;

WidgetDescriptor _widget(String type) =>
    widgetDescriptors.firstWhere((d) => d.type == type);

fbs.ValueObjectBuilder _str(int s) =>
    fbs.ValueObjectBuilder(kind: fbs.ValueKind.String, s: s);

fbs.ValueObjectBuilder _int(int i) =>
    fbs.ValueObjectBuilder(kind: fbs.ValueKind.Int, i: i);

/// The gallery plugin bundle with its `layout` page replaced by a page of
/// [nodes] and [strings], marked [secure] or not.
Uint8List _gallery(
  List<fbs.NodeObjectBuilder> nodes,
  List<String> strings, {
  required bool secure,
}) {
  final c = BundleContainer.parse(
    Harness.goldens.bundles['widgets/gallery.pxb']!,
  );
  final sections = <Section>[];
  for (final s in c.sections) {
    if (s.kind == SectionKind.page) {
      final page = fbs.Page(s.data);
      if (page.strings![page.route] == 'layout') {
        final data = fbs.PageObjectBuilder(
          id: fbs.UuidObjectBuilder(hi: page.id!.hi, lo: page.id!.lo),
          key: 1,
          route: 1,
          nodes: nodes,
          strings: strings,
          secure: secure,
        ).toBytes('PXPG');
        sections.add(Section(s.kind, s.id, s.hash, data));
        continue;
      }
    }
    sections.add(s);
  }
  return encodeBundle(BundleKinds.plugin, sections);
}

/// A node as a builder reads it, with props by permanent ID.
final class _Node implements NodeContext {
  _Node(this.context, this.props, {this.handled = const {}});

  @override
  final BuildContext context;
  final Map<int, Object?> props;
  final Set<int> handled;
  final List<int> fired = [];

  @override
  T? decode<T>(int id, T? Function(Decoding d, Object? v) decoder) {
    final v = props[id];
    return v == null ? null : decoder(this, v);
  }

  @override
  bool handles(int id) => handled.contains(id);

  @override
  void fire(int id, [Object? payload]) => fired.add(id);

  @override
  Never missing(String name) => throw DecodeError('$name is missing');

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('the secure widgets in a page', () {
    late Harness h;
    late FakeScreen screen;
    late FakeBiometrics bio;

    setUp(() async {
      h = await Harness.create();
      screen = FakeScreen();
      bio = FakeBiometrics();
    });
    tearDown(() => h.close());

    final text = _widget('SecureTextField');
    final pin = _widget('SecurePinPad');
    final otp = _widget('OtpInput');
    final button = _widget('BiometricButton');
    final column = _widget('Column');

    Future<void> show(WidgetTester tester, {required bool secure}) async {
      tester.view
        ..physicalSize = const Size(800, 2400)
        ..devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final gallery = _gallery(
        [
          fbs.NodeObjectBuilder(widget: column.id, children: [1, 2, 3, 4]),
          fbs.NodeObjectBuilder(
            widget: text.id,
            props: [
              fbs.PropObjectBuilder(id: text.props['label'], value: _str(2)),
            ],
          ),
          fbs.NodeObjectBuilder(
            widget: pin.id,
            props: [
              fbs.PropObjectBuilder(id: pin.props['length'], value: _int(6)),
            ],
          ),
          fbs.NodeObjectBuilder(widget: otp.id),
          fbs.NodeObjectBuilder(
            widget: button.id,
            props: [
              fbs.PropObjectBuilder(id: button.props['label'], value: _str(3)),
              fbs.PropObjectBuilder(id: button.props['reason'], value: _str(4)),
            ],
          ),
        ],
        ['', 'layout', 'Password', 'Confirm', 'Approve the transfer'],
        secure: secure,
      );
      await tester.runAsync(
        () => h.startFrom(Harness.goldens.bundles['widgets/widgets.pxb']!, {
          'gallery': gallery,
        }, secure: SecurePlatforms(screen: screen, biometrics: bio)),
      );
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(body: PluxScope(child: PluxView('layout'))),
        ),
      );
      await settle(tester);
    }

    testWidgets(
      'the registry builds the four widgets, and the runtime accepts their features [SEC-090]',
      (tester) async {
        await show(tester, secure: false);
        expect(find.byType(TextField), findsNWidgets(2));
        expect(find.text('Password'), findsOneWidget);
        expect(find.text('Confirm'), findsOneWidget);
        expect(find.text('One-time code'), findsOneWidget);
        // The pad has six dots for a six-digit PIN.
        expect(find.byIcon(Icons.fingerprint), findsOneWidget);
        expect(
          nodeBuilders.keys,
          containsAll([text.id, pin.id, otp.id, button.id]),
        );
        final features = RuntimeFeatures();
        for (final f in [
          'widget.SecureTextField.v1',
          'widget.SecurePinPad.v1',
          'widget.OtpInput.v1',
          'widget.BiometricButton.v1',
          'forms.sensitive.v1',
          'security.secureui.v1',
        ]) {
          expect(features.supports(f), isTrue, reason: f);
        }
        expect(screen.calls, isEmpty, reason: 'the page is not secure');
        expect(
          h.errors.where((e) => e.code != PluxErrorCode.syncFailed),
          isEmpty,
        );
      },
    );

    testWidgets(
      'a secure page protects the screen while it shows and not after [SEC-091] [SEC-093]',
      (tester) async {
        await show(tester, secure: true);
        expect(screen.calls, [true]);
        await tester.pumpWidget(const MaterialApp(home: SizedBox()));
        await settle(tester);
        expect(screen.calls, [true, false]);
      },
    );

    testWidgets(
      'a secure page locks after the idle time and unlocks with the biometric [SEC-094]',
      (tester) async {
        await show(tester, secure: true);
        h.runtime.settings.value = SecuritySettings.fromDocument(const {
          'profile': 'maximum',
        });
        await tester.pump();
        await tester.pump(const Duration(seconds: 119));
        expect(find.text('Locked'), findsNothing);
        await tester.pump(const Duration(seconds: 2));
        expect(find.text('Locked'), findsOneWidget);
        expect(find.text('Password'), findsNothing);
        bio.outcome = BiometricOutcome.success;
        await tester.tap(find.text('Unlock'));
        await tester.pump();
        await tester.pump();
        expect(find.text('Password'), findsOneWidget);
        expect(bio.credential, everyElement(isTrue));
      },
    );

    testWidgets(
      'BiometricButton is held to the plugin\'s biometrics capability [SEC-080]',
      (tester) async {
        await show(tester, secure: false);
        await tester.tap(find.text('Confirm'));
        await tester.pump();
        expect(bio.asked, isEmpty);
        expect(
          h.errors.map((e) => e.code),
          contains(PluxErrorCode.deviceCapabilityBlocked),
        );
      },
    );
  });

  group('masked values', () {
    late Harness h;
    setUp(() async => h = await Harness.create());
    tearDown(() => h.close());

    final text = _widget('Text');
    final field = _widget('TextField');
    final column = _widget('Column');

    /// A page with a masked and a plain Text, and a masked input; the
    /// compiler's `Masked` hint is set by hand.
    Future<void> show(WidgetTester tester) async {
      tester.view
        ..physicalSize = const Size(800, 2400)
        ..devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final gallery = _gallery(
        [
          fbs.NodeObjectBuilder(widget: column.id, children: [1, 2, 3]),
          fbs.NodeObjectBuilder(
            widget: text.id,
            hints: 4,
            props: [
              fbs.PropObjectBuilder(id: text.props['data'], value: _str(2)),
            ],
          ),
          fbs.NodeObjectBuilder(
            widget: text.id,
            props: [
              fbs.PropObjectBuilder(id: text.props['data'], value: _str(3)),
            ],
          ),
          fbs.NodeObjectBuilder(
            widget: field.id,
            hints: 4,
            props: [
              fbs.PropObjectBuilder(id: field.props['value'], value: _str(4)),
            ],
          ),
        ],
        ['', 'layout', 'topsecret', 'public', 'pin-4321'],
        secure: false,
      );
      await tester.runAsync(
        () => h.startFrom(Harness.goldens.bundles['widgets/widgets.pxb']!, {
          'gallery': gallery,
        }),
      );
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(body: PluxScope(child: PluxView('layout'))),
        ),
      );
      await settle(tester);
    }

    TextField input(WidgetTester tester) =>
        tester.widget<TextField>(find.byType(TextField));

    testWidgets(
      'a value the compiler marked is bullets until the user unmasks it, and never reaches the semantics tree [SEC-092]',
      (tester) async {
        final handle = tester.ensureSemantics();
        await show(tester);
        expect(find.text('topsecret'), findsNothing);
        expect(find.text(maskBullets), findsOneWidget);
        expect(find.bySemanticsLabel('Hidden value'), findsOneWidget);
        expect(find.bySemanticsLabel(RegExp('topsecret')), findsNothing);
        expect(
          find.text('public'),
          findsOneWidget,
          reason: 'plain text is not masked',
        );

        await tester.tap(find.byTooltip('Show value').first);
        await tester.pump();
        expect(find.text('topsecret'), findsOneWidget);
        expect(find.text(maskBullets), findsNothing);

        await tester.tap(find.byTooltip('Hide value'));
        await tester.pump();
        expect(find.text('topsecret'), findsNothing);
        handle.dispose();
      },
    );

    testWidgets(
      'a masked input is obscured, takes no suggestions, learning or menu, and unmasks on request [SEC-092]',
      (tester) async {
        final handle = tester.ensureSemantics();
        await show(tester);
        var f = input(tester);
        expect(f.obscureText, isTrue);
        expect(f.enableSuggestions, isFalse);
        expect(f.autocorrect, isFalse);
        expect(f.enableIMEPersonalizedLearning, isFalse);
        expect(f.enableInteractiveSelection, isFalse);
        expect(find.bySemanticsLabel(RegExp('pin-4321')), findsNothing);

        await tester.tap(find.byTooltip('Show value').last);
        await tester.pump();
        f = input(tester);
        expect(f.obscureText, isFalse);
        expect(f.controller!.text, 'pin-4321');

        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.inactive,
        );
        await tester.pump();
        expect(
          input(tester).obscureText,
          isTrue,
          reason: 'masked again in the background',
        );
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.resumed,
        );
        handle.dispose();
      },
    );

    testWidgets(
      'a masked node reports no error and keeps the page working [SEC-092]',
      (tester) async {
        await show(tester);
        expect(
          h.errors.where((e) => e.code != PluxErrorCode.syncFailed),
          isEmpty,
        );
      },
    );
  });

  group('BiometricButton', () {
    Future<(_Node, FakeBiometrics, List<PluxException>)> build(
      WidgetTester tester, {
      required List<String> declared,
      Set<String> handled = const {'onSuccess', 'onFailure'},
    }) async {
      final bio = FakeBiometrics();
      final reports = <PluxException>[];
      final ui = SecureUi(
        settings: ValueNotifier(SecuritySettings.builtIn),
        report: reports.add,
        platforms: SecurePlatforms(screen: FakeScreen(), biometrics: bio),
        clock: FakeClock(),
      );
      addTearDown(ui.dispose);
      final d = _widget('BiometricButton');
      late _Node node;
      await tester.pumpWidget(
        MaterialApp(
          home: SecureUiScope(
            ui: ui,
            plugin: 'bank',
            guard: DeviceGuard(
              capabilities: (_) =>
                  (deviceApis: declared, networkDomains: const []),
              deepLinks: () => null,
              report: reports.add,
            ),
            child: Builder(
              builder: (context) {
                node = _Node(
                  context,
                  {
                    d.props['label']!: 'Approve',
                    d.props['reason']!: 'Approve it',
                  },
                  handled: {for (final e in handled) d.events[e]!},
                );
                return buildBiometricButton(node);
              },
            ),
          ),
        ),
      );
      return (node, bio, reports);
    }

    testWidgets(
      'runs the biometric prompt with the reason and fires onSuccess [SEC-090] [SEC-080]',
      (tester) async {
        final (node, bio, reports) = await build(
          tester,
          declared: ['biometrics'],
        );
        bio.outcome = BiometricOutcome.success;
        await tester.tap(find.text('Approve'));
        await tester.pump();
        expect(bio.asked, ['Approve it']);
        final d = _widget('BiometricButton');
        expect(node.fired, [d.events['onSuccess']]);
        expect(reports, isEmpty);
      },
    );

    testWidgets(
      'fires onFailure when the confirmation fails, and reports a device that cannot ask [SEC-090]',
      (tester) async {
        final (node, bio, reports) = await build(
          tester,
          declared: ['biometrics'],
        );
        final d = _widget('BiometricButton');
        await tester.tap(find.text('Approve'));
        await tester.pump();
        bio.failure = const PluxDeviceException.unavailable('none enrolled');
        await tester.tap(find.text('Approve'));
        await tester.pump();
        expect(node.fired, [d.events['onFailure'], d.events['onFailure']]);
        expect(reports.single.code, PluxErrorCode.deviceUnavailable);
      },
    );

    testWidgets('without the capability the prompt never opens [SEC-080]', (
      tester,
    ) async {
      final (node, bio, reports) = await build(tester, declared: ['haptics']);
      await tester.tap(find.text('Approve'));
      await tester.pump();
      expect(bio.asked, isEmpty);
      expect(node.fired, [_widget('BiometricButton').events['onFailure']]);
      expect(reports.single.code, PluxErrorCode.deviceCapabilityBlocked);
    });
  });

  group('the biometricAuth action', () {
    late FakeBiometrics bio;
    late List<PluxException> reports;
    List<String> declared = ['biometrics'];

    setUp(() {
      bio = FakeBiometrics();
      reports = [];
      declared = ['biometrics'];
    });

    Future<RunResult> run(Map<String, Object?> inputs) async {
      final host = ActionHost(
        context: StepContext(
          navigator: FakeNavigator(),
          emit: (_, _) {},
          nativeActions: const NoNativeActions(),
          services: deviceServices(
            packages: const [],
            openLink: (_) async => true,
            biometrics: bio,
          ),
          device: DeviceScope(
            plugin: 'bank',
            guard: DeviceGuard(
              capabilities: (_) =>
                  (deviceApis: declared, networkDomains: const []),
              deepLinks: () => null,
              report: reports.add,
            ),
          ),
        ),
        limits: const ActionLimits(
          stepsPerRun: 100,
          stepTimeout: Duration(seconds: 5),
          runTimeout: Duration(seconds: 10),
        ),
        report: reports.add,
        record: (_, {fields = const {}, route = '', pluginKey = ''}) {},
        route: 'home',
        pluginKey: 'bank',
      );
      return (await host.start(
        ActionGraph(
          id: 'g',
          steps: [
            GraphStep(
              id: 'auth',
              action: id('biometricAuth'),
              inputs: {
                for (final e in inputs.entries)
                  input('biometricAuth', e.key): lit(e.value),
              },
              branches: const {'success': 1, 'failure': 2},
            ),
            GraphStep(
              id: 'yes',
              action: id('stop'),
              inputs: {input('stop', 'result'): lit('confirmed')},
            ),
            GraphStep(
              id: 'no',
              action: id('stop'),
              inputs: {input('stop', 'result'): lit('refused')},
            ),
          ],
        ),
        roots: () => const {},
        key: 'k',
      ))!;
    }

    test('takes the success branch when the user confirms, with the reason and the credential fallback [SEC-027]', () async {
      bio.outcome = BiometricOutcome.success;
      final r = await run({'reason': 'Approve'});
      expect(r.result, 'confirmed');
      expect(bio.asked, ['Approve']);
      expect(bio.credential, [
        true,
      ], reason: 'the credential is the default fallback');
      await run({'reason': 'Approve', 'allowDeviceCredential': false});
      expect(bio.credential, [true, false]);
    });

    test('takes the failure branch when the confirmation fails or is cancelled [SEC-027]', () async {
      for (final o in [BiometricOutcome.failed, BiometricOutcome.cancelled]) {
        bio.outcome = o;
        expect((await run({'reason': 'Approve'})).result, 'refused');
      }
    });

    test('a device that cannot ask fails the step with PLX-5403, and a bad input is refused [SEC-027]', () async {
      bio.outcome = BiometricOutcome.unavailable;
      var r = await run({'reason': 'Approve'});
      expect(r.outcome, RunOutcome.failed);
      expect(r.error?.code, PluxErrorCode.deviceUnavailable);
      bio.failure = const PluxDeviceException.unavailable('no prompt');
      r = await run({'reason': 'Approve'});
      expect(r.error?.code, PluxErrorCode.deviceUnavailable);
      r = await run({'reason': 'Approve', 'allowDeviceCredential': 'yes'});
      expect(r.error?.kind, ActionErrorKind.validation);
    });

    test('needs the biometrics capability, and the prompt does not open without it [SEC-080]', () async {
      declared = ['haptics'];
      final r = await run({'reason': 'Approve'});
      expect(r.error?.code, PluxErrorCode.deviceCapabilityBlocked);
      expect(bio.asked, isEmpty);
      expect(DeviceGuard.apiOf['biometricAuth'], 'biometrics');
      expect(reports.first.details, containsPair('api', 'biometrics'));
    });
  });

  group('sensitive values', () {
    late Harness h;
    setUp(() async => h = await Harness.create());
    tearDown(() => h.close());

    test(
      'events that carry a secret are marked, and others are not [SEC-092]',
      () {
        final text = _widget('SecureTextField');
        final pin = _widget('SecurePinPad');
        final otp = _widget('OtpInput');
        final button = _widget('BiometricButton');
        expect(isSensitiveEvent(text.id, text.events['onChanged']!), isTrue);
        expect(isSensitiveEvent(text.id, text.events['onSubmitted']!), isTrue);
        expect(isSensitiveEvent(pin.id, pin.events['onCompleted']!), isTrue);
        expect(isSensitiveEvent(otp.id, otp.events['onCompleted']!), isTrue);
        expect(
          isSensitiveEvent(button.id, button.events['onSuccess']!),
          isFalse,
        );
        expect(isSensitiveEvent(_widget('TextField').id, 1), isFalse);
      },
    );

    test('a run started by a sensitive event records no values, even in debug builds [SEC-092] [ACT-031]', () async {
      final traces = TraceBuffer(capacity: 4, values: true);
      final host = ActionHost(
        context: StepContext(
          navigator: FakeNavigator(),
          emit: (_, _) {},
          nativeActions: const NoNativeActions(),
        ),
        limits: const ActionLimits(
          stepsPerRun: 100,
          stepTimeout: Duration(seconds: 5),
          runTimeout: Duration(seconds: 10),
        ),
        report: (_) {},
        record: (_, {fields = const {}, route = '', pluginKey = ''}) {},
        route: 'home',
        pluginKey: 'bank',
        traces: traces,
      );
      final graph = ActionGraph(
        id: 'g',
        steps: [
          GraphStep(
            id: 'check',
            action: id('condition'),
            inputs: {
              input('condition', 'when'): (roots) => roots['event'] == '1234',
            },
          ),
        ],
      );
      await host.start(graph, roots: () => const {}, key: 'a', event: '1234');
      await host.start(
        graph,
        roots: () => const {},
        key: 'b',
        event: '1234',
        sensitiveEvent: true,
      );
      final runs = traces.runs;
      expect(runs[0].steps.single.inputs, {'when': true});
      expect(runs[1].steps.single.inputs, {'when': PluxStepTrace.redacted});
      expect(runs[1].steps.single.output, PluxStepTrace.redacted);
    });

    testWidgets(
      'a form field tagged sensitive is read from the bundle [SCH-012] [SEC-092]',
      (tester) async {
        await tester.runAsync(
          () => h.startFrom(Harness.goldens.bundles['widgets/widgets.pxb']!, {
            'gallery': Harness.goldens.bundles['widgets/gallery.pxb']!,
          }),
        );
        final renderer = h.runtime.renderer! as PluxRenderer;
        final view = renderer.view(h.runtime.active.value!, 'gallery');
        Uint8List form(bool sensitive) => fbs.FormObjectBuilder(
          name: 1,
          fields: [
            fbs.FormFieldObjectBuilder(name: 2, type: 3, sensitive: sensitive),
            fbs.FormFieldObjectBuilder(name: 4, type: 3),
          ],
        ).toBytes();
        final decls = decodeForms(
          [fbs.Form(form(true))],
          bundle: view,
          strings: (i) => ['', 'login', 'pin', 'string?', 'user'][i],
          types: const {},
          literal: (_) => null,
          regex: RegexLimits.trusted,
        );
        final fields = decls.single.fields;
        expect(
          [for (final f in fields) (f.name, f.sensitive)],
          [('pin', true), ('user', false)],
        );
      },
    );
  });
}
