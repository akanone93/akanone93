// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import Darwin
import Foundation
import Security
import UIKit

/// Plux's own checks for a hostile iOS device (SEC-070, ADR-0057). Each
/// check is small and answers with the codes of the indicators that
/// tripped.
///
/// Every check can be evaded by someone who controls the device: jailbreak
/// hiders rename paths and filter dyld's image list, a debugger can clear
/// P_TRACED, Frida can be renamed. The checks raise the cost of an attack;
/// they are signals for the server to weigh, not proof.
enum PluxRasp {
  /// Runs every check. A bundle or team identifier that is nil is not
  /// compared.
  static func scan(teamId: String?, bundleId: String?) -> [[String: Any]] {
    let checks: [(String, [String])] = [
      ("root", jailbreak()),
      ("hooking", hooking()),
      ("debugger", debugger()),
      ("emulator", simulator()),
      ("repackaging", repackaging(teamId: teamId, bundleId: bundleId)),
      ("screen_capture", screenCapture()),
    ]
    return checks.filter { !$0.1.isEmpty }.map { ["kind": $0.0, "indicators": $0.1] }
  }

  private static let jailbreakPaths = [
    "/Applications/Cydia.app", "/Applications/Sileo.app", "/Applications/Zebra.app",
    "/Library/MobileSubstrate/MobileSubstrate.dylib", "/usr/libexec/cydia", "/usr/sbin/sshd",
    "/usr/bin/ssh", "/bin/bash", "/etc/apt", "/private/var/lib/apt/", "/private/var/stash",
    "/var/jb",
  ]
  private static let jailbreakSchemes = ["cydia://package/com.example.package", "sileo://package/com.example.package", "zbra://packages/com.example.package"]
  private static let hookingImages = ["mobilesubstrate", "substrateloader", "substitute", "tweakinject", "ellekit", "libhooker"]
  private static let fridaImages = ["frida", "cynject", "libcycript"]

  /// Jailbreak: its files, a write outside the sandbox, its package managers' URL schemes.
  private static func jailbreak() -> [String] {
    var found: [String] = []
    if jailbreakPaths.contains(where: { FileManager.default.fileExists(atPath: $0) }) { found.append("jb_paths") }
    if canWriteOutsideSandbox() { found.append("sandbox_write") }
    // canOpenURL answers only for the schemes the host lists in LSApplicationQueriesSchemes.
    let schemes = DispatchQueue.main.sync {
      jailbreakSchemes.contains { URL(string: $0).map { UIApplication.shared.canOpenURL($0) } ?? false }
    }
    if schemes { found.append("url_schemes") }
    return found
  }

  /// A sandboxed app cannot create a file outside its container.
  private static func canWriteOutsideSandbox() -> Bool {
    let path = "/private/plux-rasp-probe"
    do {
      try "probe".write(toFile: path, atomically: true, encoding: .utf8)
      try? FileManager.default.removeItem(atPath: path)
      return true
    } catch {
      return false
    }
  }

  /// Hooking: Substrate-like and Frida libraries in the process, or injection by environment.
  private static func hooking() -> [String] {
    var found: [String] = []
    let images = (0..<_dyld_image_count()).compactMap { _dyld_get_image_name($0).map { String(cString: $0).lowercased() } }
    if images.contains(where: { image in hookingImages.contains { image.contains($0) } }) { found.append("substrate_dylib") }
    if images.contains(where: { image in fridaImages.contains { image.contains($0) } }) { found.append("frida_dylib") }
    if getenv("DYLD_INSERT_LIBRARIES") != nil { found.append("dyld_insert") }
    return found
  }

  /// A debugger: the kernel marks a traced process.
  private static func debugger() -> [String] {
    var info = kinfo_proc()
    var size = MemoryLayout<kinfo_proc>.stride
    var mib: [Int32] = [CTL_KERN, KERN_PROC, KERN_PROC_PID, getpid()]
    let status = sysctl(&mib, UInt32(mib.count), &info, &size, nil, 0)
    return status == 0 && (info.kp_proc.p_flag & P_TRACED) != 0 ? ["p_traced"] : []
  }

  /// The simulator.
  private static func simulator() -> [String] {
    #if targetEnvironment(simulator)
      return ["simulator"]
    #else
      return []
    #endif
  }

  /// Repackaging: a copy re-signed by someone else has another bundle
  /// identifier or another team.
  private static func repackaging(teamId: String?, bundleId: String?) -> [String] {
    var found: [String] = []
    if let bundleId, Bundle.main.bundleIdentifier != bundleId { found.append("bundle_id") }
    if let teamId, appIdentifierPrefix() != teamId { found.append("team_id") }
    return found
  }

  /// The team the app is signed by: the first part of the access group of a keychain item the app creates.
  private static func appIdentifierPrefix() -> String? {
    let probe: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: "dev.plux.rasp",
      kSecAttrAccount as String: "probe",
    ]
    SecItemDelete(probe as CFDictionary)
    var add = probe
    add[kSecReturnAttributes as String] = true
    var item: CFTypeRef?
    defer { SecItemDelete(probe as CFDictionary) }
    guard SecItemAdd(add as CFDictionary, &item) == errSecSuccess,
          let attributes = item as? [String: Any],
          let group = attributes[kSecAttrAccessGroup as String] as? String else { return nil }
    return group.split(separator: ".").first.map(String.init)
  }

  /// The screen is recorded, mirrored or shown on an external display.
  private static func screenCapture() -> [String] {
    DispatchQueue.main.sync {
      let screens = UIApplication.shared.connectedScenes.compactMap { ($0 as? UIWindowScene)?.screen }
      return screens.contains { $0.isCaptured } ? ["captured"] : []
    }
  }
}
