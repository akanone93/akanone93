// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The builders of the secure widgets of P6 (SEC-090, ADR-0059):
/// `SecureTextField`, `SecurePinPad`, `OtpInput` and `BiometricButton`.
/// Their IDs come from the generated registry (BND-011); the widgets
/// themselves are in `secure/widgets.dart`.
library;

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/device/platform.dart' show PickedFiles;
import 'package:plux_flutter/src/device/types.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/node_context.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';
import 'package:plux_flutter/src/secure/kyc.dart';
import 'package:plux_flutter/src/secure/otp.dart';
import 'package:plux_flutter/src/secure/platform.dart';
import 'package:plux_flutter/src/secure/secure_ui.dart';
import 'package:plux_flutter/src/secure/widgets.dart';

WidgetDescriptor _widget(String type) =>
    widgetDescriptors.firstWhere((d) => d.type == type);

final WidgetDescriptor _text = _widget('SecureTextField');
final WidgetDescriptor _pin = _widget('SecurePinPad');
final WidgetDescriptor _otp = _widget('OtpInput');
final WidgetDescriptor _bio = _widget('BiometricButton');
final WidgetDescriptor _kyc = _widget('KycCameraCapture');

/// The secure widgets' builders, by permanent widget ID.
final Map<int, NodeBuilder> secureBuilders = {
  _text.id: buildSecureTextField,
  _pin.id: buildSecurePinPad,
  _otp.id: buildOtpInput,
  _bio.id: buildBiometricButton,
  _kyc.id: buildKycCameraCapture,
};

/// Whether event [event] of widget [widget] carries a secret: a run it
/// starts records no values in its trace (SEC-092, ACT-031).
bool isSensitiveEvent(int widget, int event) {
  if (widget == _text.id) {
    return event == _text.events['onChanged'] ||
        event == _text.events['onSubmitted'];
  }
  if (widget == _pin.id) return event == _pin.events['onCompleted'];
  if (widget == _otp.id) return event == _otp.events['onCompleted'];
  return false;
}

/// Tells the inactivity lock that the user is typing (SEC-094).
VoidCallback? _activity(NodeContext c) =>
    SecureUiScope.maybeOf(c.context)?.ui.lock.activity;

/// `SecureTextField`: obscured, with no suggestions, learning or clipboard.
Widget buildSecureTextField(NodeContext c) {
  final p = _text.props, e = _text.events;
  return c.text(
    c.decode(p['value']!, asString) ?? '',
    (controller) => SecureTextFieldView(
      controller: controller,
      label: c.decode(p['label']!, asString),
      obscure: c.decode(p['obscure']!, asBool) ?? true,
      maxLength: c.decode(p['maxLength']!, asInt),
      onChanged: c.handles(e['onChanged']!)
          ? (v) => c.fire(e['onChanged']!, v)
          : null,
      onSubmitted: c.handles(e['onSubmitted']!)
          ? (v) => c.fire(e['onSubmitted']!, v)
          : null,
      onActivity: _activity(c),
    ),
  );
}

/// `SecurePinPad`: a keypad with an optional shuffled layout.
Widget buildSecurePinPad(NodeContext c) {
  final p = _pin.props, onCompleted = _pin.events['onCompleted']!;
  return SecurePinPadView(
    length: c.decode(p['length']!, asInt) ?? 4,
    randomize: c.decode(p['randomizeLayout']!, asBool) ?? false,
    haptics: c.decode(p['haptics']!, asBool) ?? true,
    onCompleted: (pin) => c.fire(onCompleted, pin),
    onActivity: _activity(c),
  );
}

/// `OtpInput`: a one-time code with autofill, a format check and a
/// completion event.
Widget buildOtpInput(NodeContext c) {
  final p = _otp.props, onCompleted = _otp.events['onCompleted']!;
  final format = OtpFormat(
    length: c.decode(p['length']!, asInt) ?? 6,
    charset: OtpCharset.named(c.decode(p['alphabet']!, nameOtpAlphabet)),
  );
  return c.text(
    '',
    (controller) => OtpInputView(
      controller: controller,
      format: format,
      autofill: c.decode(p['autofill']!, asBool) ?? true,
      onCompleted: c.handles(onCompleted)
          ? (code) => c.fire(onCompleted, code)
          : null,
      onActivity: _activity(c),
      sms: const ChannelSmsConsentPlatform(),
    ),
  );
}

/// `KycCameraCapture`: a framing guide, the camera of `plux_media` and a
/// quality check, for an ID document or a selfie. It needs the `camera`
/// device API (SEC-080) and `plux_media` in `PluxConfig.devicePackages`
/// (RT-060); without either the button is disabled and the problem
/// reported. The photo is kept behind an opaque reference, which
/// `onCaptured` carries.
Widget buildKycCameraCapture(NodeContext c) {
  final p = _kyc.props, onCaptured = _kyc.events['onCaptured']!;
  final mode =
      c.decode(p['mode']!, nameKycCaptureMode) ??
      c.missing('KycCameraCapture.mode');
  final scope = SecureUiScope.maybeOf(c.context);
  final guard = scope?.guard;
  final files = scope?.service<PickedFiles>();
  return KycCameraCaptureView(
    mode: mode,
    capture: scope == null || guard == null || files == null
        ? null
        : () async {
            try {
              guard.admit(scope.plugin, 'capturePhoto', const {});
            } on ActionError {
              // Blocked: the guard has reported it.
              return null;
            }
            final kyc = scope.service<PluxKycCapture>();
            if (kyc == null) {
              guard.report(
                PluxException(
                  PluxErrorCode.devicePackageMissing,
                  'KycCameraCapture needs plux_media: add it to '
                  'PluxConfig.devicePackages',
                  details: {'plugin': scope.plugin},
                ),
              );
              return null;
            }
            try {
              final photo = await kyc.capture(mode);
              if (photo == null) return null;
              return KycShot(
                path: photo.file.path,
                reference: files.add(photo.file)['handle']! as String,
                problems: photo.problems,
              );
            } on PluxDeviceException catch (e) {
              guard.report(
                PluxException(
                  e.failure == PluxDeviceFailure.denied
                      ? PluxErrorCode.devicePermissionDenied
                      : PluxErrorCode.deviceUnavailable,
                  'KycCameraCapture: ${e.message}',
                  details: {'plugin': scope.plugin},
                ),
              );
              return null;
            }
          },
    onCaptured: c.handles(onCaptured) ? (ref) => c.fire(onCaptured, ref) : null,
    onActivity: _activity(c),
  );
}

/// `BiometricButton`: runs the biometric prompt of `biometricAuth`, under
/// the plugin's `biometrics` capability (SEC-080), and fires `onSuccess` or
/// `onFailure`. Without the runtime's device services the button is
/// disabled.
Widget buildBiometricButton(NodeContext c) {
  final p = _bio.props, e = _bio.events;
  final label =
      c.decode(p['label']!, asString) ?? c.missing('BiometricButton.label');
  final reason =
      c.decode(p['reason']!, asString) ?? c.missing('BiometricButton.reason');
  final scope = SecureUiScope.maybeOf(c.context);
  final guard = scope?.guard;
  return BiometricButtonView(
    label: label,
    ask: scope == null || guard == null
        ? null
        : () async {
            try {
              guard.admit(scope.plugin, 'biometricAuth', const {});
              return await scope.ui.biometrics.authenticate(
                reason: reason,
                allowDeviceCredential: true,
              );
            } on ActionError {
              // Blocked: the guard has reported it.
              return BiometricOutcome.failed;
            } on Exception catch (error) {
              guard.report(
                PluxException(
                  PluxErrorCode.deviceUnavailable,
                  'BiometricButton: the biometric prompt failed',
                  details: {
                    'plugin': scope.plugin,
                    'error': '${error.runtimeType}',
                  },
                ),
              );
              return BiometricOutcome.unavailable;
            }
          },
    onSuccess: c.handles(e['onSuccess']!)
        ? () => c.fire(e['onSuccess']!)
        : null,
    onFailure: c.handles(e['onFailure']!)
        ? () => c.fire(e['onFailure']!)
        : null,
  );
}
