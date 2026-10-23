// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What the Plux host apps share (P10 D-6, ADR-0093): signing in to a Plux
/// server (the device authorization grant, or an access token), handling the
/// links the platform delivers, reading the root keys a host pins, and
/// opening a reconstruction release in the runtime. Nothing here chooses a
/// trust anchor: the host passes the root keys it was built with.
library;

export 'src/api.dart';
export 'src/controller.dart';
export 'src/device_sign_in.dart';
export 'src/link.dart';
export 'src/links.dart';
export 'src/opener.dart';
export 'src/root_keys.dart';
