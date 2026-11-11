// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The signed-in phone's apps (DEV-001, ADR-0104): the ones open in Studio
/// are marked live; a tap opens the app at its entry screen, joined to the
/// Studio session.
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:plux_dev/src/ui/theme.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// How often the list asks which apps are open in Studio while it shows.
const Duration appsRefresh = Duration(seconds: 15);

/// The list of apps.
final class AppsScreen extends StatefulWidget {
  /// Creates the screen.
  const AppsScreen({
    super.key,
    required this.account,
    required this.load,
    required this.onOpen,
    required this.onScan,
    required this.onSignOut,
  });

  /// Whom the phone is signed in as.
  final DevPhoneAccount account;

  /// Loads the apps.
  final Future<List<DevPhoneApp>> Function() load;

  /// Opens an app; completes once it runs, or failed (and said why).
  final Future<void> Function(DevPhoneApp app) onOpen;

  /// Scans a code: a session's pairing code opens its app at once.
  final Future<void> Function() onScan;

  /// Signs the phone out.
  final Future<void> Function() onSignOut;

  @override
  State<AppsScreen> createState() => _AppsScreenState();
}

final class _AppsScreenState extends State<AppsScreen> {
  List<DevPhoneApp>? _apps;
  String? _error;
  String? _opening;
  Timer? _refresh;
  late final AppLifecycleListener _lifecycle;

  @override
  void initState() {
    super.initState();
    _lifecycle = AppLifecycleListener(onResume: () => unawaited(_reload()));
    _refresh = Timer.periodic(appsRefresh, (_) => unawaited(_reload()));
    unawaited(_reload());
  }

  @override
  void dispose() {
    _refresh?.cancel();
    _lifecycle.dispose();
    super.dispose();
  }

  Future<void> _reload() async {
    try {
      final apps = await widget.load();
      if (mounted) {
        setState(() {
          _apps = apps;
          _error = null;
        });
      }
    } on DevPhoneException catch (e) {
      if (mounted) {
        setState(
          () => _error = e.failure == DevPhoneFailure.unreachable
              ? "Studio's server can't be reached. Pull to try again."
              : e.message,
        );
      }
    }
  }

  Future<void> _open(DevPhoneApp app) async {
    if (_opening != null) return;
    setState(() => _opening = app.id);
    try {
      await widget.onOpen(app);
    } finally {
      if (mounted) setState(() => _opening = null);
    }
  }

  String get _whom => [
    widget.account.userDisplay,
    widget.account.organizationName,
  ].where((s) => s.isNotEmpty).join(' · ');

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final apps = _apps;
    return Scaffold(
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: _reload,
          child: ListView(
            padding: const EdgeInsets.all(DevSizes.page),
            children: [
              Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('Your apps', style: text.headlineMedium),
                        if (_whom.isNotEmpty) ...[
                          const SizedBox(height: 4),
                          Text(_whom, style: text.bodyMedium),
                        ],
                      ],
                    ),
                  ),
                  IconButton(
                    key: const Key('scan'),
                    tooltip: 'Scan a code',
                    iconSize: 26,
                    style: IconButton.styleFrom(
                      minimumSize: const Size.square(DevSizes.touch),
                    ),
                    onPressed: widget.onScan,
                    icon: const Icon(Icons.qr_code_scanner_rounded),
                  ),
                  PopupMenuButton<String>(
                    key: const Key('menu'),
                    tooltip: 'More',
                    icon: const Icon(Icons.more_horiz_rounded),
                    onSelected: (v) {
                      if (v == 'sign-out') unawaited(widget.onSignOut());
                    },
                    itemBuilder: (_) => const [
                      PopupMenuItem(
                        value: 'sign-out',
                        child: Text('Sign this phone out'),
                      ),
                    ],
                  ),
                ],
              ),
              const SizedBox(height: 24),
              if (_error case final error?)
                Padding(
                  padding: const EdgeInsets.only(bottom: 16),
                  child: Text(
                    error,
                    style: text.bodyMedium?.copyWith(color: DevColors.danger),
                  ),
                ),
              if (apps == null && _error == null)
                const Padding(
                  padding: EdgeInsets.all(48),
                  child: Center(child: CircularProgressIndicator()),
                )
              else if (apps != null && apps.isEmpty)
                DevCard(
                  raised: false,
                  child: Text(
                    'This organisation has no apps yet. Create one in Studio.',
                    style: text.bodyLarge,
                  ),
                )
              else
                for (final app in apps ?? const <DevPhoneApp>[]) ...[
                  _AppTile(
                    app: app,
                    opening: _opening == app.id,
                    onTap: () => _open(app),
                  ),
                  const SizedBox(height: 12),
                ],
              const SizedBox(height: 12),
              Text(
                'Open an app in Studio, then tap it here. It opens at its entry screen, live.',
                style: text.bodyMedium,
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// The colour of an app's initial, the same for the same app.
Color appTint(String id) {
  const tints = [
    Color(0xFF5B3DF5),
    Color(0xFF0EA5E9),
    Color(0xFF10B981),
    Color(0xFFF59E0B),
    Color(0xFFEC4899),
    Color(0xFF8B5CF6),
  ];
  return tints[id.codeUnits.fold(0, (a, b) => a + b) % tints.length];
}

final class _AppTile extends StatelessWidget {
  const _AppTile({
    required this.app,
    required this.opening,
    required this.onTap,
  });

  final DevPhoneApp app;
  final bool opening;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final tint = appTint(app.id);
    final initial = app.name.isEmpty
        ? '?'
        : app.name.characters.first.toUpperCase();
    return Semantics(
      button: true,
      label:
          '${app.name}, ${app.live ? 'open in Studio' : 'not open in Studio'}',
      excludeSemantics: true,
      child: DevCard(
        key: Key('app-${app.id}'),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        onTap: opening ? null : onTap,
        child: Row(
          children: [
            DecoratedBox(
              decoration: BoxDecoration(
                color: tint.withValues(alpha: 0.12),
                borderRadius: BorderRadius.circular(16),
              ),
              child: SizedBox.square(
                dimension: 52,
                child: Center(
                  child: Text(
                    initial,
                    style: TextStyle(
                      color: tint,
                      fontSize: 22,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ),
            ),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    app.name,
                    style: text.titleLarge?.copyWith(fontSize: 17),
                  ),
                  const SizedBox(height: 4),
                  Row(
                    children: [
                      DecoratedBox(
                        decoration: BoxDecoration(
                          color: app.live ? DevColors.live : DevColors.line,
                          shape: BoxShape.circle,
                        ),
                        child: const SizedBox.square(dimension: 8),
                      ),
                      const SizedBox(width: 6),
                      Text(
                        app.live ? 'Open in Studio' : 'Not open in Studio',
                        style: text.bodyMedium,
                      ),
                    ],
                  ),
                ],
              ),
            ),
            if (opening)
              const SizedBox.square(
                dimension: 24,
                child: CircularProgressIndicator(strokeWidth: 2.5),
              )
            else
              const Icon(Icons.chevron_right_rounded, color: DevColors.muted),
          ],
        ),
      ),
    );
  }
}
