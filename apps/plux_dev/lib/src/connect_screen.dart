// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Connecting this phone to Studio, once (DEV-002, ADR-0104): scan the code
/// Studio shows under Profile › Phones, then pick the number Studio shows.
library;

import 'package:flutter/material.dart';
import 'package:plux_dev/src/phone.dart';
import 'package:plux_dev/src/ui/number_picker.dart';
import 'package:plux_dev/src/ui/theme.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// Claims a sign-in with this phone's key; the Dev app passes
/// `PhoneDirectory.claim`.
typedef ClaimSignIn = Future<PhoneSignIn> Function(DevSignInLink link);

/// The words for a failed connection.
String connectFailure(Object error) => switch (error) {
  DevPhoneException(failure: DevPhoneFailure.wrongNumber) => "That wasn't the number Studio showed. Show a new code in Studio and scan it again.",
  DevPhoneException(failure: DevPhoneFailure.closed) =>
    'This code expired or was already used. Show a new one in Studio.',
  DevPhoneException(failure: DevPhoneFailure.unreachable) => "Studio's server can't be reached. Check that the phone reaches it (adb reverse for a USB phone) and try again.",
  DevPhoneException(failure: DevPhoneFailure.rateLimited) =>
    'Too many attempts. Wait a few minutes and try again.',
  DevPhoneException(:final message) => 'The phone could not connect: $message',
  FormatException() => "That isn't a Plux Dev code. In Studio, open Profile › Phones and choose Connect a phone.",
  _ => 'The phone could not connect: $error',
};

/// The first screen of an unconnected phone.
final class ConnectScreen extends StatefulWidget {
  /// Creates the screen.
  const ConnectScreen({
    super.key,
    required this.scan,
    required this.claim,
    required this.onSignedIn,
  });

  /// Opens the camera and returns the code read, or null.
  final Future<String?> Function(BuildContext context) scan;

  /// Claims a sign-in.
  final ClaimSignIn claim;

  /// Receives the signed-in phone.
  final ValueChanged<SignedInPhone> onSignedIn;

  @override
  State<ConnectScreen> createState() => _ConnectScreenState();
}

final class _ConnectScreenState extends State<ConnectScreen> {
  bool _busy = false;
  String? _error;

  Future<void> _connect(String? text) async {
    if (text == null || text.trim().isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      if (!DevSignInLink.matches(text)) {
        throw const FormatException('not a sign-in link');
      }
      final claim = await widget.claim(DevSignInLink.parse(text));
      if (!mounted) return;
      final picked = await pickNumber(
        context,
        options: claim.options,
        title: 'Pick the number shown in Studio',
        detail: _whom(claim),
        expiresAt: claim.expiresAt,
      );
      if (picked == null) {
        _fail(null);
        return;
      }
      final phone = await claim.complete(picked);
      if (mounted) widget.onSignedIn(phone);
    } on Object catch (e) {
      _fail(connectFailure(e));
    }
  }

  String _whom(PhoneSignIn claim) {
    final who = [
      claim.userDisplay,
      claim.organizationName,
    ].where((s) => s.isNotEmpty).join(' · ');
    return who.isEmpty
        ? 'Connecting this phone'
        : 'Connecting this phone as $who';
  }

  void _fail(String? message) {
    if (!mounted) return;
    setState(() {
      _busy = false;
      _error = message;
    });
  }

  Future<void> _paste() async {
    final controller = TextEditingController();
    final text = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Paste the link'),
        content: TextField(
          key: const Key('link-field'),
          controller: controller,
          autofocus: true,
          autocorrect: false,
          keyboardType: TextInputType.url,
          decoration: const InputDecoration(hintText: 'plux-dev://signin?…'),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Cancel'),
          ),
          TextButton(
            key: const Key('link-connect'),
            onPressed: () => Navigator.of(context).pop(controller.text),
            child: const Text('Connect'),
          ),
        ],
      ),
    );
    controller.dispose();
    await _connect(text);
  }

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return Scaffold(
      body: CustomScrollView(
        slivers: [
          // The name large at first, settling into the toolbar on scroll.
          const SliverAppBar.large(
            leading: Padding(padding: EdgeInsets.all(12), child: _Mark()),
            title: Text('Plux Dev'),
          ),
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(
              DevSizes.page,
              0,
              DevSizes.page,
              DevSizes.page,
            ),
            sliver: SliverList.list(
              children: [
                Text(
                  'Your Studio apps, live on this phone.',
                  style: text.bodyLarge?.copyWith(color: DevColors.muted),
                ),
                const SizedBox(height: 24),
                const DevCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      _Step(
                        n: 1,
                        text: 'In Studio, open Profile › Phones and choose Connect a phone.',
                      ),
                      SizedBox(height: 14),
                      _Step(n: 2, text: 'Scan the code with this phone.'),
                      SizedBox(height: 14),
                      _Step(n: 3, text: 'Pick the number Studio shows.'),
                    ],
                  ),
                ),
                const SizedBox(height: 24),
                FilledButton.icon(
                  key: const Key('scan'),
                  onPressed: _busy
                      ? null
                      : () async => _connect(await widget.scan(context)),
                  icon: _busy
                      ? const SizedBox.square(
                          dimension: 18,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: Colors.white,
                          ),
                        )
                      : const Icon(Icons.qr_code_scanner_rounded, size: 20),
                  label: Text(_busy ? 'Connecting…' : 'Scan the code'),
                ),
                const SizedBox(height: 4),
                TextButton(
                  key: const Key('paste'),
                  onPressed: _busy ? null : _paste,
                  child: const Text('Paste a link instead'),
                ),
                if (_error case final error?) ...[
                  const SizedBox(height: 12),
                  _Notice(text: error),
                ],
                const SizedBox(height: 20),
                Text(
                  'This phone stays connected. Sign it out any time in the app or in Studio.',
                  style: text.bodySmall,
                  textAlign: TextAlign.center,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

final class _Mark extends StatelessWidget {
  const _Mark();

  @override
  Widget build(BuildContext context) => DecoratedBox(
    decoration: BoxDecoration(
      color: DevColors.primary,
      borderRadius: BorderRadius.circular(10),
      boxShadow: const [
        BoxShadow(
          color: Color(0x335B3DF5),
          blurRadius: 12,
          offset: Offset(0, 4),
        ),
      ],
    ),
    child: const SizedBox.square(
      dimension: 32,
      child: Icon(Icons.bolt_rounded, color: Colors.white, size: 20),
    ),
  );
}

final class _Step extends StatelessWidget {
  const _Step({required this.n, required this.text});

  final int n;
  final String text;

  @override
  Widget build(BuildContext context) => Row(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      DecoratedBox(
        decoration: BoxDecoration(
          color: DevColors.primary.withValues(alpha: 0.1),
          shape: BoxShape.circle,
        ),
        child: SizedBox.square(
          dimension: 24,
          child: Center(
            child: Text(
              '$n',
              style: const TextStyle(
                color: DevColors.primary,
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
        ),
      ),
      const SizedBox(width: 12),
      Expanded(
        child: Padding(
          padding: const EdgeInsets.only(top: 2),
          child: Text(
            text,
            style: Theme.of(context).textTheme.bodyMedium
                ?.copyWith(color: DevColors.ink),
          ),
        ),
      ),
    ],
  );
}

/// A soft red notice.
final class _Notice extends StatelessWidget {
  const _Notice({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) => Semantics(
    liveRegion: true,
    child: DecoratedBox(
      decoration: BoxDecoration(
        color: DevColors.danger.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(DevSizes.button),
      ),
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Text(
          text,
          style: const TextStyle(color: Color(0xFFB4232A), height: 1.4),
        ),
      ),
    ),
  );
}
