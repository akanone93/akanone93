// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Masking of sensitive values in text displays and inputs (SEC-092,
/// SCH-012): the compiler marks a node whose binding reads a sensitive
/// path with the `Masked` hint, and the runtime shows bullets, or an
/// obscured input, until the user taps the unmask button.
library;

import 'package:flutter/material.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';

/// The `Masked` bit of `Node.hints` (`NodeHints`).
const int maskedHint = 4;

WidgetDescriptor _widget(String type) =>
    widgetDescriptors.firstWhere((d) => d.type == type);

/// The text displays the compiler can mark masked.
final Set<int> _displays = {
  for (final t in const ['Text', 'SelectableText']) _widget(t).id,
};

/// The values a masked input is built with in place of its own, by widget
/// and prop ID: obscured, with no suggestions, autocorrection, keyboard
/// learning or selection menu.
final Map<int, Map<int, Object>> _inputs = {
  for (final t in const ['TextField', 'TextFormField', 'CupertinoTextField'])
    _widget(t).id: {
      for (final e in const <String, Object>{
        'obscureText': true,
        'enableSuggestions': false,
        'autocorrect': false,
        'enableIMEPersonalizedLearning': false,
        'enableInteractiveSelection': false,
      }.entries)
        ?_widget(t).props[e.key]: e.value,
    },
};

/// Whether widget [id] is a text display or input that masks.
bool isMaskable(int id) => _displays.contains(id) || _inputs.containsKey(id);

/// Whether widget [id] is an input.
bool isMaskedInput(int id) => _inputs.containsKey(id);

/// The value a masked input of widget [widget] is built with for prop
/// [prop], or null when its own value stands.
Object? maskedProp(int widget, int prop) => _inputs[widget]?[prop];

/// What a masked value shows in place of itself: the same however long the
/// value is.
const String maskBullets = '••••••••';

/// Shows [revealed] only after the user taps the unmask button; until then
/// a display shows bullets and an input is [hidden], the same input built
/// obscured. Nothing of the value reaches the semantics tree while masked.
/// The value is masked again when the app leaves the foreground.
final class MaskedNode extends StatefulWidget {
  /// Creates the node. [hidden] is null for a display.
  const MaskedNode({super.key, required this.revealed, this.hidden});

  /// The node built as it would be without masking.
  final Widget revealed;

  /// The input built obscured, or null for a display.
  final Widget? hidden;

  @override
  State<MaskedNode> createState() => _MaskedNodeState();
}

final class _MaskedNodeState extends State<MaskedNode>
    with WidgetsBindingObserver {
  bool _shown = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed && _shown) {
      setState(() => _shown = false);
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final input = widget.hidden != null;
    final Widget value;
    if (_shown) {
      value = widget.revealed;
    } else if (input) {
      value = widget.hidden!;
    } else {
      value = Semantics(
        label: 'Hidden value',
        excludeSemantics: true,
        child: const Text(maskBullets),
      );
    }
    final toggle = IconButton(
      icon: Icon(_shown ? Icons.visibility_off : Icons.visibility),
      tooltip: _shown ? 'Hide value' : 'Show value',
      onPressed: () => setState(() => _shown = !_shown),
    );
    return Row(
      mainAxisSize: input ? MainAxisSize.max : MainAxisSize.min,
      children: [
        input ? Expanded(child: value) : Flexible(child: value),
        toggle,
      ],
    );
  }
}
