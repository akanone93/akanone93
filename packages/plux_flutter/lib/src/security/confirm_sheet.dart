// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The sheet on which the user confirms a transaction (SEC-027, SEC-028): the
/// amount, the currency and the payee exactly as the SCA key will sign them,
/// each as one labelled row a screen reader reads as a unit.
library;

import 'package:flutter/material.dart';
import 'package:plux_flutter/src/security/sca.dart';

/// Shows [TransactionConfirmSheet] as a modal bottom sheet.
final class SheetTransactionConfirmer implements TransactionConfirmer {
  /// Creates the confirmer.
  const SheetTransactionConfirmer();

  @override
  Future<bool> confirm(BuildContext context, TransactionSummary summary) async {
    final confirmed = await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      builder: (_) => TransactionConfirmSheet(summary: summary),
    );
    return confirmed ?? false;
  }
}

/// The content of the confirmation sheet. It pops `true` when the user
/// confirms and `false` when they cancel; dismissing it pops nothing, which
/// is a cancellation.
final class TransactionConfirmSheet extends StatelessWidget {
  /// Creates the sheet for [summary].
  const TransactionConfirmSheet({required this.summary, super.key});

  /// What the user confirms; every field is shown as it is, without
  /// formatting, since it is the very text that is signed.
  final TransactionSummary summary;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Semantics(
      scopesRoute: true,
      namesRoute: true,
      explicitChildNodes: true,
      label: scaPromptTitle,
      child: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(24, 24, 24, 16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Semantics(
                header: true,
                child: Text(scaPromptTitle, style: theme.textTheme.titleLarge),
              ),
              const SizedBox(height: 16),
              _Row(label: 'Amount', value: summary.amount),
              _Row(label: 'Currency', value: summary.currency),
              _Row(label: 'Payee', value: summary.payee),
              if (summary.description case final description?)
                _Row(label: 'Description', value: description),
              const SizedBox(height: 24),
              FilledButton(
                onPressed: () => Navigator.of(context).pop(true),
                child: const Text('Confirm'),
              ),
              const SizedBox(height: 8),
              TextButton(
                onPressed: () => Navigator.of(context).pop(false),
                child: const Text('Cancel'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// One label and its value, read together.
final class _Row extends StatelessWidget {
  const _Row({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return MergeSemantics(
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(label, style: theme.textTheme.labelMedium),
            Text(value, style: theme.textTheme.titleMedium),
          ],
        ),
      ),
    );
  }
}
