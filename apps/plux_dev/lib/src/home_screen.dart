// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Dev app's home: the pairing status, the ways to pair, and the page
/// to open once a session is applying dev bundles (DEV-001, DEV-003).
library;

import 'package:flutter/material.dart';
import 'package:plux_dev/development_roots.dart';
import 'package:plux_devtools/plux_devtools.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// Opens the page named [route]; `Plux.open` unless a test replaces it.
typedef OpenPage = Future<void> Function(BuildContext context, String route);

Future<void> _openWithRuntime(BuildContext context, String route) =>
    Plux.open<void>(context, route);

/// The home screen.
final class DevHomeScreen extends StatefulWidget {
  /// Creates the screen. Pairing is offered only when [hasRoots] and the
  /// session [PluxDevSession.isAvailable].
  const DevHomeScreen({
    super.key,
    required this.session,
    required this.hasRoots,
    required this.scan,
    this.openPage = _openWithRuntime,
  });

  /// This build's pairing with a dev session.
  final PluxDevSession session;

  /// Whether the build pins any development root.
  final bool hasRoots;

  /// Scans a pairing QR code; null when the user gave up.
  final Future<String?> Function(BuildContext context) scan;

  /// Opens a page of the app being previewed.
  final OpenPage openPage;

  @override
  State<DevHomeScreen> createState() => _DevHomeScreenState();
}

final class _DevHomeScreenState extends State<DevHomeScreen> {
  final TextEditingController _link = TextEditingController();
  final TextEditingController _route = TextEditingController(text: 'home');
  bool _pasting = false;

  bool get _canPair => widget.hasRoots && widget.session.isAvailable;

  @override
  void dispose() {
    _link.dispose();
    _route.dispose();
    super.dispose();
  }

  Future<void> _scan() async {
    final code = await widget.scan(context);
    if (code != null && _canPair) await widget.session.pair(code);
  }

  Future<void> _paste() async {
    final text = _link.text.trim();
    if (text.isEmpty || !_canPair) return;
    await widget.session.pair(text);
    if (mounted) setState(() => _pasting = false);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Plux Dev')),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          if (!widget.hasRoots)
            const _Notice(noDevelopmentRootsMessage)
          else if (!widget.session.isAvailable)
            const _Notice(
              'This build has no dev channel. Build it with '
              '--dart-define=plux.devtools=true.',
            ),
          ValueListenableBuilder<PluxDevSessionState>(
            valueListenable: widget.session.state,
            builder: (context, state, _) => Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _Status(state: state),
                if (state is PluxDevPaired)
                  TextButton(
                    key: const Key('leave-session'),
                    onPressed: widget.session.end,
                    child: const Text('Leave the session'),
                  ),
              ],
            ),
          ),
          const SizedBox(height: 16),
          FilledButton.icon(
            key: const Key('scan-qr'),
            onPressed: _canPair ? _scan : null,
            icon: const Icon(Icons.qr_code_scanner),
            label: const Text('Scan QR'),
          ),
          const SizedBox(height: 8),
          if (_pasting) ...[
            TextField(
              key: const Key('link-field'),
              controller: _link,
              autocorrect: false,
              decoration: const InputDecoration(
                labelText: 'Pairing link',
                hintText: 'plux-dev://pair?…',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 8),
            FilledButton(
              key: const Key('pair-link'),
              onPressed: _canPair ? _paste : null,
              child: const Text('Pair'),
            ),
          ] else
            OutlinedButton.icon(
              key: const Key('paste-link'),
              onPressed: _canPair
                  ? () => setState(() => _pasting = true)
                  : null,
              icon: const Icon(Icons.link),
              label: const Text('Paste link'),
            ),
          const Divider(height: 32),
          TextField(
            key: const Key('route-field'),
            controller: _route,
            autocorrect: false,
            decoration: const InputDecoration(
              labelText: 'Page',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 8),
          OutlinedButton(
            key: const Key('open-page'),
            onPressed: () => widget.openPage(context, _route.text.trim()),
            child: const Text('Open page'),
          ),
        ],
      ),
    ),
  );
}

final class _Notice extends StatelessWidget {
  const _Notice(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: Text(
      text,
      style: TextStyle(color: Theme.of(context).colorScheme.error),
    ),
  );
}

final class _Status extends StatelessWidget {
  const _Status({required this.state});

  final PluxDevSessionState state;

  @override
  Widget build(BuildContext context) => Semantics(
    liveRegion: true,
    child: Text(switch (state) {
      PluxDevIdle() => 'Not paired',
      PluxDevPairingNow() => 'Pairing…',
      PluxDevPaired(:final sessionId, :final deviceId) =>
        'Paired with session $sessionId as device $deviceId',
      PluxDevError(:final code) => 'Pairing failed ($code)',
    }, style: Theme.of(context).textTheme.titleMedium),
  );
}
