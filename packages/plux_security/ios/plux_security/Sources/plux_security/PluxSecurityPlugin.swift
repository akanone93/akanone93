// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import Flutter
import Foundation

/// The platform channel of plux_security (SEC-070, ADR-0057): one call,
/// `scan`, that runs the checks of PluxRasp on a background queue and
/// answers with what they found. Nothing here reads or returns a path, an
/// identifier or any value from the device, only the kind of finding and the
/// codes of the checks that tripped (SEC-092).
public class PluxSecurityPlugin: NSObject, FlutterPlugin {
  private let queue = DispatchQueue(label: "dev.plux.security", qos: .utility)

  public static func register(with registrar: FlutterPluginRegistrar) {
    let channel = FlutterMethodChannel(name: "dev.plux/security", binaryMessenger: registrar.messenger())
    registrar.addMethodCallDelegate(PluxSecurityPlugin(), channel: channel)
  }

  public func handle(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
    guard call.method == "scan" else { return result(FlutterMethodNotImplemented) }
    let args = call.arguments as? [String: Any] ?? [:]
    let teamId = args["iosTeamId"] as? String
    let bundleId = args["iosBundleId"] as? String
    queue.async {
      let found = PluxRasp.scan(teamId: teamId, bundleId: bundleId)
      DispatchQueue.main.async { result(found) }
    }
  }
}
