# 0058. Encryption at rest and native AES-GCM

- **Status:** Accepted (maintainer, 2026-10-07, P6 S0; P6 plan §2.1 B15, supersedes P5's B6 for `strict` and `maximum`)
- **Date:** 2026-10-07
- **Requirements:** `SEC-072`, `SEC-073`, `SEC-053`, `NFR-013`, `DB-002`

## Context and problem

Under `strict` and `maximum`, the database, response cache, outbox, `persisted` state and
key-value store are encrypted. P5 measured pure-Dart AES-GCM (`DartAesGcm`) at ten times
slower than native, too slow for every read and write.

## Decision drivers

- Within 10% of the plain stores in the P5 benchmarks (`NFR-013`).
- Keys never on disk unwrapped; unwrapped once per launch.
- No new third-party crypto unless measurement demands it.

## Considered options

1. **Platform crypto through FFI or platform channels: CryptoKit `AES.GCM` on iOS,
   Conscrypt/`javax.crypto` (ARMv8 AES instructions) on Android.**
2. A vendored, audited C library (e.g. BoringSSL's AES-GCM) compiled by the build hook.
3. Keep pure Dart.

## Decision

Chosen option: **decided by measurement in S7 between 1 and 2, defaulting to 1.** Option 1
adds nothing to the app; option 2 is chosen only if the call overhead of 1 misses the
budget for small records. Option 3 is ruled out by P5's measurement.

- **Keys.** One 256-bit data key per store, generated on the device, wrapped by a secure
  hardware key (Keystore AES key / Secure Enclave ECIES) and stored wrapped; unwrapped once
  per launch and kept in memory, never per read or write. Under `standard` with
  `encryptLocalStores` off, nothing changes (ADR-0049's per-profile wrapping is revised).
- **Format.** Each record: version byte, 12-byte random nonce, ciphertext, tag; the store
  and record key as additional data. SQLCipher stays for `plux_db_drift` (`DB-002`) with
  its key from the same wrap.
- **Off the UI isolate** for bulk work (L-7).

## Consequences

- **Positive:** hardware AES speed; nothing new to ship (option 1).
- **Negative:** two platform implementations; FFI and channel overhead to measure.
- **Follow-up:** S7 measures and records the result in this ADR; then Accepted.

## Revisions

### Revision (2026-10-11, P6 S7a): what was built, and what is left to measure

- **Option 1, through one synchronous FFI call.** Platform channels cannot answer
  synchronously and stores seal on background isolates, so `plux_native` exports
  `plux_aead` (`native/plux_native.c`). On Android it calls `javax.crypto`
  `AES/GCM/NoPadding` through JNI (`JNI_OnLoad` keeps the Java VM; the plugin loads the
  library with `System.loadLibrary`); on iOS it finds `plux_swift_aead`, a CryptoKit
  `AES.GCM` function the plugin exports (`PluxAead.swift`), with `dlsym`. Elsewhere it answers
  "unsupported" and Dart's `DartAead` runs. `PlatformAead.tryCreate` is trusted only after
  it seals a fixed input to the same bytes as `DartAead`, opens them, and refuses an altered
  record; `platformAead` is the result, decided once per isolate. The JNI function was run
  here against a real JVM (`-Xcheck:jni`, attached and detached threads, tampered and empty
  records, a tag compared with Python's AES-GCM); the Kotlin, Swift and the
  Android/iOS builds were not, and run in CI and on devices.
- **Measurement left to the maintainer.** `test/security/aead_bench_test.dart`
  (`PLUX_BENCH_AEAD=1`) prints the per-record cost of seal and open through `RecordCipher`
  at 64 B, 1 KiB and 64 KiB. On the Linux CI host `DartAead` took about 11 µs, 67 µs and
  7.1 ms (median, JIT). The JNI path measured on that host against SunJCE (not Conscrypt, not
  a phone) took about 38 µs, 23 µs and 103 µs per call: at 64 B the call overhead is above
  the pure-Dart cost, at 64 KiB the native path is about seventy times faster. The device
  numbers decide between the three options; if small records must be as fast as bulk,
  a size threshold below which `DartAead` runs is a one-line change in `PlatformAead`, and
  option 2 (a vendored C library) removes the JNI overhead.
- **Data keys.** One 256-bit key per store (`dk-outbox.<app>.<env>`, `dk-persisted…`,
  `dk-secure…`, `dk-kv…`, and the cache's `data-cache…`), made on the device. A
  `HardwareWrappedSecretStore` keeps them in the platform's secret storage only as wrapped
  bytes (`hw1.` and base64) and unwraps each once per launch. The platform wraps with
  `dataKeyWrap` and `dataKeyUnwrap` on `dev.plux/runtime`: an Android Keystore AES-256-GCM
  key, or a Secure Enclave P-256 key with ECIES
  (`eciesEncryptionCofactorX963SHA256AESGCM`), a Keychain key where there is no Secure
  Enclave. The wrapped bytes also bind the secret's name. A key stored unwrapped by an
  earlier launch is wrapped in place on its first read, so the SQLCipher key of
  `plux_db_drift` (which takes the same wrap when the profile requires encryption)
  survives the change; a key whose hardware key is gone reads as absent, and the records it
  sealed are discarded and reported, as ADR-0049 says.
- **Stores and migration.** `encryptLocalStores` is read once at start from the stored
  security configuration; a change reaches the stores on the next launch. Then the outbox,
  `persisted` and `secure` state, the key-value store and the whole response cache (not only
  `encrypted` sources) are ADR-0058 records. A file a `standard` launch wrote — plain JSON,
  or the `PXK1` file under a software-wrapped key — is read and rewritten in place (new
  file, flush, rename, directory sync) on the first load; the old key is removed after the
  rename. Cache entries are sealed the same way, one by one; entries of the earlier
  encrypted format are dropped, as a cache may. Loosening the profile again is not
  supported: the standard stores cannot read records and discard them.
- **Cache index.** A cache entry's file keeps its 8-byte store time in the clear so the
  index reads it without decrypting; the time is not authenticated.
