// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The debug overlay: a badge over the host app that opens a panel with the
/// sync status, the active release and the problems the runtime reported.
library;

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:plux_devtools/src/dev_session.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// Wraps the host app with the Plux debug overlay. In release builds it is
/// [child] alone, so the overlay is tree-shaken away.
///
/// ```dart
/// MaterialApp(builder: (context, child) => PluxDevtools(child: child!));
/// ```
final class PluxDevtools extends StatefulWidget {
  /// Creates the overlay over [child].
  const PluxDevtools({
    super.key,
    required this.child,
    this.enabled = true,
    this.diagnostics,
    this.onSync,
    this.session,
    this.scan,
  });

  /// The host app.
  final Widget child;

  /// Whether the overlay shows; it never shows in release builds.
  final bool enabled;

  /// The diagnostics shown; `Plux.diagnostics` when null.
  final PluxDiagnostics? diagnostics;

  /// Runs a sync; `Plux.sync` when null.
  final Future<Object?> Function()? onSync;

  /// The controller of this build's pairing with a dev session (DEV-040);
  /// without it the overlay has no pairing tab.
  final PluxDevSession? session;

  /// Scans a pairing QR code and returns its text, or null when the user
  /// cancelled. The host provides it (the Dev app passes `plux_scanner`);
  /// without it the overlay offers the paste field alone.
  final Future<String?> Function(BuildContext context)? scan;

  @override
  State<PluxDevtools> createState() => _PluxDevtoolsState();
}

enum _Tab { sync, release, log, session }

final class _PluxDevtoolsState extends State<PluxDevtools> {
  bool _open = false;
  _Tab _tab = _Tab.sync;
  bool _syncing = false;

  PluxDiagnostics? get _diagnostics =>
      widget.diagnostics ?? (Plux.isInitialized ? Plux.diagnostics : null);

  Future<void> _sync() async {
    setState(() => _syncing = true);
    try {
      await (widget.onSync ?? Plux.sync)();
    } finally {
      if (mounted) setState(() => _syncing = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (kReleaseMode || !widget.enabled) return widget.child;
    final d = _diagnostics;
    return Stack(
      textDirection: TextDirection.ltr,
      children: [
        widget.child,
        if (_open)
          Positioned.fill(
            child: SafeArea(
              child: Align(
                alignment: Alignment.bottomCenter,
                child: _Panel(
                  diagnostics: d,
                  session: widget.session,
                  scan: widget.scan,
                  tab: _tab,
                  syncing: _syncing,
                  onTab: (t) => setState(() => _tab = t),
                  onSync: d == null || _syncing
                      ? null
                      : () => unawaited(_sync()),
                  onClose: () => setState(() => _open = false),
                ),
              ),
            ),
          )
        else
          PositionedDirectional(
            end: 12,
            bottom: 12,
            child: SafeArea(
              child: _Badge(
                diagnostics: d,
                onTap: () => setState(() => _open = true),
              ),
            ),
          ),
      ],
    );
  }
}

/// The closed state: a small badge with the release sequence and a count
/// of reported problems.
final class _Badge extends StatelessWidget {
  const _Badge({required this.diagnostics, required this.onTap});

  final PluxDiagnostics? diagnostics;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final d = diagnostics;
    final listenable = d == null
        ? const _Never()
        : Listenable.merge([d.release, d.log]);
    return Semantics(
      button: true,
      label: 'Open Plux devtools',
      child: Material(
        color: Colors.black87,
        shape: const StadiumBorder(),
        child: InkWell(
          customBorder: const StadiumBorder(),
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            child: ListenableBuilder(
              listenable: listenable,
              builder: (context, _) {
                final seq = d?.release.value?.sequence;
                final problems = d?.log.value.length ?? 0;
                return Text(
                  'Plux ${seq == null ? '–' : '#$seq'}'
                  '${problems == 0 ? '' : ' · $problems!'}',
                  style: const TextStyle(color: Colors.white, fontSize: 12),
                );
              },
            ),
          ),
        ),
      ),
    );
  }
}

final class _Never implements Listenable {
  const _Never();

  @override
  void addListener(VoidCallback listener) {}

  @override
  void removeListener(VoidCallback listener) {}
}

final class _Panel extends StatelessWidget {
  const _Panel({
    required this.diagnostics,
    required this.session,
    required this.scan,
    required this.tab,
    required this.syncing,
    required this.onTab,
    required this.onSync,
    required this.onClose,
  });

  final PluxDiagnostics? diagnostics;
  final PluxDevSession? session;
  final Future<String?> Function(BuildContext context)? scan;
  final _Tab tab;
  final bool syncing;
  final ValueChanged<_Tab> onTab;
  final VoidCallback? onSync;
  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    final d = diagnostics;
    return Material(
      elevation: 8,
      child: SizedBox(
        height: 320,
        child: Column(
          children: [
            Row(
              children: [
                for (final t in _Tab.values)
                  if (t != _Tab.session || session != null)
                    TextButton(
                      onPressed: () => onTab(t),
                      child: Text(
                        t.name,
                        style: TextStyle(
                          fontWeight: t == tab ? FontWeight.bold : null,
                        ),
                      ),
                    ),
                const Spacer(),
                TextButton(onPressed: onClose, child: const Text('close')),
              ],
            ),
            const Divider(height: 1),
            Expanded(
              child: tab == _Tab.session && session != null
                  ? _SessionTab(session!, scan: scan)
                  : d == null
                  ? const Center(child: Text('Plux is not initialized'))
                  : switch (tab) {
                      _Tab.sync => _SyncTab(
                        d,
                        syncing: syncing,
                        onSync: onSync,
                      ),
                      _Tab.release => _ReleaseTab(d),
                      _Tab.log => _LogTab(d),
                      _Tab.session => const SizedBox.shrink(),
                    },
            ),
          ],
        ),
      ),
    );
  }
}

/// The pairing tab: the state of the dev session, a paste field and, when
/// the host provides a scanner, a scan action.
final class _SessionTab extends StatefulWidget {
  const _SessionTab(this.session, {required this.scan});

  final PluxDevSession session;
  final Future<String?> Function(BuildContext context)? scan;

  @override
  State<_SessionTab> createState() => _SessionTabState();
}

final class _SessionTabState extends State<_SessionTab> {
  final TextEditingController _link = TextEditingController();
  final ValueNotifier<bool> _entering = ValueNotifier(false);
  late final Listenable _changes = Listenable.merge([
    widget.session.state,
    _entering,
  ]);

  @override
  void dispose() {
    _entering.dispose();
    _link.dispose();
    super.dispose();
  }

  Future<void> _scan() async {
    final text = await widget.scan!(context);
    if (text == null || !mounted) return;
    await widget.session.pair(text);
  }

  // The overlay sits above the app's navigator, so the text field needs an
  // Overlay of its own.
  @override
  Widget build(BuildContext context) => Overlay(
    initialEntries: [
      OverlayEntry(
        builder: (context) => ListenableBuilder(
          listenable: _changes,
          builder: (context, _) => _content(context),
        ),
      ),
    ],
  );

  Widget _content(BuildContext context) {
    final state = widget.session.state.value;
    return ListView(
      padding: const EdgeInsets.all(12),
      children: [
        Text(switch (state) {
          PluxDevIdle() =>
            widget.session.isAvailable
                ? 'Not paired'
                : 'Dev sessions are not part of this build',
          PluxDevPairingNow() => 'Pairing…',
          PluxDevPaired(:final sessionId, :final deviceId) =>
            'Paired with session $sessionId as device $deviceId',
          PluxDevError(:final code) => 'Pairing failed: $code',
        }),
        const SizedBox(height: 8),
        if (state is PluxDevPaired)
          Align(
            alignment: AlignmentDirectional.centerStart,
            child: FilledButton(
              onPressed: () => unawaited(widget.session.end()),
              child: const Text('End dev session'),
            ),
          )
        else if (widget.session.isAvailable && state is! PluxDevPairingNow) ...[
          if (_entering.value) ...[
            TextField(
              controller: _link,
              autocorrect: false,
              enableSuggestions: false,
              decoration: const InputDecoration(
                labelText: 'Pairing link',
                hintText: 'plux-dev://pair?…',
              ),
            ),
            const SizedBox(height: 8),
            Align(
              alignment: AlignmentDirectional.centerStart,
              child: FilledButton(
                onPressed: () => unawaited(widget.session.pair(_link.text)),
                child: const Text('Pair'),
              ),
            ),
          ] else
            Align(
              alignment: AlignmentDirectional.centerStart,
              child: FilledButton(
                onPressed: () => _entering.value = true,
                child: const Text('Pair with dev session'),
              ),
            ),
          if (widget.scan != null)
            Align(
              alignment: AlignmentDirectional.centerStart,
              child: TextButton(
                onPressed: () => unawaited(_scan()),
                child: const Text('Scan QR code'),
              ),
            ),
        ],
      ],
    );
  }
}

final class _SyncTab extends StatelessWidget {
  const _SyncTab(this.d, {required this.syncing, required this.onSync});

  final PluxDiagnostics d;
  final bool syncing;
  final VoidCallback? onSync;

  @override
  Widget build(BuildContext context) => ValueListenableBuilder(
    valueListenable: d.syncStatus,
    builder: (context, event, _) => ListView(
      padding: const EdgeInsets.all(12),
      children: [
        Text('Status: ${describeSyncEvent(event)}'),
        const SizedBox(height: 8),
        Align(
          alignment: AlignmentDirectional.centerStart,
          child: FilledButton(
            onPressed: onSync,
            child: Text(syncing ? 'Syncing…' : 'Sync now'),
          ),
        ),
      ],
    ),
  );
}

final class _ReleaseTab extends StatelessWidget {
  const _ReleaseTab(this.d);

  final PluxDiagnostics d;

  @override
  Widget build(BuildContext context) => ValueListenableBuilder(
    valueListenable: d.release,
    builder: (context, release, _) {
      if (release == null) return const Center(child: Text('No release'));
      return ListView(
        children: [
          ListTile(dense: true, title: Text('Release #${release.sequence}')),
          for (final p in release.plugins)
            ListTile(
              dense: true,
              title: Text(p.key.isEmpty ? 'app bundle' : p.key),
              subtitle: Text(
                'v${p.version} · ${p.bundleHash.substring(0, p.bundleHash.length.clamp(0, 12))}',
              ),
              trailing: p.switchedOff ? const Text('switched off') : null,
            ),
        ],
      );
    },
  );
}

final class _LogTab extends StatelessWidget {
  const _LogTab(this.d);

  final PluxDiagnostics d;

  @override
  Widget build(BuildContext context) => ValueListenableBuilder(
    valueListenable: d.log,
    builder: (context, log, _) {
      if (log.isEmpty) return const Center(child: Text('No problems'));
      return ListView(
        children: [
          for (final entry in log.reversed)
            ListTile(
              dense: true,
              title: Text('${entry.error.code.id} ${entry.error.code.title}'),
              subtitle: Text(entry.error.message),
            ),
        ],
      );
    },
  );
}

/// A one-line description of a sync event for people.
String describeSyncEvent(SyncEvent? e) => switch (e) {
  null => 'not synced yet',
  SyncChecking() => 'checking',
  SyncDownloading(:final received, :final total) =>
    'downloading $received of $total bytes',
  SyncUpToDate() => 'up to date',
  SyncStaged(:final sequence) => 'release $sequence staged',
  SyncActivated(:final sequence) => 'release $sequence active',
  SyncRolledBack(:final from, :final to) =>
    'release $from failed its trial; back to $to',
  SyncFailed(:final error) => 'failed: ${error.code.id} ${error.message}',
};
