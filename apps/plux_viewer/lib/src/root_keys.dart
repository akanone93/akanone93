// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The root keys the viewer is built with (SEC-142, SEC-051): the trust
/// anchor of every reconstruction it opens. A host app pins the root keys of
/// its installation at build time; the viewer does the same, so that neither
/// the server nor anyone in the TLS path can make it accept a forged
/// reconstruction by offering keys of their own.
library;

import 'package:plux_flutter/plux_flutter.dart';

/// The `--dart-define` that carries the keys: the contents of the
/// installation's `keys.json`, as `plux pull` writes it, a JSON list of
/// `{keyId, algorithm, role, publicKey}` with the public key in hex.
const String rootKeysDefine = 'PLUX_VIEWER_ROOT_KEYS';

/// The keys the viewer was built with; empty when none were given.
List<PluxPublicKey> builtInRootKeys() =>
    parseRootKeys(const String.fromEnvironment(rootKeysDefine));

/// Reads the keys of [json], a `keys.json`. Blank text holds no key.
/// Throws [FormatException] when it is not a key list, so that a build with
/// a mistake in its keys fails at start instead of silently opening nothing.
List<PluxPublicKey> parseRootKeys(String json) {
  if (json.trim().isEmpty) return const [];
  try {
    return PluxPublicKey.parseKeysJson(json);
  } on PluxException {
    throw const FormatException(
      '$rootKeysDefine is not a keys.json: a JSON list of keyId, algorithm, '
      'role and hex publicKey',
    );
  }
}
