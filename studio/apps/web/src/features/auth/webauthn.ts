// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CompleteWebAuthnLoginRequest } from "@plux/api";

/** Runs navigator.credentials.get() for the server's options and returns the assertion fields. */
export async function getAssertion(
  optionsJson: Uint8Array,
  challengeId: string,
): Promise<
  Pick<
    CompleteWebAuthnLoginRequest,
    "challengeId" | "credentialId" | "clientDataJson" | "authenticatorData" | "signature"
  >
> {
  const json: unknown = JSON.parse(new TextDecoder().decode(optionsJson));
  const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(json as PublicKeyCredentialRequestOptionsJSON);
  const credential = await navigator.credentials.get({ publicKey });
  if (
    !(credential instanceof PublicKeyCredential) ||
    !(credential.response instanceof AuthenticatorAssertionResponse)
  ) {
    throw new Error("no assertion");
  }
  return {
    challengeId,
    credentialId: new Uint8Array(credential.rawId),
    clientDataJson: new Uint8Array(credential.response.clientDataJSON),
    authenticatorData: new Uint8Array(credential.response.authenticatorData),
    signature: new Uint8Array(credential.response.signature),
  };
}

/** Whether this browser can create and use security keys and passkeys. */
export function webAuthnAvailable(): boolean {
  return typeof PublicKeyCredential !== "undefined" && typeof navigator.credentials?.create === "function";
}

/**
 * Runs navigator.credentials.create() for the server's creation options (BeginWebAuthnRegistration) and returns what
 * FinishWebAuthnRegistration verifies: the client data and the attestation object, nothing else (SEC-100).
 */
export async function createCredential(
  optionsJson: Uint8Array,
): Promise<{ clientDataJson: Uint8Array; attestationObject: Uint8Array }> {
  const json: unknown = JSON.parse(new TextDecoder().decode(optionsJson));
  const publicKey = PublicKeyCredential.parseCreationOptionsFromJSON(json as PublicKeyCredentialCreationOptionsJSON);
  const credential = await navigator.credentials.create({ publicKey });
  if (
    !(credential instanceof PublicKeyCredential) ||
    !(credential.response instanceof AuthenticatorAttestationResponse)
  ) {
    throw new Error("no attestation");
  }
  return {
    clientDataJson: new Uint8Array(credential.response.clientDataJSON),
    attestationObject: new Uint8Array(credential.response.attestationObject),
  };
}
