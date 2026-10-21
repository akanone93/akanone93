// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The viewer's sign-in screen: a server, then either a device code the
/// auditor approves in a browser (the default, as `plux login`) or an access
/// token the auditor created in Studio and pastes (SEC-142).
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:plux_viewer/src/api.dart';
import 'package:plux_viewer/src/controller.dart';
import 'package:plux_viewer/src/device_sign_in.dart';
import 'package:plux_viewer/src/link.dart';

/// Creates the client a sign-in uses; tests replace it.
typedef ApiFactory = ViewerApi Function(Uri server);

/// The client sign-in uses: plain HTTPS to the server.
ViewerApi defaultApiFactory(Uri server) =>
    ViewerApi(server, client: http.Client());

/// Signs the auditor in to a Plux server.
final class SignInScreen extends StatefulWidget {
  /// Creates the screen. [server] is the address to start with, such as the
  /// one a link names.
  const SignInScreen({
    super.key,
    required this.controller,
    this.server,
    this.notice,
    this.apiFactory = defaultApiFactory,
  });

  /// Receives the session.
  final ViewerController controller;

  /// The server to prefill.
  final Uri? server;

  /// A line shown above the form, such as why the auditor is asked again.
  final String? notice;

  /// Creates the client for a server.
  final ApiFactory apiFactory;

  @override
  State<SignInScreen> createState() => _SignInScreenState();
}

final class _SignInScreenState extends State<SignInScreen> {
  late final TextEditingController _server = TextEditingController(
    text: widget.server?.toString() ?? '',
  );
  final TextEditingController _token = TextEditingController();
  DeviceSignIn? _running;
  DeviceGrant? _grant;
  String? _error;
  bool _pasting = false;
  bool _busy = false;

  @override
  void dispose() {
    _running?.cancel();
    _server.dispose();
    _token.dispose();
    super.dispose();
  }

  Uri? _serverOrError() {
    try {
      return ViewerLink.parseServer(_server.text);
    } on FormatException catch (e) {
      setState(() => _error = e.message);
      return null;
    }
  }

  Future<void> _startDeviceFlow() async {
    final server = _serverOrError();
    if (server == null) return;
    final flow = DeviceSignIn(widget.apiFactory(server));
    setState(() {
      _running = flow;
      _error = null;
      _busy = true;
    });
    try {
      final grant = await flow.start();
      if (!mounted) return;
      setState(() => _grant = grant);
      final token = await flow.complete(grant);
      if (!mounted) return;
      widget.controller.signedIn(ViewerSession(server, token));
    } on SignInException catch (e) {
      _fail(e.toString());
    } on ViewerApiException catch (e) {
      _fail(e.toString());
    }
  }

  void _useToken() {
    final server = _serverOrError();
    final token = _token.text.trim();
    if (server == null) return;
    if (token.isEmpty) {
      setState(() => _error = 'Paste an access token.');
      return;
    }
    widget.controller.signedIn(ViewerSession(server, token));
  }

  void _fail(String message) {
    if (!mounted) return;
    setState(() {
      _error = message;
      _grant = null;
      _running = null;
      _busy = false;
    });
  }

  void _cancel() {
    _running?.cancel();
    setState(() {
      _grant = null;
      _running = null;
      _busy = false;
    });
  }

  @override
  Widget build(BuildContext context) {
    final grant = _grant;
    return Scaffold(
      appBar: AppBar(title: const Text('Plux Viewer')),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            if (widget.notice case final notice?)
              Padding(
                padding: const EdgeInsets.only(bottom: 16),
                child: Text(notice),
              ),
            TextField(
              controller: _server,
              enabled: grant == null,
              keyboardType: TextInputType.url,
              autocorrect: false,
              decoration: const InputDecoration(
                labelText: 'Plux server',
                hintText: 'https://plux.example.org',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 16),
            if (grant != null) ...[
              _CodePrompt(grant: grant),
              const SizedBox(height: 16),
              OutlinedButton(onPressed: _cancel, child: const Text('Cancel')),
            ] else if (_pasting) ...[
              TextField(
                controller: _token,
                obscureText: true,
                enableSuggestions: false,
                autocorrect: false,
                decoration: const InputDecoration(
                  labelText: 'Access token',
                  border: OutlineInputBorder(),
                ),
              ),
              const SizedBox(height: 16),
              FilledButton(
                onPressed: _useToken,
                child: const Text('Sign in with the token'),
              ),
              TextButton(
                onPressed: () => setState(() => _pasting = false),
                child: const Text('Use a device code instead'),
              ),
            ] else ...[
              FilledButton(
                onPressed: _busy ? null : _startDeviceFlow,
                child: const Text('Sign in with a device code'),
              ),
              TextButton(
                onPressed: () => setState(() => _pasting = true),
                child: const Text('Paste an access token instead'),
              ),
            ],
            if (_error case final error?)
              Padding(
                padding: const EdgeInsets.only(top: 16),
                child: Semantics(
                  liveRegion: true,
                  child: Text(
                    error,
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.error,
                    ),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

/// The code to approve, and where.
final class _CodePrompt extends StatelessWidget {
  const _CodePrompt({required this.grant});

  final DeviceGrant grant;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('Open this address in a browser and enter the code:'),
        const SizedBox(height: 8),
        SelectableText(grant.verificationUri),
        const SizedBox(height: 16),
        Row(
          children: [
            Expanded(
              child: SelectableText(
                grant.userCode,
                style: theme.textTheme.headlineMedium,
              ),
            ),
            IconButton(
              tooltip: 'Copy the code',
              icon: const Icon(Icons.copy),
              onPressed: () => unawaited(
                Clipboard.setData(ClipboardData(text: grant.userCode)),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        const Row(
          children: [
            SizedBox.square(
              dimension: 16,
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
            SizedBox(width: 12),
            Expanded(child: Text('Waiting for approval')),
          ],
        ),
      ],
    );
  }
}
