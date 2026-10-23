// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import BackgroundTasks
import Flutter
import Network
import os
import UIKit

/// Opt-in background sync on iOS (SYN-014, ADR-0092): a `BGAppRefreshTask`
/// through `BGTaskScheduler`, under the identifier the host declares in its
/// `Info.plist` (`BGTaskSchedulerPermittedIdentifiers`) with the `fetch`
/// background mode. The system decides when, and whether, it runs. A run
/// starts a headless Flutter engine at the Dart entry point
/// `pluxBackgroundSyncMain`, which verifies and stages a release and never
/// activates it.
///
/// `BGTaskScheduler.register` must happen before the app finishes
/// launching, which is when Flutter registers its plugins; the call is made
/// once per process, whatever number of engines register the plugin.
final class PluxBackgroundSync {
  private static let log = Logger(subsystem: "dev.plux", category: "background-sync")
  private static let defaults = UserDefaults(suiteName: "dev.plux.background-sync") ?? .standard
  private static let lock = NSLock()
  private static var registered = false
  private static var declared = false

  /// Registers the task if the host declared it. Without the declaration
  /// nothing is registered, and scheduling reports `PLX-3501` instead of
  /// letting the system raise an exception.
  static func registerTask() {
    lock.lock()
    defer { lock.unlock() }
    if registered { return }
    registered = true
    let permitted = Bundle.main.object(forInfoDictionaryKey: "BGTaskSchedulerPermittedIdentifiers")
    declared = PluxBackgroundSyncPlan.isDeclared(PluxBackgroundSyncPlan.identifier, permitted: permitted)
    guard declared else {
      log.notice("PLX-3501 \(PluxBackgroundSyncPlan.identifier, privacy: .public) is not in BGTaskSchedulerPermittedIdentifiers")
      return
    }
    BGTaskScheduler.shared.register(forTaskWithIdentifier: PluxBackgroundSyncPlan.identifier, using: nil) { task in
      guard let refresh = task as? BGAppRefreshTask else {
        task.setTaskCompleted(success: false)
        return
      }
      PluxBackgroundRun(task: refresh).start()
    }
  }

  func handle(_ method: String, _ args: [String: Any], result: @escaping FlutterResult) {
    switch method {
    case "backgroundSyncSchedule":
      guard Self.declared else {
        return result(FlutterError(code: "PLX-3501", message: "not declared", details: nil))
      }
      guard let callback = (args["callback"] as? NSNumber)?.int64Value, let descriptor = args["descriptor"] as? String else {
        return result(FlutterError(code: "PLUX_PLATFORM", message: "arguments", details: nil))
      }
      let plan = PluxBackgroundSyncPlan(arguments: args)
      Self.defaults.set(NSNumber(value: callback), forKey: "callback")
      Self.defaults.set(descriptor, forKey: "descriptor")
      Self.defaults.set(plan.stored, forKey: "plan")
      do {
        try Self.submit(plan)
        result(nil)
      } catch {
        result(FlutterError(code: "PLUX_PLATFORM", message: "BGTaskScheduler", details: nil))
      }
    case "backgroundSyncCancel":
      if Self.declared {
        BGTaskScheduler.shared.cancel(taskRequestWithIdentifier: PluxBackgroundSyncPlan.identifier)
      }
      for key in ["callback", "descriptor", "plan"] { Self.defaults.removeObject(forKey: key) }
      result(nil)
    default:
      result(FlutterMethodNotImplemented)
    }
  }

  /// Asks the system for the next run, no sooner than the plan's interval.
  static func submit(_ plan: PluxBackgroundSyncPlan) throws {
    let request = BGAppRefreshTaskRequest(identifier: PluxBackgroundSyncPlan.identifier)
    request.earliestBeginDate = plan.earliestBeginDate(from: Date())
    try BGTaskScheduler.shared.submit(request)
  }

  static var storedPlan: PluxBackgroundSyncPlan? {
    guard let plan = defaults.dictionary(forKey: "plan") else { return nil }
    return PluxBackgroundSyncPlan(arguments: plan)
  }

  static var storedCallback: Int64? {
    (defaults.object(forKey: "callback") as? NSNumber)?.int64Value
  }

  static var storedDescriptor: String? {
    defaults.string(forKey: "descriptor")
  }

  static func logSkipped(_ constraint: String) {
    log.notice("PLX-3502 background sync skipped: \(constraint, privacy: .public)")
  }
}

/// One run of the task: checks the constraints, asks for the next run, and
/// runs the Dart entry point in a headless engine until it reports `done` or
/// the system expires the task.
final class PluxBackgroundRun {
  private let task: BGAppRefreshTask
  private var engine: FlutterEngine?
  private var completed = false
  private let lock = NSLock()

  init(task: BGAppRefreshTask) {
    self.task = task
  }

  func start() {
    task.expirationHandler = { [weak self] in self?.finish(success: false) }
    guard let plan = PluxBackgroundSync.storedPlan,
          let callback = PluxBackgroundSync.storedCallback,
          PluxBackgroundSync.storedDescriptor != nil else {
      // Cancelled between the request and the run.
      return finish(success: true)
    }
    // The next run is asked for first: a run that fails or is cut short must
    // not end the series.
    try? PluxBackgroundSync.submit(plan)
    DispatchQueue.main.async { [self] in
      UIDevice.current.isBatteryMonitoringEnabled = true
      let state = UIDevice.current.batteryState
      let unmet = plan.unmetConstraint(
        charging: state == .charging || state == .full,
        lowPowerMode: ProcessInfo.processInfo.isLowPowerModeEnabled,
        expensiveNetwork: plan.requiresUnmeteredNetwork ? Self.networkIsExpensive() : false
      )
      if let unmet = unmet {
        PluxBackgroundSync.logSkipped(unmet)
        return finish(success: true)
      }
      run(callback: callback)
    }
  }

  private func run(callback: Int64) {
    guard let info = FlutterCallbackCache.lookupCallbackInformation(callback) else {
      return finish(success: false)
    }
    let flutter = FlutterEngine(name: "dev.plux.background-sync", project: nil, allowHeadlessExecution: true)
    engine = flutter
    let channel = FlutterMethodChannel(name: "dev.plux/background-sync", binaryMessenger: flutter.binaryMessenger)
    channel.setMethodCallHandler { [weak self] call, result in
      switch call.method {
      case "descriptor":
        result(PluxBackgroundSync.storedDescriptor)
      case "done":
        let outcome = (call.arguments as? [String: Any])?["outcome"] as? String ?? "failed"
        result(nil)
        self?.finish(success: outcome != "failed")
      default:
        result(FlutterMethodNotImplemented)
      }
    }
    // Only the Plux plugin is registered: the sync needs the runtime channel
    // and nothing else of the host.
    if let registrar = flutter.registrar(forPlugin: "PluxFlutterPlugin") {
      PluxFlutterPlugin.register(with: registrar)
    }
    if !flutter.run(withEntrypoint: info.callbackName, libraryURI: info.callbackLibraryPath) {
      finish(success: false)
    }
  }

  /// Completes the task once, on any thread, whether the run ended, failed or
  /// was expired by the system; the engine is stopped with it.
  private func finish(success: Bool) {
    lock.lock()
    if completed {
      lock.unlock()
      return
    }
    completed = true
    lock.unlock()
    task.setTaskCompleted(success: success)
    DispatchQueue.main.async { [self] in
      engine?.destroyContext()
      engine = nil
    }
  }

  /// Whether the current network path is expensive (cellular or a hotspot);
  /// a path that does not report within a second counts as expensive.
  private static func networkIsExpensive() -> Bool {
    let monitor = NWPathMonitor()
    let semaphore = DispatchSemaphore(value: 0)
    let answer = Answer()
    monitor.pathUpdateHandler = { path in
      answer.set(path.isExpensive)
      semaphore.signal()
    }
    monitor.start(queue: DispatchQueue(label: "dev.plux.background-sync.path"))
    let reported = semaphore.wait(timeout: .now() + 1) == .success
    monitor.cancel()
    return reported ? answer.value : true
  }

  private final class Answer {
    private let lock = NSLock()
    private var stored = true
    func set(_ value: Bool) {
      lock.lock()
      stored = value
      lock.unlock()
    }
    var value: Bool {
      lock.lock()
      defer { lock.unlock() }
      return stored
    }
  }
}
