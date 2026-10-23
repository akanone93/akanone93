// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The auditor's reconstructions (SEC-142): the newest of every app they can
/// see, to open in the runtime's reconstruction mode.
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:plux_viewer/src/api.dart';

/// A reconstruction with the name of its app.
typedef ListedReconstruction = ({AppSummary app, ReconstructionSummary item});

/// Lists the reconstructions of every app of the signed-in auditor.
final class ListScreen extends StatefulWidget {
  /// Creates the screen over [api]. [onOpen] opens one reconstruction;
  /// [onSignOut] ends the session.
  const ListScreen({
    super.key,
    required this.api,
    required this.onOpen,
    required this.onSignOut,
  });

  /// The signed-in client.
  final ViewerApi api;

  /// Called with the reconstruction the auditor chose.
  final void Function(ReconstructionSummary item) onOpen;

  /// Called when the auditor signs out, or when the server refuses the
  /// token.
  final VoidCallback onSignOut;

  @override
  State<ListScreen> createState() => _ListScreenState();
}

final class _ListScreenState extends State<ListScreen> {
  List<ListedReconstruction>? _items;
  Object? _error;
  int _generation = 0;

  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  /// Loads the list; of overlapping loads only the last one is shown.
  Future<void> _load() async {
    final generation = ++_generation;
    setState(() {
      _items = null;
      _error = null;
    });
    try {
      final apps = await widget.api.listApps();
      final lists = await Future.wait([
        for (final app in apps) widget.api.listReconstructions(app.id),
      ]);
      final items = [
        for (var i = 0; i < apps.length; i++)
          for (final item in lists[i]) (app: apps[i], item: item),
      ]..sort((a, b) => b.item.createdAt.compareTo(a.item.createdAt));
      if (mounted && generation == _generation) {
        setState(() => _items = items);
      }
    } on ViewerApiException catch (e) {
      if (!mounted || generation != _generation) return;
      if (e.unauthenticated) widget.onSignOut();
      setState(() => _error = e);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Reconstructions'),
      actions: [
        IconButton(
          tooltip: 'Refresh',
          icon: const Icon(Icons.refresh),
          onPressed: () => unawaited(_load()),
        ),
        IconButton(
          tooltip: 'Sign out',
          icon: const Icon(Icons.logout),
          onPressed: widget.onSignOut,
        ),
      ],
    ),
    body: _body(),
  );

  Widget _body() {
    final items = _items;
    if (_error case final error?) {
      return _Message(
        text: '$error',
        action: ('Try again', () => unawaited(_load())),
      );
    }
    if (items == null) return const Center(child: CircularProgressIndicator());
    if (items.isEmpty) {
      return const _Message(
        text:
            'There are no reconstructions yet. One is requested for a device '
            'and a moment, and appears here once it is signed.',
      );
    }
    return ListView.separated(
      itemCount: items.length,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (context, i) =>
          _Tile(entry: items[i], onOpen: widget.onOpen),
    );
  }
}

/// One reconstruction.
final class _Tile extends StatelessWidget {
  const _Tile({required this.entry, required this.onOpen});

  final ListedReconstruction entry;
  final void Function(ReconstructionSummary item) onOpen;

  /// [at] as `2026-10-20 09:30 UTC`.
  static String when(DateTime at) {
    final t = at.toUtc();
    String two(int n) => n.toString().padLeft(2, '0');
    return '${t.year}-${two(t.month)}-${two(t.day)} '
        '${two(t.hour)}:${two(t.minute)} UTC';
  }

  @override
  Widget build(BuildContext context) {
    final item = entry.item;
    return ListTile(
      enabled: item.openable,
      title: Text(when(item.moment)),
      subtitle: Text(
        '${entry.app.name} · release ${item.releaseSequence} · device '
        '${item.deviceId}${item.openable ? '' : ' · waiting to be signed'}',
      ),
      trailing: const Icon(Icons.chevron_right),
      onTap: item.openable ? () => onOpen(item) : null,
    );
  }
}

/// A line of text, with an optional button.
final class _Message extends StatelessWidget {
  const _Message({required this.text, this.action});

  final String text;
  final (String, VoidCallback)? action;

  @override
  Widget build(BuildContext context) => Center(
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(text, textAlign: TextAlign.center),
          if (action case (final label, final onPressed)) ...[
            const SizedBox(height: 16),
            OutlinedButton(onPressed: onPressed, child: Text(label)),
          ],
        ],
      ),
    ),
  );
}
