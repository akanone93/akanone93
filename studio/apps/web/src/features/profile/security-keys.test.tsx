// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { renderApp, type Upstream } from "../../test-harness.tsx";

/** A stand-in for the browser's WebAuthn: create() answers with fixed client data and attestation. */
class FakeAttestation {
  clientDataJSON = new TextEncoder().encode("client-data").buffer;
  attestationObject = new Uint8Array([1, 2, 3]).buffer;
}
class FakeCredential {
  response = new FakeAttestation();
  static seen: unknown[] = [];
  static parseCreationOptionsFromJSON(json: unknown) {
    FakeCredential.seen.push(json);
    return json;
  }
}
const globals = globalThis as Record<string, unknown>;
let created = 0;

beforeEach(() => {
  created = 0;
  FakeCredential.seen = [];
  globals["PublicKeyCredential"] = FakeCredential;
  globals["AuthenticatorAttestationResponse"] = FakeAttestation;
  Object.defineProperty(navigator, "credentials", {
    configurable: true,
    value: {
      create: async () => {
        created += 1;
        return new FakeCredential();
      },
    },
  });
});

afterEach(() => {
  cleanup();
  delete globals["PublicKeyCredential"];
  delete globals["AuthenticatorAttestationResponse"];
  Object.defineProperty(navigator, "credentials", { configurable: true, value: undefined });
});

const identity = (over: Partial<NonNullable<Upstream["identity"]>> = {}): NonNullable<Upstream["identity"]> => ({
  factors: [{ id: "f1", kind: "webauthn", label: "YubiKey", confirmed: true }],
  calls: [],
  ...over,
});

describe("[SEC-100] security keys on the profile", () => {
  test("a key is added with the browser's ceremony and only its client data and attestation are sent", async () => {
    const id = identity({ factors: [] });
    renderApp("/me", { identity: id });
    await screen.findByText(/No security key yet/);
    fireEvent.change(screen.getByLabelText("Name of the new key"), { target: { value: " Laptop " } });
    fireEvent.click(screen.getByRole("button", { name: "Add security key" }));
    await screen.findByText(/Security key added/);
    expect(created).toBe(1);
    expect(id.calls).toEqual(["begin:Laptop", "finish:f-new:client-data:3"]);
    expect((FakeCredential.seen[0] as { rp: { id: string } }).rp.id).toBe("localhost");
    const list = await screen.findByRole("list", { name: "Security keys" });
    await within(list).findByText("Laptop");
  });

  test("a refusal for a missing second factor says how to present one", async () => {
    const id = identity({ refuse: { register: "PLX-8011" } });
    renderApp("/me", { identity: id });
    await screen.findByText("YubiKey");
    fireEvent.click(screen.getByRole("button", { name: "Add security key" }));
    await screen.findByText(/Sign out and sign in again with one of your keys/);
    expect(created).toBe(0);
  });

  test("a key is removed only after the confirmation", async () => {
    const id = identity();
    renderApp("/me", { identity: id });
    fireEvent.click(await screen.findByRole("button", { name: "Remove YubiKey" }));
    await screen.findByText("Remove this security key?");
    expect(id.calls).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(id.calls).toEqual(["delete:f1"]));
    await screen.findByText(/No security key yet/);
  });

  test("without WebAuthn the browser is said to be unable", async () => {
    delete globals["PublicKeyCredential"];
    renderApp("/me", { identity: identity() });
    await screen.findByText("This browser cannot use security keys or passkeys.");
    expect(screen.queryByRole("button", { name: "Add security key" })).toBeNull();
  });
});
