// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** The server's challenge (WebAuthn JSON serialisation) as the options the step-up dialog D3 hands to the authenticator (SEC-103). */
export function requestOptions(optionsJson: string): CredentialRequestOptions {
  const json = JSON.parse(optionsJson) as PublicKeyCredentialRequestOptionsJSON;
  return { publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(json) };
}

/** The assertion fields CompleteStepUp verifies; anything but a WebAuthn assertion is refused. */
export function assertionFields(credential: Credential): {
  credentialId: Uint8Array;
  clientDataJson: Uint8Array;
  authenticatorData: Uint8Array;
  signature: Uint8Array;
} {
  if (
    !(credential instanceof PublicKeyCredential) ||
    !(credential.response instanceof AuthenticatorAssertionResponse)
  ) {
    throw new Error("no assertion");
  }
  return {
    credentialId: new Uint8Array(credential.rawId),
    clientDataJson: new Uint8Array(credential.response.clientDataJSON),
    authenticatorData: new Uint8Array(credential.response.authenticatorData),
    signature: new Uint8Array(credential.response.signature),
  };
}
