// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The HTTP and WebSocket clients of the Dev app (DEV-001): every request it
/// makes, its sign-in, the runtime's sync and the dev channel, trusts the
/// system's certificate authorities and, when the build names one, the
/// development certificate authority of the installation it previews.
///
/// A development server such as `make dev`'s presents a certificate of its
/// own authority. Installing that authority on the device does not help:
/// `dart:io`, which every Flutter request and WebSocket goes through here,
/// trusts only the system's authorities on Android and never the ones a user
/// installs. The authority is therefore given to the build, as the root keys
/// are, and only that build trusts it. Without it the Dev app trusts the
/// system's authorities alone.
library;

import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;
import 'package:http/io_client.dart';

/// The `--dart-define` that carries the authority: the PEM text of the
/// installation's development certificate authority (`make dev`:
/// `deploy/compose/.secrets/ca/ca.crt`).
const String devCertificateAuthorityDefine = 'PLUX_DEV_CA';

const String _authority = String.fromEnvironment(devCertificateAuthorityDefine);

/// The security context of [pem]: the system's authorities, and [pem]'s
/// when it is not blank. Throws [FormatException] naming
/// [devCertificateAuthorityDefine] when [pem] holds no certificate.
SecurityContext devSecurityContext(String pem) {
  final context = SecurityContext(withTrustedRoots: true);
  if (pem.trim().isEmpty) return context;
  try {
    context.setTrustedCertificatesBytes(utf8.encode(pem));
  } on TlsException {
    throw const FormatException(
      '$devCertificateAuthorityDefine is not a PEM certificate: pass the '
      "text of the development certificate authority's ca.crt",
    );
  }
  return context;
}

/// Checks the authority the build carries, so that a build with a mistake in
/// it fails at start instead of at its first request.
void checkDevCertificateAuthority() => devSecurityContext(_authority);

/// The `dart:io` client of the Dev app: WebSockets connect with it. A
/// top-level function, since the runtime calls it on other isolates.
HttpClient devIoClient() => HttpClient(context: devSecurityContext(_authority));

/// The HTTP client of the Dev app, over [devIoClient]. A top-level function,
/// since the runtime calls it on other isolates.
http.Client devHttpClient() => IOClient(devIoClient());
