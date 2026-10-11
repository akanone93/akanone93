// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import CryptoKit
import Foundation

/// AES-256-GCM by CryptoKit (SEC-073, ADR-0058), for the encrypted stores and
/// confidential bundles. `plux_native` (native/plux_native.c) finds
/// `plux_swift_aead` with `dlsym` and calls it once per record, on whatever
/// isolate thread asks, so it keeps no state and takes only buffers.
///
/// Results follow `plux_aead`: the bytes written, or a negative number —
/// -2 when the platform fails or an argument is wrong, -3 when the tag does not
/// verify, -4 when `outCapacity` is too small.
@_cdecl("plux_swift_aead")
public func pluxSwiftAead(
  _ mode: Int32,
  _ key: UnsafePointer<UInt8>?,
  _ nonce: UnsafePointer<UInt8>?,
  _ aad: UnsafePointer<UInt8>?,
  _ aadLength: UInt64,
  _ input: UnsafePointer<UInt8>?,
  _ inputLength: UInt64,
  _ output: UnsafeMutablePointer<UInt8>?,
  _ outCapacity: UInt64
) -> Int64 {
  let tagLength = 16
  guard let key, let nonce, let output,
        aadLength <= Int32.max, inputLength <= Int32.max,
        aad != nil || aadLength == 0, input != nil || inputLength == 0
  else { return -2 }
  let symmetric = SymmetricKey(data: Data(bytes: key, count: 32))
  let iv: AES.GCM.Nonce
  do { iv = try AES.GCM.Nonce(data: Data(bytes: nonce, count: 12)) } catch { return -2 }
  let associated = aad.map { Data(bytes: $0, count: Int(aadLength)) } ?? Data()
  let message = input.map { Data(bytes: $0, count: Int(inputLength)) } ?? Data()
  do {
    let result: Data
    if mode == 0 {
      let box = try AES.GCM.seal(message, using: symmetric, nonce: iv, authenticating: associated)
      result = box.ciphertext + box.tag
    } else if mode == 1 {
      guard message.count >= tagLength else { return -3 }
      let split = message.count - tagLength
      let box = try AES.GCM.SealedBox(
        nonce: iv, ciphertext: message.prefix(split), tag: message.suffix(tagLength))
      result = try AES.GCM.open(box, using: symmetric, authenticating: associated)
    } else {
      return -2
    }
    guard UInt64(result.count) <= outCapacity else { return -4 }
    result.copyBytes(to: output, count: result.count)
    return Int64(result.count)
  } catch CryptoKitError.authenticationFailure {
    return -3
  } catch {
    return -2
  }
}

/// Keeps `plux_swift_aead` linked into the app: nothing in Swift calls it, so
/// without a reference the linker may drop it as dead code.
enum PluxAead {
  static func retain() {
    let entry: @convention(c) (
      Int32, UnsafePointer<UInt8>?, UnsafePointer<UInt8>?, UnsafePointer<UInt8>?, UInt64,
      UnsafePointer<UInt8>?, UInt64, UnsafeMutablePointer<UInt8>?, UInt64
    ) -> Int64 = pluxSwiftAead
    withExtendedLifetime(entry) {}
  }
}
