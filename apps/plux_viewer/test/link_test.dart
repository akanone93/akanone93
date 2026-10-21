// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_viewer/viewer.dart';

/// The `plux-viewer://` link of a reconstruction [SEC-142].
void main() {
  group('parses what the server gives as openUri [SEC-142]', () {
    test('a reconstruction on an https server', () {
      final link = ViewerLink.parse(
        Uri.parse(
          'plux-viewer://reconstruction/rec_01HZ?server=https%3A%2F%2Fplux.example.org',
        ),
      );
      expect(link.id, 'rec_01HZ');
      expect(link.server, Uri.parse('https://plux.example.org/'));
    });

    test('a server behind a path, kept with a trailing slash', () {
      final link = ViewerLink.parse(
        Uri.parse(
          'plux-viewer://reconstruction/abc?server=https://example.org/plux',
        ),
      );
      expect(link.server, Uri.parse('https://example.org/plux/'));
      expect(
        link.server.resolve('plux.v1.AuditService/GetReconstruction').path,
        '/plux/plux.v1.AuditService/GetReconstruction',
      );
    });

    test('the development machine over http', () {
      for (final host in [
        'localhost:8080',
        '127.0.0.1:8080',
        '10.0.2.2:8080',
      ]) {
        final link = ViewerLink.parse(
          Uri.parse('plux-viewer://reconstruction/abc?server=http://$host'),
        );
        expect(link.server.scheme, 'http', reason: host);
      }
    });

    test('is the inverse of uri', () {
      final link = ViewerLink(
        id: 'rec-1_a',
        server: Uri.parse('https://plux.example.org:8443/'),
      );
      expect(ViewerLink.parse(link.uri), link);
      expect(ViewerLink.parse(Uri.parse(link.uri.toString())), link);
    });

    test('takes the scheme and host in any case', () {
      expect(
        ViewerLink.parse(
          Uri.parse(
            'PLUX-VIEWER://Reconstruction/abc?server=https://a.example',
          ),
        ).id,
        'abc',
      );
    });
  });

  group('refuses what it must not follow [SEC-142]', () {
    final bad = <String, String>{
      'another scheme': 'https://reconstruction/abc?server=https://a.example',
      'another host': 'plux-viewer://settings/abc?server=https://a.example',
      'no id': 'plux-viewer://reconstruction/?server=https://a.example',
      'two segments':
          'plux-viewer://reconstruction/a/b?server=https://a.example',
      'an id that climbs':
          'plux-viewer://reconstruction/..%2F..?server=https://a.example',
      'an id with a dot':
          'plux-viewer://reconstruction/a.b?server=https://a.example',
      'an id that is too long':
          'plux-viewer://reconstruction/${'a' * 129}?server=https://a.example',
      'no server': 'plux-viewer://reconstruction/abc',
      'two servers': 'plux-viewer://reconstruction/abc?server=https://a.example&server=https://b.example',
      'a server over http':
          'plux-viewer://reconstruction/abc?server=http://plux.example.org',
      'a server that is not a URL':
          'plux-viewer://reconstruction/abc?server=plux.example.org',
      'a server with a password':
          'plux-viewer://reconstruction/abc?server=https://u:p@a.example',
      'a server with a user':
          'plux-viewer://reconstruction/abc?server=https://u@a.example',
      'a server with a query':
          'plux-viewer://reconstruction/abc?server=https://a.example/?x=1',
      'a server with a fragment':
          'plux-viewer://reconstruction/abc?server=https://a.example/%23x',
      'a server without a host':
          'plux-viewer://reconstruction/abc?server=https://',
      'a file server':
          'plux-viewer://reconstruction/abc?server=file:///etc/passwd',
      'a javascript server':
          'plux-viewer://reconstruction/abc?server=javascript:alert(1)',
    };
    bad.forEach((name, link) {
      test(name, () {
        expect(() => ViewerLink.parse(Uri.parse(link)), throwsFormatException);
      });
    });
  });

  test('isViewerUri tells the viewer\'s scheme from the rest [SEC-142]', () {
    expect(ViewerLink.isViewerUri(Uri.parse('plux-viewer://x')), isTrue);
    expect(ViewerLink.isViewerUri(Uri.parse('/')), isFalse);
    expect(ViewerLink.isViewerUri(Uri.parse('https://x')), isFalse);
  });

  test('parseServer takes a typed address with spaces [SEC-142]', () {
    expect(
      ViewerLink.parseServer('  https://plux.example.org '),
      Uri.parse('https://plux.example.org/'),
    );
    expect(() => ViewerLink.parseServer(''), throwsFormatException);
  });
}
