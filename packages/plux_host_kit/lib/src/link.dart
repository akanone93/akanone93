// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The link that opens a reconstruction in the viewer host (SEC-142,
/// ADR-0082): `plux-viewer://reconstruction/<id>?server=<server URL>`, as the
/// server gives it in a reconstruction's `openUri`.
library;

/// A link to one reconstruction release on one Plux server.
final class ViewerLink {
  /// Creates a link to reconstruction [id] on [server].
  const ViewerLink({required this.id, required this.server});

  /// The URL scheme the viewer registers.
  static const String scheme = 'plux-viewer';

  /// The only host of a viewer link.
  static const String host = 'reconstruction';

  /// Hosts a server may be reached at over plain HTTP: the development
  /// machine, and the Android emulator's alias of it.
  static const Set<String> _developmentHosts = {
    'localhost',
    '127.0.0.1',
    '[::1]',
    '10.0.2.2',
  };

  static final RegExp _id = RegExp(r'^[A-Za-z0-9_-]{1,128}$');

  /// Whether [uri] is addressed to the viewer, valid or not.
  static bool isViewerUri(Uri uri) => uri.scheme.toLowerCase() == scheme;

  /// Reads [uri]; throws [FormatException] naming what is wrong when it is
  /// not a valid reconstruction link. A link is untrusted input: the ID is a
  /// single path segment of safe characters, and the server is an HTTPS
  /// origin, or a development machine, with no credentials, query or
  /// fragment.
  static ViewerLink parse(Uri uri) {
    if (!isViewerUri(uri) || uri.host.toLowerCase() != host) {
      throw const FormatException('not a plux-viewer://reconstruction link');
    }
    final segments = uri.pathSegments;
    if (segments.length != 1 || !_id.hasMatch(segments.single)) {
      throw const FormatException('the link names no valid reconstruction');
    }
    final servers = uri.queryParametersAll['server'];
    if (servers == null || servers.length != 1) {
      throw const FormatException('the link names no server');
    }
    return ViewerLink(id: segments.single, server: parseServer(servers.single));
  }

  /// Reads a server address typed or linked; throws [FormatException] for
  /// one the viewer will not send a credential to.
  static Uri parseServer(String text) {
    final Uri url;
    try {
      url = Uri.parse(text.trim());
    } on FormatException {
      throw const FormatException('the server is not a URL');
    }
    final secure = url.scheme == 'https';
    final development =
        url.scheme == 'http' && _developmentHosts.contains(url.host);
    if (!secure && !development) {
      throw const FormatException('the server must be an https URL');
    }
    if (url.host.isEmpty ||
        url.userInfo.isNotEmpty ||
        url.hasQuery ||
        url.hasFragment) {
      throw const FormatException(
        'the server must be an origin without credentials, query or fragment',
      );
    }
    final path = url.path.endsWith('/') ? url.path : '${url.path}/';
    return url.replace(path: path);
  }

  /// The reconstruction release's identifier.
  final String id;

  /// The Plux server, with a trailing slash.
  final Uri server;

  /// The link as a URI.
  Uri get uri => Uri(
    scheme: scheme,
    host: host,
    pathSegments: [id],
    queryParameters: {'server': server.toString()},
  );

  @override
  bool operator ==(Object other) =>
      other is ViewerLink && other.id == id && other.server == server;

  @override
  int get hashCode => Object.hash(id, server);

  @override
  String toString() => 'ViewerLink($id on $server)';
}
