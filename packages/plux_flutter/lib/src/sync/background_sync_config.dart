// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The configuration of opt-in background sync (SYN-014, ADR-0092).
library;

/// Opt-in background sync (SYN-014): the platform wakes the app now and
/// then, without a screen, to stage a newer release for the next start.
///
/// It is off by default. The task only stages: a release becomes active at
/// the next start or safe point, under the app's activation policy. Where
/// the system does not run the task (iOS decides on its own), nothing is
/// lost: the app syncs at start as before.
final class BackgroundSyncConfig {
  /// Creates a configuration; [enabled] is off unless set.
  const BackgroundSyncConfig({
    this.enabled = false,
    this.interval = const Duration(minutes: defaultIntervalMinutes),
    this.requiresUnmeteredNetwork = false,
    this.requiresCharging = false,
    this.requiresBatteryNotLow = true,
  });

  /// Background sync, on, with the default interval and constraints.
  const BackgroundSyncConfig.enabled() : this(enabled: true);

  /// The default of the limit `sync.backgroundIntervalMinutes`: six hours.
  static const int defaultIntervalMinutes = 360;

  /// The shortest interval, in minutes: the least WorkManager runs periodic
  /// work at, and the limit's minimum.
  static const int minimumIntervalMinutes = 15;

  /// Whether the app asks the platform to sync in the background.
  final bool enabled;

  /// How often to sync (the limit `sync.backgroundIntervalMinutes`, default
  /// 360 minutes); a shorter value than 15 minutes counts as 15. The system
  /// may run the task later, and on iOS decides itself whether it runs.
  final Duration interval;

  /// Whether the device must be on an unmetered network (Wi-Fi). On iOS the
  /// system has no such constraint; the task checks the network when it
  /// starts and skips the run on an expensive one (`PLX-3502`).
  final bool requiresUnmeteredNetwork;

  /// Whether the device must be charging.
  final bool requiresCharging;

  /// Whether the battery must not be low (on iOS: Low Power Mode off).
  final bool requiresBatteryNotLow;

  /// [interval] in whole minutes, at least [minimumIntervalMinutes].
  int get intervalMinutes {
    final m = interval.inMinutes;
    return m < minimumIntervalMinutes ? minimumIntervalMinutes : m;
  }
}

/// The identifier of the iOS task the host lists under
/// `BGTaskSchedulerPermittedIdentifiers` in its `Info.plist`.
const String backgroundSyncTaskIdentifier = 'dev.plux.backgroundSync';
