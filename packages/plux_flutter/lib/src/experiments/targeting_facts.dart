// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The facts about this device that targeting rules compare (REL-051,
/// ABT-006): build, runtime version, OS, device class, locale, region,
/// channel and the user attributes `setUserContext` gave. They are read on
/// the device and compared there; none of them is sent anywhere.
library;

import 'dart:ui' show Locale;

import 'package:plux_flutter/src/core/config.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';
import 'package:plux_flutter/src/runtime_info.dart';

/// A window at least this wide, in logical pixels, is a tablet.
const double _tabletWidth = 600;

/// The targeting facts of a device whose host app has build [hostBuild],
/// on release channel [channel], running [os] with [osVersion] (empty when
/// unknown, and then absent), showing [locale] in a window [width] logical
/// pixels wide, for [user].
///
/// `build` is a number when [hostBuild] is one; `region` is the country of
/// the device's locale setting, never derived from the network address.
TargetingFacts targetingFacts({
  required String hostBuild,
  required String channel,
  required String os,
  required Locale locale,
  required double width,
  String osVersion = '',
  PluxUser? user,
}) => {
  'build': ?(int.tryParse(hostBuild) ?? (hostBuild.isEmpty ? null : hostBuild)),
  'runtimeVersion': PluxRuntimeInfo.version,
  'os': os,
  if (osVersion.isNotEmpty) 'osVersion': osVersion,
  'deviceClass': width >= _tabletWidth ? 'tablet' : 'phone',
  'locale': locale.toLanguageTag(),
  if (locale.countryCode case final region? when region.isNotEmpty)
    'region': region,
  'channel': channel,
  if (user != null)
    for (final MapEntry(:key, :value) in user.attributes.entries)
      'user.$key': value,
};
