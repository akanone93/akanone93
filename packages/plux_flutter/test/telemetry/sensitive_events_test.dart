// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';

void main() {
  // Verifies: SCH-012, SEC-092.
  test('the registry marks the personal event payloads sensitive', () {
    WidgetDescriptor widget(String type) =>
        widgetDescriptors.firstWhere((w) => w.type == type);
    expect(widget('AmountInput').sensitiveEvents, {'onChanged'});
    expect(widget('OtpInput').sensitiveEvents, {'onCompleted'});
    expect(widget('QrScanner').sensitiveEvents, {'onDetected'});
    expect(widget('Switch').sensitiveEvents, isEmpty);
    for (final w in widgetDescriptors) {
      expect(w.events.keys, containsAll(w.sensitiveEvents));
    }
  });
}
