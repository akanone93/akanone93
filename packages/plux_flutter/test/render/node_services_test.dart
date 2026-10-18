// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_components/plux_components.dart';
import 'package:plux_flutter/plux_flutter.dart';

import '../support/harness.dart';

/// What a widget package asks of the node it builds (RT-060): validating the
/// fields of the form around it, picking and uploading a file, and
/// reporting a missing device package. The builders of `plux_components`
/// that use them are tested in that package; these probes stand in for them
/// on the gallery pages.
void main() {
  final g = Harness.goldens;
  late Harness h;
  late List<String> log;
  setUp(() async {
    h = await Harness.create();
    log = [];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (_) async => null);
  });
  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null);
    return h.close();
  });

  Future<void> open(
    WidgetTester tester,
    String route,
    NodeBuilder probe,
    int id,
  ) async {
    tester.view.physicalSize = const Size(800, 3400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      await h.startFrom(
        g.bundles['widgets/widgets.pxb']!,
        {'gallery': g.bundles['widgets/gallery.pxb']!},
        widgetPackages: [
          const PluxComponents(),
          _Probe({id: probe}),
        ],
      );
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        home: PluxScope(child: PluxView(route)),
      ),
    );
    await settle(tester);
  }

  testWidgets('a node validates the fields it names in the form around it '
      '[RT-060]', (tester) async {
    await open(tester, 'components-a', (c) {
      return TextButton(
        onPressed: () async {
          log.add('a: ${await c.validateFields({'a'})}');
          log.add('none: ${await c.validateFields(const {})}');
        },
        child: const Text('probe'),
      );
    }, PluxComponentIds.wizardForm);
    await tester.tap(find.text('probe'));
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 50)),
    );
    await settle(tester);
    expect(log, ['a: false', 'none: true']);
  });

  testWidgets('a node outside a form validates nothing [RT-060]', (
    tester,
  ) async {
    await open(tester, 'components-b', (c) {
      return TextButton(
        onPressed: () async => log.add('${await c.validateFields({'a'})}'),
        child: const Text('probe'),
      );
    }, PluxComponentIds.fileUploader);
    await tester.tap(find.text('probe'));
    await settle(tester);
    expect(log, ['true']);
  });

  testWidgets('a node gets no file picker without a media package, uploads '
      'through a data source and reports the missing package [RT-060]', (
    tester,
  ) async {
    await open(tester, 'components-b', (c) {
      return TextButton(
        onPressed: () async {
          log.add('picker: ${c.filePicker(const ['image/png'])}');
          final upload = c.fileUpload('nowhere', 'put');
          log.add('upload: ${upload != null}');
          c.reportDevicePackageMissing('the probe needs plux_media');
          if (upload != null) {
            try {
              await upload(
                const PluxPickedFile(
                  path: '/tmp/x',
                  name: 'x',
                  mimeType: 'text/plain',
                  sizeBytes: 1,
                ),
                (_) {},
              );
            } on PluxException catch (e) {
              log.add(
                'failed: ${e.code == PluxErrorCode.dataSourceUnavailable}',
              );
            }
          }
        },
        child: const Text('probe'),
      );
    }, PluxComponentIds.fileUploader);
    await tester.tap(find.text('probe'));
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 50)),
    );
    await settle(tester);
    expect(log.take(2), ['picker: null', 'upload: true']);
    expect(
      h.errors.where((e) => e.code == PluxErrorCode.devicePackageMissing),
      isNotEmpty,
    );
  });
}

/// A widget package of one builder, for a test.
final class _Probe implements PluxWidgetPackage {
  const _Probe(this.builders);

  @override
  String get name => 'probe';

  @override
  final Map<int, NodeBuilder> builders;
}
