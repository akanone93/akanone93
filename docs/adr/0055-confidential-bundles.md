# 0055. Confidential bundles: content keys wrapped to device keys, no deltas

- **Status:** Accepted (maintainer, 2026-10-07, P6 S0; P6 plan §2.1 B10)
- **Date:** 2026-10-07
- **Requirements:** `SEC-053`, `SEC-052`, `NFR-013`, `REL-020`

## Context and problem

Some customers must keep page content (texts, flows, business rules in PXL) confidential:
a CDN or a curious user must not be able to read it. `SEC-053` requires AES-256-GCM with a
per-release content key wrapped to a device-held hardware key-agreement key.

## Decision drivers

- Verify before decrypting (`SEC-052`); decrypt off the UI isolate (L-7).
- Only eligible devices receive the key; the key never leaves secure hardware wrapped.
- Decryption ≤ 10 ms per MiB p95 (`NFR-013`).

## Considered options

1. **Per-release content key, sections encrypted, key wrapped per device with ECDH-ES
   (P-256) + HKDF + AES-KW, no deltas.**
2. Encrypted deltas (patch over ciphertext or over plaintext then re-encrypt).
3. Transport encryption only.

## Decision

Chosen option: **1**.

- **Encryption.** At publish, the compiler's output is unchanged; the release step draws a
  random 256-bit content key per release and encrypts each section with AES-256-GCM (a
  unique nonce per section from a counter; the section hash and bundle ID as additional
  data). Hashes in `targets` cover the ciphertext, so verification precedes decryption.
  The content key is stored encrypted under KMS (`SEC-106`).
- **Key delivery.** A device registers a key-agreement public key (ECDH P-256 in secure
  hardware). For eligible devices (assurance ≥ the configured level) the manifest response,
  outside the signed part, carries the content key wrapped with ECDH-ES against that key,
  HKDF-SHA-256 bound to the device ID and release, and AES-KW. Ineligible devices get no
  key and do not sync confidential plugins (`PLX-6050`).
- **No deltas.** Ciphertext does not diff; a changed confidential bundle downloads whole.
  Encrypted deltas may come later with their own ADR.
- **On the device.** Bundles stay encrypted at rest; sections are decrypted into memory
  off the UI isolate, once per launch, on first use, with the platform's hardware AES
  (ADR-0058). Decrypted bytes are never written to disk.
- `confidentialBundles` (off / optional / required) is a profile setting.

## Consequences

- **Positive:** content is opaque to CDN and storage; revocation is per device.
- **Negative:** larger updates for confidential releases; the mmap zero-copy path of
  ADR-0030 does not apply to decrypted sections.
- **Follow-up:** S7, with the `NFR-013` benchmark; ADR-0003, ADR-0029, ADR-0030 revised.

## Options in detail

### Option 2: encrypted deltas

Requires either deterministic encryption (leaks equality) or plaintext patching on the
device, which needs the old plaintext on disk. Deferred.

### Option 3: transport encryption

TLS already exists; it does not protect content at the CDN or on the device.

## Revisions (2026-10-11)

Decided by the maintainer's lead during S7 (P6 plan §5.7); the decision above stands, with these refinements as built.

1. **Associated data.** A section's hash covers its ciphertext, so it cannot be part of the associated data, and the container has no bundle ID. The associated data of a sealed section is the container kind, the section kind (two bytes each, little endian) and the 16-byte section ID. A section sealed for one place does not open in another.
2. **The signed manifest.** For a confidential release the signed part names the sealed bundles: the hash and size are the ciphertext's, and the required features gain `security.confidential.v1`, so a runtime that cannot decrypt refuses the release before it downloads anything. The wrapped content key stays outside the signed part (`GetManifestResponse.content_key`); a key that does not open the signed ciphertext is refused by the device.
3. **Confidentiality is per release, honoured per environment.** A release is created as confidential (`CreateReleaseRequest.confidential`) or not. A release is built once and promoted between environments, so the environment a manifest is signed for decides at signing time, by `confidentialBundles`: `required` seals every release, `optional` the releases created as confidential, `off` none (and refuses to create one). The worker seals a copy of each bundle the first time it signs a manifest for the release; the published versions are untouched.
4. **Eligibility.** A new security setting, `confidentialMinAssurance` (AL0–AL3, default AL2 in every profile, tighter is higher, server-only), is the lowest assurance level a device must hold to receive the content key. A device below it, revoked, or with no registered key-agreement key gets no key.
5. **Errors.** `PLX-6050` covers a device that may not have the key and a key that does not open the content (wrapped for another device or release, altered, or unusable on the device). Section hash failures stay `PLX-3041` and are raised before anything is decrypted.
