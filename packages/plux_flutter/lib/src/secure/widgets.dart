// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The secure input widgets (SEC-090, SEC-092, ADR-0059): hardened text
/// entry, a PIN pad, a one-time code field and a biometric button. They are
/// plain Flutter widgets with no knowledge of nodes or bundles; the
/// builders in `render/builders/secure.dart` give them their props and
/// events.
library;

import 'dart:async';
import 'dart:math';

import 'package:flutter/foundation.dart'
    show TargetPlatform, defaultTargetPlatform;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:plux_flutter/src/secure/otp.dart';
import 'package:plux_flutter/src/secure/pin_layout.dart';
import 'package:plux_flutter/src/secure/platform.dart';

/// `SecureTextField`: text entry that leaks nothing through the keyboard or
/// the clipboard (SEC-090).
///
/// Suggestions, autocorrect, smart punctuation and the keyboard's
/// personalised learning are off (`IME_FLAG_NO_PERSONALIZED_LEARNING` on
/// Android), the text is obscured, which is secure text entry on iOS, and
/// no menu, long press or shortcut copies, cuts or pastes. The characters
/// are shown only after the user taps the eye button (SEC-092), and are
/// hidden again when the field loses focus.
final class SecureTextFieldView extends StatefulWidget {
  /// Creates the field over [controller].
  const SecureTextFieldView({
    super.key,
    required this.controller,
    this.label,
    this.obscure = true,
    this.maxLength,
    this.onChanged,
    this.onSubmitted,
    this.onActivity,
  });

  /// Holds the text.
  final TextEditingController controller;

  /// The field's label.
  final String? label;

  /// Whether the characters are hidden; when false there is nothing to
  /// unmask.
  final bool obscure;

  /// The most characters the field takes.
  final int? maxLength;

  /// Called on every change with the text.
  final ValueChanged<String>? onChanged;

  /// Called when the user submits.
  final ValueChanged<String>? onSubmitted;

  /// Called on every edit, for the inactivity lock (SEC-094).
  final VoidCallback? onActivity;

  @override
  State<SecureTextFieldView> createState() => _SecureTextFieldViewState();
}

final class _SecureTextFieldViewState extends State<SecureTextFieldView> {
  final FocusNode _focus = FocusNode();
  bool _revealed = false;

  /// The keys that copy, cut or paste on the platforms with a keyboard.
  static final Map<ShortcutActivator, Intent> _clipboardKeys = {
    for (final key in [
      LogicalKeyboardKey.keyC,
      LogicalKeyboardKey.keyX,
      LogicalKeyboardKey.keyV,
    ]) ...{
      SingleActivator(key, control: true):
          const DoNothingAndStopPropagationIntent(),
      SingleActivator(key, meta: true):
          const DoNothingAndStopPropagationIntent(),
    },
    const SingleActivator(LogicalKeyboardKey.insert, shift: true):
        const DoNothingAndStopPropagationIntent(),
    const SingleActivator(LogicalKeyboardKey.insert, control: true):
        const DoNothingAndStopPropagationIntent(),
    const SingleActivator(LogicalKeyboardKey.delete, shift: true):
        const DoNothingAndStopPropagationIntent(),
  };

  @override
  void initState() {
    super.initState();
    _focus.addListener(_focusChanged);
  }

  void _focusChanged() {
    if (!_focus.hasFocus && _revealed) setState(() => _revealed = false);
  }

  @override
  void dispose() {
    _focus
      ..removeListener(_focusChanged)
      ..dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final hidden = widget.obscure && !_revealed;
    return Shortcuts(
      shortcuts: _clipboardKeys,
      child: TextField(
        controller: widget.controller,
        focusNode: _focus,
        obscureText: hidden,
        enableSuggestions: false,
        autocorrect: false,
        enableIMEPersonalizedLearning: false,
        enableInteractiveSelection: false,
        contextMenuBuilder: (context, state) => const SizedBox.shrink(),
        keyboardType: TextInputType.visiblePassword,
        textCapitalization: TextCapitalization.none,
        smartDashesType: SmartDashesType.disabled,
        smartQuotesType: SmartQuotesType.disabled,
        maxLength: widget.maxLength,
        decoration: InputDecoration(
          labelText: widget.label,
          counterText: '',
          suffixIcon: widget.obscure
              ? IconButton(
                  icon: Icon(
                    _revealed ? Icons.visibility_off : Icons.visibility,
                  ),
                  tooltip: _revealed ? 'Hide' : 'Show',
                  onPressed: () => setState(() => _revealed = !_revealed),
                )
              : null,
        ),
        onChanged: (text) {
          widget.onActivity?.call();
          widget.onChanged?.call(text);
        },
        onSubmitted: widget.onSubmitted,
      ),
    );
  }
}

/// `SecurePinPad`: a PIN keypad that is not the system keyboard (SEC-090).
///
/// With [randomize] the digits sit in a new order each time the pad
/// appears and after each completed entry, so a finger's position says
/// nothing about the digit. Keys show no pressed state, ripple or
/// animation; a press only vibrates, when [haptics] is on. Each key is a
/// button for screen readers, named by its digit; the entered digits are
/// never announced, only how many there are.
final class SecurePinPadView extends StatefulWidget {
  /// Creates the pad for a PIN of [length] digits.
  const SecurePinPadView({
    super.key,
    required this.length,
    required this.onCompleted,
    this.randomize = false,
    this.haptics = true,
    this.random = Random.secure,
    this.onActivity,
  });

  /// The number of digits.
  final int length;

  /// Called with the PIN once [length] digits are entered. The pad clears
  /// itself first, so a wrong PIN can be entered again.
  final ValueChanged<String> onCompleted;

  /// Whether the digit positions are shuffled.
  final bool randomize;

  /// Whether a key press vibrates.
  final bool haptics;

  /// Creates the generator the layout is drawn from, once per display; a
  /// seeded one in tests.
  final Random Function() random;

  /// Called on every key press, for the inactivity lock (SEC-094).
  final VoidCallback? onActivity;

  @override
  State<SecurePinPadView> createState() => _SecurePinPadViewState();
}

final class _SecurePinPadViewState extends State<SecurePinPadView> {
  final List<int> _entered = [];
  late List<int> _order = _layout();

  List<int> _layout() =>
      pinDigitOrder(widget.randomize ? widget.random() : null);

  @override
  void didUpdateWidget(SecurePinPadView old) {
    super.didUpdateWidget(old);
    if (old.randomize != widget.randomize) _order = _layout();
    if (old.length != widget.length) _entered.clear();
  }

  @override
  void dispose() {
    _entered.clear();
    super.dispose();
  }

  void _feedback() {
    widget.onActivity?.call();
    if (widget.haptics) unawaited(HapticFeedback.selectionClick());
  }

  void _press(int digit) {
    _feedback();
    _entered.add(digit);
    if (_entered.length < widget.length) {
      setState(() {});
      return;
    }
    final pin = _entered.join();
    _entered.clear();
    setState(() => _order = _layout());
    widget.onCompleted(pin);
  }

  void _delete() {
    _feedback();
    if (_entered.isEmpty) return;
    setState(_entered.removeLast);
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    Widget key(int i) => _PinKey(
      label: '${_order[i]}',
      semanticLabel: '${_order[i]}',
      onTap: () => _press(_order[i]),
    );
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Semantics(
          label: 'PIN',
          value: '${_entered.length} of ${widget.length} digits entered',
          liveRegion: true,
          excludeSemantics: true,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              for (var i = 0; i < widget.length; i++)
                Padding(
                  padding: const EdgeInsets.all(6),
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      border: Border.all(color: scheme.onSurface, width: 1.5),
                      color: i < _entered.length ? scheme.onSurface : null,
                    ),
                    child: const SizedBox.square(dimension: 14),
                  ),
                ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        for (var row = 0; row < 3; row++)
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [for (var col = 0; col < 3; col++) key(row * 3 + col)],
          ),
        Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            const SizedBox(width: _PinKey.width, height: _PinKey.height),
            key(9),
            _PinKey(label: '⌫', semanticLabel: 'Delete', onTap: _delete),
          ],
        ),
      ],
    );
  }
}

/// One key of the pad: a fixed look, whether pressed or not.
final class _PinKey extends StatelessWidget {
  const _PinKey({
    required this.label,
    required this.semanticLabel,
    required this.onTap,
  });

  static const double width = 80;
  static const double height = 60;

  final String label;
  final String semanticLabel;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) => Semantics(
    button: true,
    label: semanticLabel,
    excludeSemantics: true,
    onTap: onTap,
    child: GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      child: SizedBox(
        width: width,
        height: height,
        child: Center(
          child: Text(label, style: Theme.of(context).textTheme.headlineSmall),
        ),
      ),
    ),
  );
}

/// `OtpInput`: a field for a one-time code (SEC-090).
///
/// The platform's one-time-code autofill (`AutofillHints.oneTimeCode`: the
/// keyboard's suggestion on iOS and Android) fills it when [autofill] is
/// on. Characters outside the [format]'s alphabet, or past its length, are
/// dropped; pasted or autofilled text is accepted only if all of it fits
/// the format. [onCompleted] fires once the code has all its characters.
///
/// On Android, when the field gains focus empty, [sms] waits for the user
/// to share the code's SMS (the SMS User Consent API, no SMS permission,
/// SEC-090); the first run of exactly the format's length in the message
/// fills the field as if typed. The wait ends when the field loses focus or
/// goes. A failed call leaves the field as it was.
final class OtpInputView extends StatefulWidget {
  /// Creates the field over [controller].
  const OtpInputView({
    super.key,
    required this.controller,
    required this.format,
    required this.onCompleted,
    this.autofill = true,
    this.label = 'One-time code',
    this.onActivity,
    this.sms,
  });

  /// Holds the code.
  final TextEditingController controller;

  /// The code's length and alphabet.
  final OtpFormat format;

  /// The SMS User Consent API, used on Android when [autofill] is on; null
  /// to go without.
  final SmsConsentPlatform? sms;

  /// Called with the code once it is complete.
  final ValueChanged<String>? onCompleted;

  /// Whether the platform may fill the code in.
  final bool autofill;

  /// The field's accessible name.
  final String label;

  /// Called on every edit, for the inactivity lock (SEC-094).
  final VoidCallback? onActivity;

  @override
  State<OtpInputView> createState() => _OtpInputViewState();
}

final class _OtpInputViewState extends State<OtpInputView> {
  final FocusNode _focus = FocusNode();
  bool _complete = false;
  bool _refused = false;
  bool _waiting = false;

  @override
  void initState() {
    super.initState();
    _focus.addListener(_focusChanged);
  }

  @override
  void dispose() {
    _focus.removeListener(_focusChanged);
    _cancelSms();
    _focus.dispose();
    super.dispose();
  }

  void _focusChanged() {
    if (!_focus.hasFocus) {
      _cancelSms();
    } else if (widget.controller.text.isEmpty) {
      unawaited(_awaitSms());
    }
  }

  void _cancelSms() {
    if (!_waiting) return;
    _waiting = false;
    unawaited(widget.sms?.cancel());
  }

  Future<void> _awaitSms() async {
    final sms = widget.sms;
    if (sms == null ||
        !widget.autofill ||
        _waiting ||
        defaultTargetPlatform != TargetPlatform.android) {
      return;
    }
    _waiting = true;
    final String? message;
    try {
      message = await sms.awaitSmsConsent();
    } on Object {
      // The field stays as it is; the user types the code.
      _waiting = false;
      return;
    }
    if (!_waiting || !mounted) return;
    _waiting = false;
    final code = message == null ? null : widget.format.extract(message);
    if (code == null || widget.controller.text.isNotEmpty) return;
    widget.controller.value = TextEditingValue(
      text: code,
      selection: TextSelection.collapsed(offset: code.length),
    );
    _changed(code);
  }

  void _changed(String text) {
    widget.onActivity?.call();
    final done = text.length == widget.format.length;
    if (_refused) setState(() => _refused = false);
    if (done && !_complete) {
      if (widget.autofill) TextInput.finishAutofillContext(shouldSave: false);
      widget.onCompleted?.call(text);
    }
    _complete = done;
  }

  @override
  Widget build(BuildContext context) {
    final numeric = widget.format.charset == OtpCharset.numeric;
    return TextField(
      controller: widget.controller,
      focusNode: _focus,
      autofillHints: widget.autofill ? const [AutofillHints.oneTimeCode] : null,
      keyboardType: numeric
          ? TextInputType.number
          : TextInputType.visiblePassword,
      textCapitalization: TextCapitalization.none,
      enableSuggestions: false,
      autocorrect: false,
      enableIMEPersonalizedLearning: false,
      textAlign: TextAlign.center,
      style: Theme.of(context).textTheme.headlineSmall?.copyWith(
        letterSpacing: 8,
        fontFeatures: const [FontFeature.tabularFigures()],
      ),
      inputFormatters: [
        OtpFormatter(
          widget.format,
          onRefused: () => setState(() => _refused = true),
        ),
      ],
      decoration: InputDecoration(
        labelText: widget.label,
        counterText: '',
        errorText: _refused ? 'That is not a valid code.' : null,
      ),
      onChanged: _changed,
    );
  }
}

/// `BiometricButton`: a button that asks the user to confirm with a
/// biometric or the device credential, then reports the result. A press
/// while the prompt is open does nothing.
final class BiometricButtonView extends StatefulWidget {
  /// Creates the button; [ask] shows the prompt, and a null one disables
  /// the button.
  const BiometricButtonView({
    super.key,
    required this.label,
    required this.ask,
    this.onSuccess,
    this.onFailure,
  });

  /// The button text.
  final String label;

  /// Shows the system prompt and completes with how it ended.
  final Future<BiometricOutcome> Function()? ask;

  /// Called when the user confirms.
  final VoidCallback? onSuccess;

  /// Called when the confirmation fails or is cancelled.
  final VoidCallback? onFailure;

  @override
  State<BiometricButtonView> createState() => _BiometricButtonViewState();
}

final class _BiometricButtonViewState extends State<BiometricButtonView> {
  bool _asking = false;

  Future<void> _press() async {
    final ask = widget.ask;
    if (ask == null || _asking) return;
    setState(() => _asking = true);
    var outcome = BiometricOutcome.failed;
    try {
      outcome = await ask();
    } finally {
      if (mounted) setState(() => _asking = false);
    }
    if (outcome == BiometricOutcome.success) {
      widget.onSuccess?.call();
    } else {
      widget.onFailure?.call();
    }
  }

  @override
  Widget build(BuildContext context) => FilledButton.icon(
    onPressed: widget.ask == null || _asking ? null : () => unawaited(_press()),
    icon: const Icon(Icons.fingerprint),
    label: Text(widget.label),
  );
}
