// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Reading the root keys a host pins at build time (SEC-142, SEC-051): the
/// trust anchor of every reconstruction it opens. The kit never chooses the
/// set. The host passes the keys of its own installation to
/// `ReconstructionOpener`, so that a production host trusts production roots,
/// a development host trusts development roots, and one binary never trusts
/// both.
library;

import 'package:plux_flutter/plux_flutter.dart';

/// Reads the keys of [json], a `keys.json`: the list `plux pull` writes, of
/// `{keyId, algorithm, role, publicKey}` with the public key in hex. Blank
/// text holds no key. Throws [FormatException], naming [define], the build
/// setting that carried [json], when it is not a key list, so that a build
/// with a mistake in its keys fails at start instead of silently opening
/// nothing.
List<PluxPublicKey> parseRootKeys(String json, {required String define}) {
  if (json.trim().isEmpty) return const [];
  try {
    return PluxPublicKey.parseKeysJson(json);
  } on PluxException {
    throw FormatException(
      '$define is not a keys.json: a JSON list of keyId, algorithm, '
      'role and hex publicKey',
    );
  }
}
