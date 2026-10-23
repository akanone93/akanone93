// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The development root keys the Dev app is built with (DEV-001, SEC-051,
/// ADR-0093): the trust anchor of every dev bundle it applies. The Dev app
/// pins development roots only and holds no production root: one binary
/// never trusts both sets (SEC-056), and the viewer owns the production set.
library;

import 'package:plux_flutter/plux_flutter.dart' show PluxPublicKey;
import 'package:plux_host_kit/plux_host_kit.dart';

/// The `--dart-define` that carries the keys: the contents of the
/// installation's development `keys.json`, as `plux pull` writes it, a JSON
/// list of `{keyId, algorithm, role, publicKey}` with the public key in hex.
const String developmentRootKeysDefine = 'PLUX_DEV_ROOT_KEYS';

/// The host build the Dev app reports to the runtime.
const String devHostBuild = 'plux-dev';

/// The environment whose dev bundles the Dev app applies.
const String devEnvironment = 'development';

/// Why a Dev app built without development roots pairs with nothing.
const String noDevelopmentRootsMessage =
    'No development roots configured. This build pins no development root '
    'keys, so it cannot tell a genuine dev bundle from a forged one and '
    'pairs with nothing. Build it with '
    '--dart-define=$developmentRootKeysDefine=<keys.json>.';

/// Reads the development roots of [json], a `keys.json`; blank text holds
/// none. Throws [FormatException] naming [developmentRootKeysDefine] when it
/// is not a key list.
List<PluxPublicKey> parseDevelopmentRootKeys(String json) =>
    parseRootKeys(json, define: developmentRootKeysDefine);

/// The keys the Dev app was built with; empty when none were given.
List<PluxPublicKey> developmentRootKeys() => parseDevelopmentRootKeys(
  const String.fromEnvironment(developmentRootKeysDefine),
);
