// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import Foundation

/// The rules of background sync on iOS (SYN-014, ADR-0092) that need no
/// system framework: what the app asked for, when the next run may begin,
/// which constraint stops a run, and whether the host declared the task. It
/// imports only Foundation so that a unit test can run it anywhere.
struct PluxBackgroundSyncPlan: Equatable {
  /// The identifier the host lists under `BGTaskSchedulerPermittedIdentifiers`.
  static let identifier = "dev.plux.backgroundSync"

  /// The least interval, in minutes: the limit `sync.backgroundIntervalMinutes` minimum.
  static let minimumIntervalMinutes = 15

  /// The limit's default, in minutes.
  static let defaultIntervalMinutes = 360

  let intervalMinutes: Int
  let requiresUnmeteredNetwork: Bool
  let requiresCharging: Bool
  let requiresBatteryNotLow: Bool

  /// Reads the arguments of `backgroundSyncSchedule`. A missing interval is
  /// the default and a shorter one the minimum; the constraints default to
  /// the safest ones.
  init(arguments: [String: Any]) {
    let minutes = (arguments["intervalMinutes"] as? NSNumber)?.intValue ?? Self.defaultIntervalMinutes
    intervalMinutes = max(minutes, Self.minimumIntervalMinutes)
    requiresUnmeteredNetwork = arguments["requiresUnmeteredNetwork"] as? Bool ?? false
    requiresCharging = arguments["requiresCharging"] as? Bool ?? false
    requiresBatteryNotLow = arguments["requiresBatteryNotLow"] as? Bool ?? true
  }

  /// The plan as stored for the runs the system starts without the app.
  var stored: [String: Any] {
    [
      "intervalMinutes": intervalMinutes,
      "requiresUnmeteredNetwork": requiresUnmeteredNetwork,
      "requiresCharging": requiresCharging,
      "requiresBatteryNotLow": requiresBatteryNotLow,
    ]
  }

  /// The `earliestBeginDate` of a request made at [now]: the system starts
  /// the task no sooner, and decides itself when.
  func earliestBeginDate(from now: Date) -> Date {
    now.addingTimeInterval(TimeInterval(intervalMinutes * 60))
  }

  /// The constraint the device does not meet, or nil. iOS has no scheduler
  /// constraints for background app refresh, so the task checks them itself
  /// when it starts; a run that returns one is skipped with `PLX-3502`.
  func unmetConstraint(charging: Bool, lowPowerMode: Bool, expensiveNetwork: Bool) -> String? {
    if requiresCharging && !charging { return "charging" }
    if requiresBatteryNotLow && lowPowerMode { return "batteryNotLow" }
    if requiresUnmeteredNetwork && expensiveNetwork { return "unmeteredNetwork" }
    return nil
  }

  /// Whether the host declared [identifier] in `BGTaskSchedulerPermittedIdentifiers`.
  static func isDeclared(_ identifier: String, permitted: Any?) -> Bool {
    guard let list = permitted as? [String] else { return false }
    return list.contains(identifier)
  }
}
