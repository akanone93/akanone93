// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Overlays keep the page scope (X1.7).
library;

import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:plux_flutter/src/render/scope.dart';

/// [child] carrying the render scope and the providers of [from], for
/// content that Flutter shows in an overlay or a route above the page
/// (menus, search views, option lists, drag feedback, context-menu
/// previews), where the page's inherited widgets are not in reach.
Widget keepScope(BuildContext from, Widget child) {
  final scope = RenderScope.of(from);
  return UncontrolledProviderScope(
    container: ProviderScope.containerOf(from, listen: false),
    child: RenderScopeWidget(scope: scope, child: child),
  );
}
