// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/data/transport.dart';

void main() {
  test('a failed TLS handshake keeps what TLS reported', () {
    expect(
      handshakeFailure(
        const HandshakeException(
          'Handshake error in client',
          OSError(
            'CERTIFICATE_VERIFY_FAILED: unable to get local issuer certificate',
            1,
          ),
        ),
      ),
      'the TLS handshake failed: Handshake error in client: '
      'CERTIFICATE_VERIFY_FAILED: unable to get local issuer certificate',
    );
    expect(
      handshakeFailure(
        const HandshakeException('Connection terminated during handshake'),
      ),
      'the TLS handshake failed: Connection terminated during handshake',
    );
    expect(
      handshakeFailure(const HandshakeException()),
      'the TLS handshake failed',
    );
  });
}
