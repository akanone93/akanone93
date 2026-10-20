// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What Plux shows in place of its pages while a mandatory update is
/// pending (REL-070): the host's `PluxConfig.updateScreen`, else
/// [PluxDefaultUpdateScreen].
library;

import 'package:flutter/material.dart';
import 'package:plux_flutter/src/core/config.dart';
import 'package:plux_flutter/src/l10n/locale_host.dart';

/// Builds the update screen for the control document's [messageKey]: the
/// host's `PluxConfig.updateScreen`, or [PluxDefaultUpdateScreen].
Widget buildUpdateScreen(
  BuildContext context,
  PluxConfig config,
  String messageKey,
) =>
    config.updateScreen?.call(context, messageKey) ??
    PluxDefaultUpdateScreen(messageKey: messageKey);

/// The update screen Plux shows when the host configures none: a plain,
/// themed panel with a progress indicator, a localised title and the text
/// the message key stands for, or the key itself while the runtime strings
/// have no text for it.
final class PluxDefaultUpdateScreen extends StatelessWidget {
  /// Creates the screen for [messageKey].
  const PluxDefaultUpdateScreen({required this.messageKey, super.key});

  /// The translation key the control document names.
  final String messageKey;

  @override
  Widget build(BuildContext context) {
    final strings = effectiveRuntimeStrings(context);
    final text = strings[messageKey];
    final theme = Theme.of(context);
    return ColoredBox(
      color: theme.colorScheme.surface,
      child: Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const CircularProgressIndicator(),
              const SizedBox(height: 16),
              Text(
                strings['plux.update.required'],
                textAlign: TextAlign.center,
                style: theme.textTheme.titleMedium,
              ),
              const SizedBox(height: 8),
              Text(
                text.isEmpty ? messageKey : text,
                textAlign: TextAlign.center,
                style: theme.textTheme.bodyMedium,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
