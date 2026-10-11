// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import AVFoundation
import Contacts
import CoreLocation
import Flutter
import Foundation
import LocalAuthentication
import Photos
import UIKit
import UserNotifications

/// The device actions that need the platform (SEC-080): the share sheet and
/// permission prompts, the biometric prompt (SEC-027, SEC-094) and the cover
/// of secure pages (SEC-091). Nothing here reads a picked file's contents; a
/// file is handed to the share sheet by URL, and only files under the app's
/// container can be shared. The biometric prompt answers with a word and
/// never with biometric data.
final class PluxDevice: NSObject, CLLocationManagerDelegate {
  private var location: CLLocationManager?
  private var locationResult: FlutterResult?
  private var secure = false
  private var cover: UIView?
  private var inactive = false

  override init() {
    super.init()
    let center = NotificationCenter.default
    center.addObserver(self, selector: #selector(resigned), name: UIApplication.willResignActiveNotification, object: nil)
    center.addObserver(self, selector: #selector(became), name: UIApplication.didBecomeActiveNotification, object: nil)
    center.addObserver(self, selector: #selector(coverNeedsUpdate), name: UIScreen.capturedDidChangeNotification, object: nil)
  }

  deinit {
    NotificationCenter.default.removeObserver(self)
  }

  // MARK: Secure pages

  /// Turns the cover of the window on or off. iOS cannot block screenshots,
  /// so while a secure page shows, the window is covered when the screen is
  /// captured (recording, mirroring) and blurred in the app switcher.
  func secureScreen(_ args: [String: Any], result: @escaping FlutterResult) {
    guard let enabled = args["enabled"] as? Bool else {
      return result(FlutterError(code: "PLUX_PLATFORM", message: "enabled", details: nil))
    }
    secure = enabled
    updateCover()
    result(nil)
  }

  // willResignActive comes while the application is still active, so the
  // state is kept here rather than read.
  @objc private func resigned() {
    inactive = true
    updateCover()
  }

  @objc private func became() {
    inactive = false
    updateCover()
  }

  @objc private func coverNeedsUpdate() {
    updateCover()
  }

  private func updateCover() {
    let captured = UIScreen.main.isCaptured
    guard secure, inactive || captured, let window = Self.keyWindow() else {
      cover?.removeFromSuperview()
      cover = nil
      return
    }
    // A captured screen is covered outright; the app switcher's snapshot is
    // blurred.
    let view: UIView = captured && !inactive
      ? UIView()
      : UIVisualEffectView(effect: UIBlurEffect(style: .systemThickMaterial))
    if captured && !inactive { view.backgroundColor = .systemBackground }
    cover?.removeFromSuperview()
    view.frame = window.bounds
    view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
    window.addSubview(view)
    cover = view
  }

  private static func keyWindow() -> UIWindow? {
    UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
      .flatMap { $0.windows }.first(where: { $0.isKeyWindow })
  }

  // MARK: Biometrics

  /// Asks the user to confirm with Face ID or Touch ID or, when allowed, the
  /// device passcode. The answer is one of "success", "cancelled", "failed"
  /// or "unavailable". The app's Info.plist needs NSFaceIDUsageDescription.
  func biometricAuth(_ args: [String: Any], result: @escaping FlutterResult) {
    guard let reason = args["reason"] as? String, !reason.isEmpty else {
      return result(FlutterError(code: "PLUX_PLATFORM", message: "reason", details: nil))
    }
    let credential = args["allowDeviceCredential"] as? Bool ?? true
    let policy: LAPolicy = credential ? .deviceOwnerAuthentication : .deviceOwnerAuthenticationWithBiometrics
    let context = LAContext()
    var error: NSError?
    guard context.canEvaluatePolicy(policy, error: &error) else {
      return result((error as? LAError)?.code == .biometryLockout ? "failed" : "unavailable")
    }
    context.evaluatePolicy(policy, localizedReason: reason) { ok, error in
      DispatchQueue.main.async {
        if ok { return result("success") }
        switch (error as? LAError)?.code {
        case .userCancel, .systemCancel, .appCancel, .userFallback:
          result("cancelled")
        case .biometryNotAvailable, .biometryNotEnrolled, .passcodeNotSet:
          result("unavailable")
        default:
          result("failed")
        }
      }
    }
  }

  // MARK: Share

  func share(_ args: [String: Any], result: @escaping FlutterResult) {
    var items: [Any] = []
    if let text = args["text"] as? String { items.append(text) }
    if let url = args["url"] as? String, let u = URL(string: url) { items.append(u) }
    if let path = args["filePath"] as? String {
      let file = URL(fileURLWithPath: path).resolvingSymlinksInPath()
      guard Self.insideContainer(file) else {
        return result(FlutterError(code: "PLUX_PLATFORM", message: "file", details: nil))
      }
      items.append(file)
    }
    guard !items.isEmpty, let top = Self.topViewController() else {
      return result(FlutterError(code: "PLUX_PLATFORM", message: "presenter", details: nil))
    }
    let sheet = UIActivityViewController(activityItems: items, applicationActivities: nil)
    // iPad presents the sheet as a popover anchored to the screen's centre.
    if let popover = sheet.popoverPresentationController {
      popover.sourceView = top.view
      popover.sourceRect = CGRect(x: top.view.bounds.midX, y: top.view.bounds.midY, width: 0, height: 0)
      popover.permittedArrowDirections = []
    }
    sheet.completionWithItemsHandler = { _, _, _, _ in result(nil) }
    top.present(sheet, animated: true)
  }

  private static func insideContainer(_ file: URL) -> Bool {
    let home = URL(fileURLWithPath: NSHomeDirectory()).resolvingSymlinksInPath().path
    return file.path.hasPrefix(home + "/")
  }

  private static func topViewController() -> UIViewController? {
    let scene = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
    var top = scene?.windows.first(where: { $0.isKeyWindow })?.rootViewController
    while let presented = top?.presentedViewController { top = presented }
    return top
  }

  // MARK: Permissions

  func requestPermission(_ args: [String: Any], result: @escaping FlutterResult) {
    switch args["permission"] as? String {
    case "camera":
      AVCaptureDevice.requestAccess(for: .video) { ok in DispatchQueue.main.async { result(ok) } }
    case "photos":
      PHPhotoLibrary.requestAuthorization(for: .readWrite) { status in
        DispatchQueue.main.async { result(status == .authorized || status == .limited) }
      }
    case "contacts":
      CNContactStore().requestAccess(for: .contacts) { ok, _ in DispatchQueue.main.async { result(ok) } }
    case "notifications":
      UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .badge, .sound]) { ok, _ in
        DispatchQueue.main.async { result(ok) }
      }
    case "biometrics":
      let context = LAContext()
      var error: NSError?
      guard context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &error) else {
        return result(false)
      }
      context.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: "Allow biometric sign-in") { ok, _ in
        DispatchQueue.main.async { result(ok) }
      }
    case "location":
      requestLocation(result)
    default:
      result(FlutterError(code: "PLUX_PLATFORM", message: "permission", details: nil))
    }
  }

  private func requestLocation(_ result: @escaping FlutterResult) {
    let manager = CLLocationManager()
    switch manager.authorizationStatus {
    case .authorizedWhenInUse, .authorizedAlways:
      return result(true)
    case .denied, .restricted:
      return result(false)
    default:
      break
    }
    locationResult?(false)
    locationResult = result
    manager.delegate = self
    location = manager
    manager.requestWhenInUseAuthorization()
  }

  func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
    guard manager.authorizationStatus != .notDetermined, let result = locationResult else { return }
    locationResult = nil
    location = nil
    result(manager.authorizationStatus == .authorizedWhenInUse || manager.authorizationStatus == .authorizedAlways)
  }
}
