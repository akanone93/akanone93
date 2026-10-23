// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What the viewer host remembers while it runs: who is signed in to which
/// server, and the link waiting to be opened. Nothing here is written to
/// disk: the access token lives in memory and is gone when the process ends,
/// so a lost or shared device holds no credential (SEC-092, AGENTS.md §5).
library;

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:plux_host_kit/src/api.dart';
import 'package:plux_host_kit/src/link.dart';

/// A signed-in auditor.
final class ViewerSession {
  /// Creates a session on [server] with [token].
  const ViewerSession(this.server, this.token);

  /// The server the token was issued by, and is sent to.
  final Uri server;

  /// The access token. It is never logged, shown or written.
  final String token;

  /// The client that calls the server as the auditor.
  ViewerApi api({http.Client? client}) =>
      ViewerApi(server, token: token, client: client);

  @override
  String toString() => 'ViewerSession($server)';
}

/// The viewer's state; screens listen to it.
final class ViewerController extends ChangeNotifier {
  ViewerSession? _session;
  ViewerLink? _pending;
  String? _rejected;

  /// The signed-in auditor, or null.
  ViewerSession? get session => _session;

  /// The link waiting for a session on its server, or null.
  ViewerLink? get pendingLink => _pending;

  /// Why the last link was refused, until it is shown; or null.
  String? takeRejection() {
    final message = _rejected;
    _rejected = null;
    return message;
  }

  /// Whether a refusal is waiting to be shown.
  bool get hasRejection => _rejected != null;

  /// Records a sign-in.
  void signedIn(ViewerSession session) {
    _session = session;
    notifyListeners();
  }

  /// Forgets the session; a pending link stays.
  void signOut() {
    _session = null;
    notifyListeners();
  }

  /// Queues [link] to be opened. A link to another server than the
  /// session's ends that session: a token is sent only to the server that
  /// issued it, so the auditor signs in to the server the link names.
  void offerLink(ViewerLink link) {
    _pending = link;
    if (_session != null && _session!.server != link.server) _session = null;
    notifyListeners();
  }

  /// Records that a link was refused, with the reason to show.
  void rejectLink(String reason) {
    _rejected = reason;
    notifyListeners();
  }

  /// The pending link, when the session can open it; it is not offered
  /// again.
  ViewerLink? takeLink() {
    final link = _pending;
    final session = _session;
    if (link == null || session == null || session.server != link.server) {
      return null;
    }
    _pending = null;
    return link;
  }
}
