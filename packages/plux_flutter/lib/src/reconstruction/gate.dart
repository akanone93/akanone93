// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a reconstruction may run (SEC-142, ADR-0082): in reconstruction
/// mode the runtime shows the page as a user saw it and does nothing that
/// reaches beyond the app's own screens. Every action is either admitted
/// here, because it only changes what the reconstruction shows, or refused
/// before its handler is reached.
library;

import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';

/// Admits the actions of a reconstruction (SEC-142).
///
/// The lists are explicit and complete: a test checks that every action of
/// the registry is on exactly one of them, so that an action a later phase
/// adds is decided on purpose. An action on neither list is refused.
final class ReconstructionGate {
  /// Creates the gate.
  const ReconstructionGate();

  /// The actions that run: control flow, the state of the page, the app's
  /// own navigation and presentation, forms without a submission, local
  /// animations and messages shown on screen. None of them leaves the app.
  static const Set<String> admitted = {
    'announce',
    'callFlow',
    'condition',
    'controlAnimation',
    'delay',
    'emitEvent',
    'forEach',
    'navigate',
    'openBottomSheet',
    'openDialog',
    'parallel',
    'patchState',
    'pop',
    'resetForm',
    'resetState',
    'setState',
    'showSnackbar',
    'showToast',
    'startAnimation',
    'stop',
    'switch',
    'switchTab',
    'validateForm',
  };

  /// The actions refused: they write or read data sources, call functions,
  /// reach the host or the device, open anything outside the app, or change
  /// the presentation the reconstruction applies.
  static const Set<String> refused = {
    'apiCall',
    'biometricAuth',
    'callNative',
    'capturePhoto',
    'copyToClipboard',
    'dbDelete',
    'dbInsert',
    'dbQuery',
    'dbUpdate',
    'dbUpsert',
    'emitHostEvent',
    'getLocation',
    'haptic',
    'invokeFunction',
    'kvGet',
    'kvRemove',
    'kvSet',
    'logout',
    'openUrl',
    'pickFile',
    'pickImage',
    'readConfig',
    'refreshData',
    'requestPermission',
    'scanCode',
    'setLocale',
    'setThemeMode',
    'share',
    'signTransaction',
    'startPayment',
    'submitForm',
    'subscribe',
    'sync',
    'trackEvent',
    'unsubscribe',
  };

  /// Admits a step running [action], or fails it with `PLX-4010`, which its
  /// `onError` can handle: the runtime does not run the action in
  /// reconstruction mode.
  void admit(String action) {
    if (admitted.contains(action)) return;
    throw ActionError(
      ActionErrorKind.custom,
      PluxErrorCode.actionsNotAvailable,
      'action $action is not run in reconstruction mode (SEC-142)',
    );
  }
}
