// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The production root keys the viewer is built with (SEC-142, SEC-051): the
/// trust anchor of every reconstruction it opens. A host app pins the root
/// keys of its installation at build time; the viewer does the same, so that
/// neither the server nor anyone in the TLS path can make it accept a forged
/// reconstruction by offering keys of their own. The viewer owns this set and
/// passes it to the host kit; the development app passes development roots
/// of its own, and no binary holds both.
library;

import 'package:plux_flutter/plux_flutter.dart' show PluxPublicKey;
import 'package:plux_host_kit/plux_host_kit.dart';

/// The `--dart-define` that carries the keys: the contents of the
/// installation's `keys.json`, as `plux pull` writes it, a JSON list of
/// `{keyId, algorithm, role, publicKey}` with the public key in hex.
const String rootKeysDefine = 'PLUX_VIEWER_ROOT_KEYS';

/// The host build the viewer reports to the runtime.
const String viewerHostBuild = 'plux-viewer';

/// Why a viewer built without root keys opens nothing.
const String viewerNoRootKeysMessage =
    'This build of the viewer pins no root keys, so it cannot tell a genuine '
    'reconstruction from a forged one. Build it with '
    '--dart-define=PLUX_VIEWER_ROOT_KEYS=<keys.json>.';

/// The keys the viewer was built with; empty when none were given.
List<PluxPublicKey> builtInRootKeys() => parseRootKeys(
  const String.fromEnvironment(rootKeysDefine),
  define: rootKeysDefine,
);
