// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_viewer/viewer.dart';

/// The production root keys the viewer is built with [SEC-142] [SEC-051].
void main() {
  test('holds no key when the build was given none [SEC-142]', () {
    expect(builtInRootKeys(), isEmpty);
  });

  test('names its own build setting when it opens nothing [SEC-142]', () {
    expect(rootKeysDefine, 'PLUX_VIEWER_ROOT_KEYS');
    expect(viewerNoRootKeysMessage, contains(rootKeysDefine));
  });
}
