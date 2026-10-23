// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The screen that shows one reconstruction (SEC-142): it opens the release
/// through a [ReconstructionOpener], shows the runtime's
/// `PluxReconstructionView` once it is ready, and stops the runtime, and so
/// removes the reconstruction from the device, when the auditor leaves.
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_host_kit/plux_host_kit.dart';

/// Shows reconstruction [id] through [opener].
final class ReconstructionScreen extends StatefulWidget {
  /// Creates the screen.
  const ReconstructionScreen({
    super.key,
    required this.id,
    required this.opener,
  });

  /// The reconstruction's identifier.
  final String id;

  /// Opens it.
  final ReconstructionOpener opener;

  @override
  State<ReconstructionScreen> createState() => _ReconstructionScreenState();
}

final class _ReconstructionScreenState extends State<ReconstructionScreen> {
  String _step = 'Opening';
  String? _error;
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    unawaited(_open());
  }

  @override
  void dispose() {
    unawaited(widget.opener.close());
    super.dispose();
  }

  Future<void> _open() async {
    setState(() {
      _error = null;
      _ready = false;
    });
    try {
      await widget.opener.open(
        widget.id,
        progress: (step) {
          if (mounted) setState(() => _step = step);
        },
        cancelled: () => !mounted,
      );
      if (mounted) setState(() => _ready = true);
    } on OpenException catch (e) {
      if (mounted) setState(() => _error = e.message);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Reconstruction')),
    body: _ready
        ? const PluxReconstructionView()
        : Center(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: _error == null
                  ? Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        const CircularProgressIndicator(),
                        const SizedBox(height: 16),
                        Text(_step),
                      ],
                    )
                  : Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text(_error!, textAlign: TextAlign.center),
                        const SizedBox(height: 16),
                        OutlinedButton(
                          onPressed: () => unawaited(_open()),
                          child: const Text('Try again'),
                        ),
                      ],
                    ),
            ),
          ),
  );
}
