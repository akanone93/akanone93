// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Hands the links the platform delivers to a [ViewerController] (SEC-142,
/// ADR-0082).
library;

import 'package:flutter/widgets.dart';
import 'package:plux_host_kit/src/controller.dart';
import 'package:plux_host_kit/src/link.dart';

/// Hands the links the platform delivers to a [ViewerController].
///
/// A custom-scheme link reaches Flutter as route information, once at launch
/// (the engine's default route) and again whenever the app is open. The
/// viewer claims every link of its scheme, valid or not, so that the
/// framework's own navigator never tries to push it as a route.
final class ViewerLinks with WidgetsBindingObserver {
  /// Creates the receiver for [controller].
  ViewerLinks(this.controller);

  /// Receives the links.
  final ViewerController controller;

  /// Starts listening, and offers the link the app was launched with, if
  /// any. [initialRoute] is the engine's default route.
  void attach(String initialRoute) {
    WidgetsBinding.instance.addObserver(this);
    final uri = Uri.tryParse(initialRoute);
    if (uri != null) offer(uri);
  }

  /// Stops listening.
  void detach() => WidgetsBinding.instance.removeObserver(this);

  /// Offers [uri]; whether it was a viewer link, and so is handled.
  bool offer(Uri uri) {
    if (!ViewerLink.isViewerUri(uri)) return false;
    try {
      controller.offerLink(ViewerLink.parse(uri));
    } on FormatException catch (e) {
      controller.rejectLink('That link cannot be opened: ${e.message}.');
    }
    return true;
  }

  @override
  Future<bool> didPushRouteInformation(RouteInformation routeInformation) =>
      Future.value(offer(routeInformation.uri));
}
