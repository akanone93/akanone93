// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import CryptoKit
import Flutter
import Foundation
import Security

/// The key that wraps the data keys of the encrypted local stores (SEC-073,
/// ADR-0058): one P-256 key in the Secure Enclave, or in the Keychain where no
/// Secure Enclave exists (the simulator). A data key is wrapped with ECIES
/// (cofactor X9.63, SHA-256, AES-GCM), so wrapping needs only the public key and
/// unwrapping runs the key agreement inside the Secure Enclave. The runtime hands
/// over a data key to wrap and gets it back once per launch; the hardware key
/// never leaves the device.
final class PluxDataKeys {
  private static let tag = Data("dev.plux.datakeys".utf8)
  private static let algorithm = SecKeyAlgorithm.eciesEncryptionCofactorX963SHA256AESGCM
  private static let maxBytes = 1024
  /// An ECIES blob adds an ephemeral public point (65 bytes), the AES-GCM
  /// nonce and tag (16 bytes each).
  private static let overhead = 65 + 16
  private static let queue = DispatchQueue(label: "dev.plux.datakeys", qos: .userInitiated)

  /// Handles dataKeyWrap and dataKeyUnwrap. Keychain and Secure Enclave work
  /// runs on a serial queue; `result` is called on the main queue.
  func handle(_ method: String, _ args: [String: Any], result: @escaping FlutterResult) {
    switch method {
    case "dataKeyWrap":
      guard let data = (args["data"] as? FlutterStandardTypedData)?.data,
            !data.isEmpty, data.count <= Self.maxBytes
      else { return result(Self.fail("PLUX_PLATFORM", "data")) }
      Self.run(result) { FlutterStandardTypedData(bytes: try Self.wrap(data)) }
    case "dataKeyUnwrap":
      guard let wrapped = (args["wrapped"] as? FlutterStandardTypedData)?.data,
            wrapped.count > Self.overhead, wrapped.count <= Self.maxBytes + Self.overhead
      else { return result(Self.fail("PLUX_PLATFORM", "wrapped")) }
      Self.run(result) { FlutterStandardTypedData(bytes: try Self.unwrap(wrapped)) }
    default:
      result(FlutterMethodNotImplemented)
    }
  }

  // MARK: - Dispatch

  private struct KeyError: Error {
    let code: String
    let message: String
  }

  private static func fail(_ code: String, _ message: String) -> FlutterError {
    FlutterError(code: code, message: message, details: nil)
  }

  private static func run(_ result: @escaping FlutterResult, _ work: @escaping () throws -> Any?) {
    queue.async {
      let value: Any?
      do {
        value = try work()
      } catch let e as KeyError {
        value = fail(e.code, e.message)
      } catch {
        value = fail("PLUX_PLATFORM", "datakey")
      }
      DispatchQueue.main.async { result(value) }
    }
  }

  /// Only the failing step and its numeric code are reported, never key
  /// material or the system's error text.
  private static func platform(_ step: String, _ code: Int) -> KeyError {
    KeyError(code: "PLUX_PLATFORM", message: "\(step) \(code)")
  }

  private static func code(_ error: Unmanaged<CFError>?) -> Int {
    error.map { CFErrorGetCode($0.takeRetainedValue()) } ?? 0
  }

  // MARK: - Keychain

  private static func query() -> [String: Any] {
    [kSecClass as String: kSecClassKey,
     kSecAttrApplicationTag as String: tag,
     kSecAttrKeyClass as String: kSecAttrKeyClassPrivate]
  }

  private static func lookup() throws -> SecKey? {
    var q = query()
    q[kSecReturnRef as String] = true
    q[kSecMatchLimit as String] = kSecMatchLimitOne
    var item: CFTypeRef?
    let status = SecItemCopyMatching(q as CFDictionary, &item)
    if status == errSecItemNotFound { return nil }
    guard status == errSecSuccess, let value = item else { throw platform("keychain", Int(status)) }
    // A CoreFoundation type cannot be cast conditionally: its type ID is
    // checked first, then the cast cannot fail.
    guard CFGetTypeID(value) == SecKeyGetTypeID() else { throw platform("keychain", 1) }
    return (value as! SecKey)
  }

  private static func create() throws -> SecKey {
    var privateAttrs: [String: Any] = [
      kSecAttrIsPermanent as String: true,
      kSecAttrApplicationTag as String: tag,
    ]
    var attrs: [String: Any] = [
      kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
      kSecAttrKeySizeInBits as String: 256,
    ]
    if SecureEnclave.isAvailable {
      var accessError: Unmanaged<CFError>?
      guard let access = SecAccessControlCreateWithFlags(
        kCFAllocatorDefault, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly, .privateKeyUsage, &accessError)
      else { throw platform("access", code(accessError)) }
      privateAttrs[kSecAttrAccessControl as String] = access
      attrs[kSecAttrTokenID as String] = kSecAttrTokenIDSecureEnclave
    } else {
      privateAttrs[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
    }
    attrs[kSecPrivateKeyAttrs as String] = privateAttrs
    var createError: Unmanaged<CFError>?
    guard let key = SecKeyCreateRandomKey(attrs as CFDictionary, &createError) else {
      throw platform("keygen", code(createError))
    }
    return key
  }

  // MARK: - Wrapping

  private static func wrap(_ data: Data) throws -> Data {
    let key = try lookup() ?? create()
    guard let pub = SecKeyCopyPublicKey(key), SecKeyIsAlgorithmSupported(pub, .encrypt, algorithm) else {
      throw platform("public", 0)
    }
    var encryptError: Unmanaged<CFError>?
    guard let wrapped = SecKeyCreateEncryptedData(pub, algorithm, data as CFData, &encryptError) as Data? else {
      throw platform("wrap", code(encryptError))
    }
    return wrapped
  }

  private static func unwrap(_ wrapped: Data) throws -> Data {
    guard let key = try lookup() else { throw KeyError(code: "PLUX_KEY_MISSING", message: "missing") }
    guard SecKeyIsAlgorithmSupported(key, .decrypt, algorithm) else { throw platform("algorithm", 0) }
    var decryptError: Unmanaged<CFError>?
    guard let data = SecKeyCreateDecryptedData(key, algorithm, wrapped as CFData, &decryptError) as Data? else {
      let status = code(decryptError)
      // A blob this key did not wrap fails to decode; any other failure, a
      // locked device for one, may pass, and the blob stays.
      if status == Int(errSecDecode) || status == Int(errSecParam) {
        throw KeyError(code: "PLUX_KEY_MISSING", message: "unwrap")
      }
      throw platform("unwrap", status)
    }
    return data
  }
}
