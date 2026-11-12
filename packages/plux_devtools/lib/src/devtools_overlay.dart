// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The debug overlay: a badge over the host app that opens a dark panel with
/// the sync status, the active release, the problems the runtime reported
/// and, with a dev session, the session.
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
    this.showBadge = true,
    this.initiallyOpen = false,
    this.onClose,
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

  /// Whether the closed overlay shows its badge; a host with a button of its
  /// own (the Dev app's status pill) turns it off and opens the panel itself.
  final bool showBadge;

  /// Whether the panel is open when the overlay first shows.
  final bool initiallyOpen;

  /// Runs when the panel closes; the host that opened it hides the overlay.
  final VoidCallback? onClose;

  @override
  State<PluxDevtools> createState() => _PluxDevtoolsState();
}

enum _Tab { sync, release, log, session }

final class _PluxDevtoolsState extends State<PluxDevtools> {
  late bool _open = widget.initiallyOpen;
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
  void didUpdateWidget(PluxDevtools old) {
    super.didUpdateWidget(old);
    // Shown again: the panel opens as it did the first time.
    if (!old.enabled && widget.enabled) _open = widget.initiallyOpen;
  }

  @override
  Widget build(BuildContext context) {
    if (kReleaseMode) return widget.child;
    final d = _diagnostics;
    // The host stays the first child whether the overlay shows or not, so
    // showing it never rebuilds the app beneath.
    return Stack(
      textDirection: TextDirection.ltr,
      children: [
        widget.child,
        if (!widget.enabled)
          const SizedBox.shrink()
        else if (_open)
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
                  onClose: () {
                    setState(() => _open = false);
                    widget.onClose?.call();
                  },
                ),
              ),
            ),
          )
        else if (widget.showBadge)
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

/// The colours of the debug panel: dark and a little see-through, so it
/// reads as a tool over the app and not as part of it.
abstract final class _Ink {
  static const Color sheet = Color(0xF20E1016);
  static const Color raised = Color(0x14FFFFFF);
  static const Color line = Color(0x1FFFFFFF);
  static const Color text = Color(0xFFE8EAF2);
  static const Color muted = Color(0xFF9097AD);
  static const Color accent = Color(0xFF8B7CFF);
  static const Color good = Color(0xFF3DD68C);
  static const Color bad = Color(0xFFFF6B6B);
  static const String mono = 'monospace';
}

/// The debug panel: a dark sheet along the bottom with a header, a row of
/// tabs (sync, release, problems and, with a session, the session) and the
/// open tab, which scrolls on its own.
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

  static const Map<_Tab, String> _labels = {
    _Tab.sync: 'Sync',
    _Tab.release: 'Release',
    _Tab.log: 'Problems',
    _Tab.session: 'Session',
  };

  @override
  Widget build(BuildContext context) {
    final d = diagnostics;
    final height = (MediaQuery.sizeOf(context).height * 0.5).clamp(
      240.0,
      440.0,
    );
    return Theme(
      data: ThemeData(
        useMaterial3: true,
        brightness: Brightness.dark,
        colorScheme: const ColorScheme.dark(
          primary: _Ink.accent,
          surface: _Ink.sheet,
          onSurface: _Ink.text,
        ),
        textTheme: Typography.whiteMountainView.apply(
          bodyColor: _Ink.text,
          displayColor: _Ink.text,
        ),
      ),
      child: Material(
        color: _Ink.sheet,
        elevation: 12,
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
        ),
        clipBehavior: Clip.antiAlias,
        child: SizedBox(
          height: height,
          child: Column(
            children: [
              _Header(diagnostics: d, onClose: onClose),
              SizedBox(
                height: 40,
                child: ListView(
                  scrollDirection: Axis.horizontal,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  children: [
                    for (final t in _Tab.values)
                      if (t != _Tab.session || session != null)
                        _TabChip(
                          label: _labels[t]!,
                          count: t == _Tab.log ? d?.log : null,
                          selected: t == tab,
                          onTap: () => onTab(t),
                        ),
                  ],
                ),
              ),
              const Divider(height: 1, color: _Ink.line),
              Expanded(
                child: DefaultTextStyle.merge(
                  style: const TextStyle(color: _Ink.text, fontSize: 13),
                  child: tab == _Tab.session && session != null
                      ? _SessionTab(session!, scan: scan)
                      : d == null
                      ? const _Empty('Plux is not initialized')
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
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// The panel's header: a grab handle, its name, the active release and the
/// close button.
final class _Header extends StatelessWidget {
  const _Header({required this.diagnostics, required this.onClose});

  final PluxDiagnostics? diagnostics;
  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    final d = diagnostics;
    return Padding(
      padding: const EdgeInsetsDirectional.fromSTEB(16, 6, 4, 2),
      child: Column(
        children: [
          Container(
            width: 36,
            height: 4,
            decoration: BoxDecoration(
              color: _Ink.line,
              borderRadius: BorderRadius.circular(2),
            ),
          ),
          Row(
            children: [
              const Icon(
                Icons.bug_report_outlined,
                size: 18,
                color: _Ink.accent,
              ),
              const SizedBox(width: 8),
              const Text(
                'Plux debug',
                style: TextStyle(
                  color: _Ink.text,
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(width: 8),
              if (d != null)
                ValueListenableBuilder(
                  valueListenable: d.release,
                  builder: (context, release, _) => _Tag(
                    release == null
                        ? 'no release'
                        : 'release #${release.sequence}',
                  ),
                ),
              const Spacer(),
              // No tooltip: the panel sits above the app's navigator, where
              // there is no overlay to show one in.
              Semantics(
                label: 'Close',
                button: true,
                excludeSemantics: true,
                child: IconButton(
                  onPressed: onClose,
                  icon: const Icon(Icons.close_rounded, color: _Ink.muted),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

/// A tab of the panel, with a count when it has one.
final class _TabChip extends StatelessWidget {
  const _TabChip({
    required this.label,
    required this.selected,
    required this.onTap,
    this.count,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;
  final ValueListenable<List<PluxDiagnostic>>? count;

  @override
  Widget build(BuildContext context) {
    final c = count;
    final text = Text(
      label,
      style: TextStyle(
        color: selected ? _Ink.text : _Ink.muted,
        fontSize: 13,
        fontWeight: selected ? FontWeight.w600 : FontWeight.w500,
      ),
    );
    return Padding(
      padding: const EdgeInsetsDirectional.only(end: 4),
      child: Semantics(
        selected: selected,
        child: Material(
          color: selected ? _Ink.raised : Colors.transparent,
          shape: const StadiumBorder(),
          child: InkWell(
            customBorder: const StadiumBorder(),
            onTap: onTap,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  text,
                  if (c != null)
                    ValueListenableBuilder(
                      valueListenable: c,
                      builder: (context, log, _) => log.isEmpty
                          ? const SizedBox.shrink()
                          : Padding(
                              padding: const EdgeInsetsDirectional.only(
                                start: 6,
                              ),
                              child: _Tag('${log.length}', colour: _Ink.bad),
                            ),
                    ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// A small rounded label.
final class _Tag extends StatelessWidget {
  const _Tag(this.text, {this.colour = _Ink.muted});

  final String text;
  final Color colour;

  @override
  Widget build(BuildContext context) => DecoratedBox(
    decoration: BoxDecoration(
      color: colour.withValues(alpha: 0.16),
      borderRadius: BorderRadius.circular(6),
    ),
    child: Padding(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      child: Text(
        text,
        style: TextStyle(
          color: colour,
          fontSize: 11,
          fontWeight: FontWeight.w600,
          fontFamily: _Ink.mono,
        ),
      ),
    ),
  );
}

/// What an empty tab says, in the middle of it.
final class _Empty extends StatelessWidget {
  const _Empty(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Center(
    child: Text(text, style: const TextStyle(color: _Ink.muted)),
  );
}

/// A row of a tab: a label in the margin and its value in monospace.
final class _Row extends StatelessWidget {
  const _Row(this.label, this.value, {this.colour = _Ink.text});

  final String label;
  final String value;
  final Color colour;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 6),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 88,
          child: Text(
            label,
            style: const TextStyle(color: _Ink.muted, fontSize: 12),
          ),
        ),
        Expanded(
          child: Text(
            value,
            style: TextStyle(
              color: colour,
              fontFamily: _Ink.mono,
              fontSize: 12.5,
            ),
          ),
        ),
      ],
    ),
  );
}

/// The session tab: the state of the dev session, a paste field and, when
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
    widget.session.link,
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
    final link = widget.session.link.value;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 16),
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
        }, style: const TextStyle(fontFamily: _Ink.mono, fontSize: 12.5)),
        if (state is PluxDevPaired && link != null)
          _Row(
            'Link',
            link.name,
            colour: link == DevLinkState.live ? _Ink.good : _Ink.muted,
          ),
        const SizedBox(height: 12),
        if (state is PluxDevPaired)
          Align(
            alignment: AlignmentDirectional.centerStart,
            child: OutlinedButton(
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
              child: OutlinedButton(
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
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
      children: [
        _Row(
          'Status',
          describeSyncEvent(event),
          colour: event is SyncFailed ? _Ink.bad : _Ink.text,
        ),
        ValueListenableBuilder(
          valueListenable: d.release,
          builder: (context, release, _) =>
              _Row('Active', release == null ? 'none' : '#${release.sequence}'),
        ),
        const SizedBox(height: 8),
        Align(
          alignment: AlignmentDirectional.centerStart,
          child: OutlinedButton.icon(
            onPressed: onSync,
            icon: const Icon(Icons.sync_rounded, size: 18),
            label: Text(syncing ? 'Syncing…' : 'Sync now'),
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
      if (release == null) return const _Empty('No release');
      return ListView(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
        children: [
          Text(
            'Release #${release.sequence}',
            style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
          ),
          const SizedBox(height: 4),
          for (final p in release.plugins)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(p.key.isEmpty ? 'app bundle' : p.key),
                        Text(
                          'v${p.version} · ${p.bundleHash.substring(0, p.bundleHash.length.clamp(0, 12))}',
                          style: const TextStyle(
                            color: _Ink.muted,
                            fontFamily: _Ink.mono,
                            fontSize: 12,
                          ),
                        ),
                      ],
                    ),
                  ),
                  if (p.switchedOff)
                    const _Tag('switched off', colour: _Ink.bad),
                ],
              ),
            ),
        ],
      );
    },
  );
}

final class _LogTab extends StatelessWidget {
  const _LogTab(this.d);

  final PluxDiagnostics d;

  static String _time(DateTime t) {
    String two(int n) => n.toString().padLeft(2, '0');
    return '${two(t.hour)}:${two(t.minute)}:${two(t.second)}';
  }

  @override
  Widget build(BuildContext context) => ValueListenableBuilder(
    valueListenable: d.log,
    builder: (context, log, _) {
      if (log.isEmpty) return const _Empty('No problems');
      final entries = log.reversed.toList();
      return ListView.separated(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
        itemCount: entries.length,
        separatorBuilder: (_, _) => const Divider(height: 1, color: _Ink.line),
        itemBuilder: (context, i) {
          final entry = entries[i];
          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    _Tag(entry.error.code.id, colour: _Ink.bad),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        entry.error.code.title,
                        style: const TextStyle(fontWeight: FontWeight.w600),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    Text(
                      _time(entry.at),
                      style: const TextStyle(
                        color: _Ink.muted,
                        fontFamily: _Ink.mono,
                        fontSize: 11,
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 4),
                Text(
                  entry.error.message,
                  style: const TextStyle(
                    color: _Ink.muted,
                    fontFamily: _Ink.mono,
                    fontSize: 12,
                  ),
                ),
              ],
            ),
          );
        },
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
