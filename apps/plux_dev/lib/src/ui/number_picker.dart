// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The number match (DEV-002, ADR-0104): Studio shows a two-digit number,
/// the phone offers three, and the person picks the one Studio shows. It
/// proves the person holding the phone sees their own Studio, so a copied QR
/// code or a phone in other hands gets one guess.
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:plux_dev/src/ui/theme.dart';

/// Asks the person to pick, among [options], the number Studio shows; null
/// when they cancel or [expiresAt] passes. [title] says what it is for and
/// [detail] whom or what it concerns.
Future<int?> pickNumber(
  BuildContext context, {
  required List<int> options,
  required String title,
  required String detail,
  DateTime? expiresAt,
}) => showModalBottomSheet<int>(
  context: context,
  isScrollControlled: true,
  useSafeArea: true,
  builder: (_) => NumberPicker(
    options: options,
    title: title,
    detail: detail,
    expiresAt: expiresAt,
  ),
);

/// The sheet of the number match: three number buttons and a countdown.
final class NumberPicker extends StatefulWidget {
  /// Creates the picker.
  const NumberPicker({
    super.key,
    required this.options,
    required this.title,
    required this.detail,
    this.expiresAt,
    this.now = DateTime.now,
  });

  /// The numbers to offer, in order.
  final List<int> options;

  /// What the pick is for.
  final String title;

  /// Whom or what it concerns.
  final String detail;

  /// When the request expires; the sheet closes then.
  final DateTime? expiresAt;

  /// The clock; tests replace it.
  final DateTime Function() now;

  @override
  State<NumberPicker> createState() => _NumberPickerState();
}

final class _NumberPickerState extends State<NumberPicker> {
  Timer? _tick;

  int get _left {
    final at = widget.expiresAt;
    if (at == null) return -1;
    return at.difference(widget.now()).inSeconds.clamp(0, 999);
  }

  @override
  void initState() {
    super.initState();
    if (widget.expiresAt != null) {
      _tick = Timer.periodic(const Duration(seconds: 1), (_) {
        if (!mounted) return;
        if (_left == 0) {
          Navigator.of(context).pop();
          return;
        }
        setState(() {});
      });
    }
  }

  @override
  void dispose() {
    _tick?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final left = _left;
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(DevSizes.page, 0, DevSizes.page, 24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            widget.title,
            style: text.titleLarge,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 8),
          Text(
            widget.detail,
            style: text.bodyMedium,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 24),
          Row(
            children: [
              for (final (i, n) in widget.options.indexed) ...[
                if (i > 0) const SizedBox(width: 12),
                Expanded(child: _NumberButton(number: n)),
              ],
            ],
          ),
          const SizedBox(height: 16),
          if (left >= 0)
            Text(
              'Expires in $left s',
              style: text.bodyMedium,
              textAlign: TextAlign.center,
            ),
          const SizedBox(height: 8),
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Cancel'),
          ),
        ],
      ),
    );
  }
}

final class _NumberButton extends StatelessWidget {
  const _NumberButton({required this.number});

  final int number;

  @override
  Widget build(BuildContext context) => Semantics(
    button: true,
    label: 'Pick $number',
    excludeSemantics: true,
    child: SizedBox(
      height: 72,
      child: OutlinedButton(
        key: Key('number-$number'),
        style: OutlinedButton.styleFrom(
          shape: const RoundedRectangleBorder(
            borderRadius: BorderRadius.all(Radius.circular(DevSizes.card)),
          ),
          side: const BorderSide(color: DevColors.line, width: 1.5),
          backgroundColor: DevColors.surface,
        ),
        onPressed: () => Navigator.of(context).pop(number),
        child: Text(
          '$number',
          style: const TextStyle(
            fontSize: 28,
            fontWeight: FontWeight.w700,
            color: DevColors.ink,
            fontFeatures: [FontFeature.tabularFigures()],
          ),
        ),
      ),
    ),
  );
}
