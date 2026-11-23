// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_dev/dev_http.dart';

/// Starts the Plux Dev app with the dev channel
/// (`--dart-define=plux.devtools=true`), trusting the development
/// certificate authority it was built with (`--dart-define=PLUX_DEV_CA=<PEM>`,
/// optional). It is built with no key: the phone connects to Studio once
/// (Profile › Phones), pinning its organisation's development root, and each
/// app starts, when the developer taps it, with the development key that
/// root certifies for it (ADR-0109).
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  checkDevCertificateAuthority();
  // Dark status-bar icons over the app's light screens, from the first frame.
  SystemChrome.setSystemUIOverlayStyle(devSystemBars);
  final name = await phoneName();
  runApp(
    DevApp(
      phones: RuntimePhones(client: devHttpClient, deviceName: name),
      deviceName: name,
    ),
  );
}
