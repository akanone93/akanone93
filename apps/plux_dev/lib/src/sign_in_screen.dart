// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Dev app's sign-in: a server, then a device code the developer
/// approves in a browser (RFC 8628), as `plux login` does (CLI-002).
library;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:plux_host_kit/plux_host_kit.dart';

/// The client name the Dev app gives the server when it asks for a token.
const String devClientName = 'plux-dev';

/// The permissions the Dev app's token asks for: the apps of the
/// organisation, to choose the one to preview.
const List<String> devScopes = ['app.read'];

/// Creates the client a sign-in uses; tests replace it.
typedef DevApiFactory = ViewerApi Function(Uri server);

/// The client sign-in uses: plain HTTPS to the server, as the Dev app.
ViewerApi defaultDevApi(Uri server) => ViewerApi(
  server,
  client: http.Client(),
  clientName: devClientName,
  scopes: devScopes,
);

/// Signs the developer in to a Plux server.
final class DevSignInScreen extends StatefulWidget {
  /// Creates the screen; [onSignedIn] gets the session.
  const DevSignInScreen({
    super.key,
    required this.onSignedIn,
    this.apiFactory = defaultDevApi,
  });

  /// Receives the session.
  final ValueChanged<ViewerSession> onSignedIn;

  /// Creates the client for a server.
  final DevApiFactory apiFactory;

  @override
  State<DevSignInScreen> createState() => _DevSignInScreenState();
}

final class _DevSignInScreenState extends State<DevSignInScreen> {
  final TextEditingController _server = TextEditingController();
  DeviceSignIn? _running;
  DeviceGrant? _grant;
  String? _error;

  @override
  void dispose() {
    _running?.cancel();
    _server.dispose();
    super.dispose();
  }

  Future<void> _start() async {
    final Uri server;
    try {
      server = ViewerLink.parseServer(_server.text);
    } on FormatException catch (e) {
      setState(() => _error = e.message);
      return;
    }
    final flow = DeviceSignIn(widget.apiFactory(server));
    setState(() {
      _running = flow;
      _error = null;
    });
    try {
      final grant = await flow.start();
      if (!mounted) return;
      setState(() => _grant = grant);
      final token = await flow.complete(grant);
      if (mounted) widget.onSignedIn(ViewerSession(server, token));
    } on SignInException catch (e) {
      _fail(e.toString());
    } on ViewerApiException catch (e) {
      _fail(e.toString());
    }
  }

  void _fail(String message) {
    if (!mounted) return;
    setState(() {
      _error = message;
      _grant = null;
      _running = null;
    });
  }

  void _cancel() {
    _running?.cancel();
    setState(() {
      _grant = null;
      _running = null;
    });
  }

  @override
  Widget build(BuildContext context) {
    final grant = _grant;
    return Scaffold(
      appBar: AppBar(title: const Text('Plux Dev')),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            TextField(
              key: const Key('server-field'),
              controller: _server,
              enabled: _running == null,
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
              const Text('Open this address in a browser and enter the code:'),
              const SizedBox(height: 8),
              SelectableText(grant.verificationUri),
              const SizedBox(height: 16),
              Row(
                children: [
                  Expanded(
                    child: SelectableText(
                      grant.userCode,
                      style: Theme.of(context).textTheme.headlineMedium,
                    ),
                  ),
                  IconButton(
                    tooltip: 'Copy the code',
                    icon: const Icon(Icons.copy),
                    onPressed: () =>
                        Clipboard.setData(ClipboardData(text: grant.userCode)),
                  ),
                ],
              ),
              const SizedBox(height: 16),
              OutlinedButton(onPressed: _cancel, child: const Text('Cancel')),
            ] else
              FilledButton(
                key: const Key('sign-in'),
                onPressed: _running == null ? _start : null,
                child: const Text('Sign in with a device code'),
              ),
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
