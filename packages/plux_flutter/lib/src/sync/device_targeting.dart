// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a device tells the server about itself and its user (REL-051,
/// SEC-142): the targeting fields of a manifest request, which a rollout
/// matches against, and the reconstruction inputs of the installed-release
/// report. Only hashes of user data leave the device in a manifest request:
/// the user identifier and every user attribute value are hashed here, on
/// the sync isolate, and the server compares hashes (`HashUserAttribute` in
/// `backend/internal/rollout/targeting.go`). The reconstruction report
/// carries the host's pseudonymous user identifier itself, as
/// `ReportInstalledRequest.user_ref` says.
library;

import 'dart:convert';

import 'package:crypto/crypto.dart';
import 'package:plux_flutter/src/sync/api_client.dart' show DeviceInfo;

/// The form in which a device sends, and a rule compares, the value of the
/// user attribute [name]: the hex SHA-256 of the name, a zero byte and the
/// value. Including the name keeps equal values of different attributes
/// apart.
String hashUserAttribute(String name, String value) =>
    sha256.convert(utf8.encode('$name\u0000$value')).toString();

/// The hex SHA-256 of the pseudonymous user identifier [userId], which
/// places a device in a rollout's bucket in place of its own identifier.
String hashUserId(String userId) =>
    sha256.convert(utf8.encode(userId)).toString();

/// What the UI isolate knows about the device and its user at the moment it
/// asks for a sync, sent to the sync isolate with the command: the facts
/// that the sync isolate cannot read for itself. The user's values are in
/// the clear here and never leave the process; [DeviceTargeting.of] hashes
/// them.
final class SyncContext {
  /// Creates the context; every field is optional, and an empty one is
  /// left out of the request.
  const SyncContext({
    this.deviceClass = '',
    this.locale = '',
    this.region = '',
    this.userId,
    this.userAttributes = const {},
  });

  /// `phone` or `tablet`.
  final String deviceClass;

  /// The BCP 47 tag of the locale the device shows.
  final String locale;

  /// The country of the device's locale setting, never derived from the
  /// network address.
  final String region;

  /// The pseudonymous user identifier `setUserContext` gave, or null.
  final String? userId;

  /// The user attributes `setUserContext` gave.
  final Map<String, String> userAttributes;
}

/// The `DeviceTargeting` of a manifest request (`proto/plux/v1/
/// manifest.proto`). The host build and the channel are not in it: the
/// server knows both.
final class DeviceTargeting {
  /// Creates the targeting.
  const DeviceTargeting({
    this.os = '',
    this.osVersion = '',
    this.runtimeVersion = '',
    this.deviceClass = '',
    this.locale = '',
    this.region = '',
    this.userHash = '',
    this.userAttributes = const {},
  });

  /// The targeting of [device] in [context], with the user's values hashed.
  factory DeviceTargeting.of(DeviceInfo device, SyncContext context) {
    final user = context.userId;
    return DeviceTargeting(
      os: device.platform,
      osVersion: device.osVersion,
      runtimeVersion: device.runtimeVersion,
      deviceClass: context.deviceClass,
      locale: context.locale,
      region: context.region,
      userHash: user == null || user.isEmpty ? '' : hashUserId(user),
      userAttributes: {
        for (final MapEntry(:key, :value) in context.userAttributes.entries)
          key: hashUserAttribute(key, value),
      },
    );
  }

  /// The platform, `android` or `ios`.
  final String os;

  /// The operating system version.
  final String osVersion;

  /// The runtime's version.
  final String runtimeVersion;

  /// `phone` or `tablet`.
  final String deviceClass;

  /// The locale tag.
  final String locale;

  /// The region setting.
  final String region;

  /// The hex SHA-256 of the user identifier, or empty.
  final String userHash;

  /// The hash of each user attribute's value, by name.
  final Map<String, String> userAttributes;

  /// The request member, without the fields that are empty.
  Map<String, Object?> toJson() => {
    if (os.isNotEmpty) 'os': os,
    if (osVersion.isNotEmpty) 'osVersion': osVersion,
    if (runtimeVersion.isNotEmpty) 'runtimeVersion': runtimeVersion,
    if (deviceClass.isNotEmpty) 'deviceClass': deviceClass,
    if (locale.isNotEmpty) 'locale': locale,
    if (region.isNotEmpty) 'region': region,
    if (userHash.isNotEmpty) 'userHash': userHash,
    if (userAttributes.isNotEmpty) 'userAttributes': userAttributes,
  };
}

/// What the page looked like to the user when a release became active, kept
/// in the device's history so that a reconstruction shows it as the user saw
/// it (SEC-142): the installed-release report carries it.
final class InstalledContext {
  /// Creates the context; an empty field is not reported.
  const InstalledContext({
    this.theme = '',
    this.textScalePercent = 0,
    this.userRef = '',
  });

  /// The theme in use, `light` or `dark`.
  final String theme;

  /// The text scale in percent, 100 for the default; zero when unknown.
  final int textScalePercent;

  /// The host's pseudonymous user identifier, or empty when none is set.
  final String userRef;

  /// The members of the report request, without the fields that are empty.
  Map<String, Object?> toJson() => {
    if (theme.isNotEmpty) 'theme': theme,
    if (textScalePercent > 0) 'textScalePercent': textScalePercent,
    if (userRef.isNotEmpty) 'userRef': userRef,
  };
}
