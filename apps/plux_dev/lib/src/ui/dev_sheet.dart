// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The dev sheet of a running app (DEV-001, DEV-020, RT-060): everything a
/// developer needs on one sheet — whether the phone is live with Studio and
/// the actions on it, then the app's diagnostics in tabs: the connection,
/// sync, the active release and the problems the runtime reported.
library;

import 'dart:async';

import 'package:flutter/foundation.dart' show ValueListenable;
import 'package:flutter/material.dart';
import 'package:plux_dev/src/ui/status_pill.dart';
import 'package:plux_dev/src/ui/theme.dart';
import 'package:plux_devtools/plux_devtools.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// The tabs of the sheet.
enum DevSheetTab {
  /// The link with Studio: its state, the session and the device.
  connection,

  /// The last sync and a sync now.
  sync,

  /// The active release and its bundles.
  release,

  /// The problems the runtime reported.
  problems,
}

/// The sheet's content; [scroll] is the sheet's own controller, so the whole
/// sheet scrolls as one.
final class DevSheet extends StatefulWidget {
  /// Creates the sheet.
  const DevSheet({
    super.key,
    required this.appName,
    required this.session,
    required this.diagnostics,
    required this.notice,
    required this.onReconnect,
    required this.onSync,
    required this.onLeave,
    this.scroll,
    this.initialTab = DevSheetTab.connection,
  });

  /// The app's name.
  final String appName;

  /// The pairing with the session.
  final PluxDevSession session;

  /// The runtime's diagnostics; null before the runtime runs.
  final PluxDiagnostics? diagnostics;

  /// What the phone is doing about its link, said to the developer (joining
  /// again, waiting for Studio); null when there is nothing to say.
  final ValueListenable<String?> notice;

  /// Connects again: at once when the link is retrying, by joining Studio
  /// again when the session ended.
  final VoidCallback onReconnect;

  /// Runs a sync.
  final Future<Object?> Function() onSync;

  /// Leaves the app.
  final VoidCallback onLeave;

  /// The sheet's scroll controller.
  final ScrollController? scroll;

  /// The tab open at first.
  final DevSheetTab initialTab;

  @override
  State<DevSheet> createState() => _DevSheetState();
}

final class _DevSheetState extends State<DevSheet> {
  late DevSheetTab _tab = widget.initialTab;
  bool _syncing = false;

  Future<void> _sync() async {
    setState(() => _syncing = true);
    try {
      await widget.onSync();
    } on Object {
      // The sync tab shows the failure the runtime reported.
    } finally {
      if (mounted) setState(() => _syncing = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final d = widget.diagnostics;
    return CustomScrollView(
      controller: widget.scroll,
      slivers: [
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(
            DevSizes.page,
            0,
            DevSizes.page,
            12,
          ),
          sliver: SliverToBoxAdapter(
            child: _Header(
              appName: widget.appName,
              session: widget.session,
              notice: widget.notice,
              onReconnect: widget.onReconnect,
              onLeave: widget.onLeave,
            ),
          ),
        ),
        SliverPersistentHeader(
          pinned: true,
          delegate: _TabsHeader(
            tab: _tab,
            problems: d?.log,
            onTab: (t) => setState(() => _tab = t),
          ),
        ),
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(
            DevSizes.page,
            12,
            DevSizes.page,
            24,
          ),
          sliver: SliverToBoxAdapter(
            child: switch (_tab) {
              DevSheetTab.connection => _ConnectionTab(session: widget.session),
              _ when d == null => const _Empty(
                icon: Icons.hourglass_empty_rounded,
                text: 'The app is not running yet.',
              ),
              DevSheetTab.sync => _SyncTab(
                diagnostics: d,
                syncing: _syncing,
                onSync: _syncing ? null : () => unawaited(_sync()),
              ),
              DevSheetTab.release => _ReleaseTab(diagnostics: d),
              DevSheetTab.problems => _ProblemsTab(diagnostics: d),
            },
          ),
        ),
      ],
    );
  }
}

/// The app's name with its link state, what the phone is doing about it,
/// and the two actions.
final class _Header extends StatelessWidget {
  const _Header({
    required this.appName,
    required this.session,
    required this.notice,
    required this.onReconnect,
    required this.onLeave,
  });

  final String appName;
  final PluxDevSession session;
  final ValueListenable<String?> notice;
  final VoidCallback onReconnect;
  final VoidCallback onLeave;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return ValueListenableBuilder<DevLinkState?>(
      valueListenable: session.link,
      builder: (context, link, _) {
        final ended = link == null || link == DevLinkState.ended;
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    appName,
                    style: text.titleLarge,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                const SizedBox(width: 8),
                StatusPill(state: link),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              ended
                  ? 'Not connected to Studio. Edits reach the phone again once it joins.'
                  : 'Edits in Studio appear here as you make them; logs, state and network calls stream to its console.',
              style: text.bodySmall,
            ),
            ValueListenableBuilder<String?>(
              valueListenable: notice,
              builder: (context, said, _) => said == null
                  ? const SizedBox.shrink()
                  : Padding(
                      padding: const EdgeInsets.only(top: 10),
                      child: _Notice(text: said),
                    ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: FilledButton.tonalIcon(
                    key: const Key('reconnect'),
                    onPressed: onReconnect,
                    icon: Icon(
                      ended ? Icons.link_rounded : Icons.refresh_rounded,
                      size: 20,
                    ),
                    label: Text(ended ? 'Join again' : 'Reconnect'),
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: OutlinedButton.icon(
                    key: const Key('leave'),
                    style: OutlinedButton.styleFrom(
                      foregroundColor: DevColors.danger,
                    ),
                    onPressed: onLeave,
                    icon: const Icon(Icons.logout_rounded, size: 20),
                    label: const Text('Leave'),
                  ),
                ),
              ],
            ),
          ],
        );
      },
    );
  }
}

/// The tabs, pinned at the top of the sheet while it scrolls.
final class _TabsHeader extends SliverPersistentHeaderDelegate {
  _TabsHeader({required this.tab, required this.problems, required this.onTab});

  final DevSheetTab tab;
  final ValueListenable<List<PluxDiagnostic>>? problems;
  final ValueChanged<DevSheetTab> onTab;

  static const double _height = 52;

  @override
  double get minExtent => _height;

  @override
  double get maxExtent => _height;

  @override
  bool shouldRebuild(_TabsHeader old) =>
      old.tab != tab || old.problems != problems;

  @override
  Widget build(BuildContext context, double shrinkOffset, bool overlaps) {
    const labels = {
      DevSheetTab.connection: 'Connection',
      DevSheetTab.sync: 'Sync',
      DevSheetTab.release: 'Release',
      DevSheetTab.problems: 'Problems',
    };
    return Material(
      color:
          Theme.of(context).bottomSheetTheme.backgroundColor ??
          DevColors.surface,
      elevation: overlaps ? 1 : 0,
      shadowColor: DevColors.line,
      child: ListView(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(
          horizontal: DevSizes.page,
          vertical: 8,
        ),
        children: [
          for (final t in DevSheetTab.values)
            Padding(
              padding: const EdgeInsetsDirectional.only(end: 8),
              child: _TabChip(
                key: Key('tab-${t.name}'),
                label: labels[t]!,
                selected: t == tab,
                count: t == DevSheetTab.problems ? problems : null,
                onTap: () => onTab(t),
              ),
            ),
        ],
      ),
    );
  }
}

final class _TabChip extends StatelessWidget {
  const _TabChip({
    super.key,
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
    return Semantics(
      selected: selected,
      button: true,
      child: Material(
        color: selected ? DevColors.ink : DevColors.canvas,
        shape: const StadiumBorder(),
        child: InkWell(
          customBorder: const StadiumBorder(),
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  label,
                  style: TextStyle(
                    color: selected ? Colors.white : DevColors.ink,
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                  ),
                ),
                if (c != null)
                  ValueListenableBuilder(
                    valueListenable: c,
                    builder: (context, log, _) => log.isEmpty
                        ? const SizedBox.shrink()
                        : Padding(
                            padding: const EdgeInsetsDirectional.only(start: 6),
                            child: _Count(log.length),
                          ),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

final class _Count extends StatelessWidget {
  const _Count(this.n);

  final int n;

  @override
  Widget build(BuildContext context) => DecoratedBox(
    decoration: const BoxDecoration(
      color: DevColors.danger,
      shape: BoxShape.circle,
    ),
    child: SizedBox.square(
      dimension: 18,
      child: Center(
        child: Text(
          n > 99 ? '99' : '$n',
          style: const TextStyle(
            color: Colors.white,
            fontSize: 10,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    ),
  );
}

/// A label and its value in a card's row; the value in monospace when it is
/// an identifier.
final class _Row extends StatelessWidget {
  const _Row(this.label, this.value, {this.mono = false, this.colour});

  final String label;
  final String value;
  final bool mono;
  final Color? colour;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 7),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 96,
          child: Text(label, style: Theme.of(context).textTheme.bodySmall),
        ),
        Expanded(
          child: SelectableText(
            value,
            style: TextStyle(
              color: colour ?? DevColors.ink,
              fontSize: mono ? 12.5 : 14,
              fontFamily: mono ? 'monospace' : null,
            ),
          ),
        ),
      ],
    ),
  );
}

final class _Empty extends StatelessWidget {
  const _Empty({required this.icon, required this.text});

  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 28),
    child: Column(
      children: [
        Icon(icon, color: DevColors.muted),
        const SizedBox(height: 8),
        Text(
          text,
          style: Theme.of(context).textTheme.bodyMedium,
          textAlign: TextAlign.center,
        ),
      ],
    ),
  );
}

final class _Notice extends StatelessWidget {
  const _Notice({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) => Semantics(
    liveRegion: true,
    child: DecoratedBox(
      decoration: BoxDecoration(
        color: DevColors.warn.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(DevSizes.button),
      ),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Icon(
              Icons.info_outline_rounded,
              size: 18,
              color: Color(0xFF9A5B00),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                text,
                style: const TextStyle(color: Color(0xFF7A4800), height: 1.35),
              ),
            ),
          ],
        ),
      ),
    ),
  );
}

final class _ConnectionTab extends StatelessWidget {
  const _ConnectionTab({required this.session});

  final PluxDevSession session;

  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: Listenable.merge([session.state, session.link]),
    builder: (context, _) {
      final (label, colour) = statusOf(session.link.value);
      final state = session.state.value;
      return DevCard(
        raised: false,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        child: Column(
          children: [
            _Row('Link', label, colour: colour),
            if (state case PluxDevPaired(
              :final sessionId,
              :final deviceId,
            )) ...[
              _Row('Session', sessionId, mono: true),
              _Row('Device', deviceId, mono: true),
            ],
            if (state case PluxDevError(:final code))
              _Row('Pairing', 'failed: $code', colour: DevColors.danger),
            if (session.deviceName.isNotEmpty)
              _Row('Phone', session.deviceName),
          ],
        ),
      );
    },
  );
}

final class _SyncTab extends StatelessWidget {
  const _SyncTab({
    required this.diagnostics,
    required this.syncing,
    required this.onSync,
  });

  final PluxDiagnostics diagnostics;
  final bool syncing;
  final VoidCallback? onSync;

  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: Listenable.merge([diagnostics.syncStatus, diagnostics.release]),
    builder: (context, _) {
      final event = diagnostics.syncStatus.value;
      final release = diagnostics.release.value;
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          DevCard(
            raised: false,
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Column(
              children: [
                _Row(
                  'Status',
                  describeSyncEvent(event),
                  colour: event is SyncFailed ? DevColors.danger : null,
                ),
                _Row(
                  'Release',
                  release == null ? 'none yet' : '#${release.sequence}',
                ),
              ],
            ),
          ),
          const SizedBox(height: 12),
          OutlinedButton.icon(
            key: const Key('sync-now'),
            onPressed: onSync,
            icon: syncing
                ? const SizedBox.square(
                    dimension: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.sync_rounded, size: 20),
            label: Text(syncing ? 'Syncing…' : 'Sync now'),
          ),
        ],
      );
    },
  );
}

final class _ReleaseTab extends StatelessWidget {
  const _ReleaseTab({required this.diagnostics});

  final PluxDiagnostics diagnostics;

  @override
  Widget build(BuildContext context) => ValueListenableBuilder(
    valueListenable: diagnostics.release,
    builder: (context, release, _) {
      if (release == null) {
        return const _Empty(
          icon: Icons.inventory_2_outlined,
          text: 'No release yet.',
        );
      }
      return DevCard(
        raised: false,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Text(
                'Release #${release.sequence}',
                style: Theme.of(context).textTheme.titleMedium,
              ),
            ),
            for (final p in release.plugins)
              _Row(
                p.key.isEmpty ? 'app bundle' : p.key,
                'v${p.version} · ${p.bundleHash.substring(0, p.bundleHash.length.clamp(0, 12))}'
                '${p.switchedOff ? ' · switched off' : ''}',
                mono: true,
                colour: p.switchedOff ? DevColors.danger : null,
              ),
          ],
        ),
      );
    },
  );
}

final class _ProblemsTab extends StatelessWidget {
  const _ProblemsTab({required this.diagnostics});

  final PluxDiagnostics diagnostics;

  static String _time(DateTime t) {
    String two(int n) => n.toString().padLeft(2, '0');
    return '${two(t.hour)}:${two(t.minute)}:${two(t.second)}';
  }

  @override
  Widget build(BuildContext context) => ValueListenableBuilder(
    valueListenable: diagnostics.log,
    builder: (context, log, _) {
      if (log.isEmpty) {
        return const _Empty(
          icon: Icons.check_circle_outline_rounded,
          text: 'No problems.',
        );
      }
      final text = Theme.of(context).textTheme;
      return Column(
        children: [
          for (final entry in log.reversed)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: DevCard(
                raised: false,
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        DecoratedBox(
                          decoration: BoxDecoration(
                            color: DevColors.danger.withValues(alpha: 0.1),
                            borderRadius: BorderRadius.circular(6),
                          ),
                          child: Padding(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 6,
                              vertical: 2,
                            ),
                            child: Text(
                              entry.error.code.id,
                              style: const TextStyle(
                                color: DevColors.danger,
                                fontFamily: 'monospace',
                                fontSize: 11.5,
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                          ),
                        ),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Text(
                            entry.error.code.title,
                            style: text.titleSmall,
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        Text(_time(entry.at), style: text.bodySmall),
                      ],
                    ),
                    const SizedBox(height: 6),
                    SelectableText(
                      entry.error.message,
                      style: const TextStyle(
                        fontFamily: 'monospace',
                        fontSize: 12.5,
                        color: DevColors.ink,
                      ),
                    ),
                  ],
                ),
              ),
            ),
        ],
      );
    },
  );
}
