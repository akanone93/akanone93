// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { PhoneSignInState } from "@plux/api";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { renderApp } from "../../test-harness.tsx";

afterEach(cleanup);

/** Stands in for the browser's WebAuthn API: a platform that signs one assertion. */
function stubAssertion() {
  class StubResponse {
    clientDataJSON = new Uint8Array([1]).buffer;
    authenticatorData = new Uint8Array([2]).buffer;
    signature = new Uint8Array([3]).buffer;
  }
  class StubCredential {
    rawId = new Uint8Array([4]).buffer;
    response = new StubResponse();
    static parseRequestOptionsFromJSON(json: object) {
      return { ...json, challenge: new Uint8Array([1]) };
    }
  }
  const get = mock(async () => new StubCredential());
  Object.assign(globalThis, { PublicKeyCredential: StubCredential, AuthenticatorAssertionResponse: StubResponse });
  Object.defineProperty(navigator, "credentials", { value: { get }, configurable: true });
  return { get };
}

const slow = { timeout: 5000 };

describe("[DEV-002] phones on the profile (ADR-0104)", () => {
  test("connecting a phone asks for the security key, shows the code, then the number to pick, then the phone", async () => {
    stubAssertion();
    const stepUps: string[] = [];
    const phones = {
      phones: [] as { id: string; name: string; platform: string }[],
      calls: [] as string[],
      signIn: [
        { state: PhoneSignInState.WAITING },
        { state: PhoneSignInState.CLAIMED, number: 42, phoneName: "Pixel 8" },
        { state: PhoneSignInState.SIGNED_IN, phoneName: "Pixel 8" },
      ],
    };
    renderApp("/me", { phones, security: { stepUps } });
    fireEvent.click(await screen.findByRole("button", { name: "Connect a phone" }));
    // The security key comes first: nothing is created before it (SEC-103).
    expect(phones.calls).toEqual([]);
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(phones.calls).toEqual(["create"]));
    expect(stepUps).toEqual(["begin", "complete:su-1"]);
    await screen.findByLabelText("QR code that connects a phone to the Plux Dev app");
    const number = await screen.findByText("42", {}, slow);
    expect(number.tagName).toBe("OUTPUT");
    await screen.findByText(/Pixel 8 is connected/, {}, slow);
    expect(screen.queryByText("42")).toBeNull();
  });

  test("a wrong number says so and offers a new code", async () => {
    stubAssertion();
    const phones = {
      phones: [],
      calls: [] as string[],
      signIn: [{ state: PhoneSignInState.FAILED }],
    };
    renderApp("/me", { phones, security: { stepUps: [] } });
    fireEvent.click(await screen.findByRole("button", { name: "Connect a phone" }));
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await screen.findByText(/A wrong number was picked/, {}, slow);
    expect(screen.getByRole("button", { name: "Show a new code" })).toBeTruthy();
  });

  test("closing the dialog before the phone is connected cancels the sign-in", async () => {
    stubAssertion();
    const phones = { phones: [], calls: [] as string[], signIn: [{ state: PhoneSignInState.WAITING }] };
    renderApp("/me", { phones, security: { stepUps: [] } });
    fireEvent.click(await screen.findByRole("button", { name: "Connect a phone" }));
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await screen.findByLabelText("QR code that connects a phone to the Plux Dev app");
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(phones.calls).toEqual(["create", "cancel:si1"]));
  });

  test("a phone is signed out only after the confirmation", async () => {
    const phones = { phones: [{ id: "p1", name: "Pixel 8", platform: "android" }], calls: [] as string[], signIn: [] };
    renderApp("/me", { phones });
    const list = await screen.findByRole("list", { name: "Phones" });
    within(list).getByText("Pixel 8");
    fireEvent.click(screen.getByRole("button", { name: "Sign out Pixel 8" }));
    await screen.findByText("Sign this phone out?");
    expect(phones.calls).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(phones.calls).toEqual(["revoke:p1"]));
    await screen.findByText("No phone is signed in yet.");
  });
});
