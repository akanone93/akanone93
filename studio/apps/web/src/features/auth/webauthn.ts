// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CompleteWebAuthnLoginRequest } from "@plux/api";

/** Runs navigator.credentials.get() for the server's options and returns the assertion fields. */
export async function getAssertion(
  optionsJson: Uint8Array,
  challengeId: string,
): Promise<Pick<CompleteWebAuthnLoginRequest, "challengeId" | "credentialId" | "clientDataJson" | "authenticatorData" | "signature">> {
  const json: unknown = JSON.parse(new TextDecoder().decode(optionsJson));
  const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(json as PublicKeyCredentialRequestOptionsJSON);
  const credential = await navigator.credentials.get({ publicKey });
  if (!(credential instanceof PublicKeyCredential) || !(credential.response instanceof AuthenticatorAssertionResponse)) {
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
