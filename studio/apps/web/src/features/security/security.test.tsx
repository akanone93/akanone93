// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";

afterEach(cleanup);

interface ProfileCall {
  profile: string;
  environmentId: string;
  expectedVersion: bigint;
  overrides: string;
}

/** Stands in for the browser's WebAuthn API: a platform that parses the server's options and signs one assertion. */
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

const base = `/o/${orgA}/apps/app1`;
const manage = ["app.read", "app.manage"];
const devices = [
  { id: "dev-1", platform: "android", hostBuild: "1.2.0", assuranceLevel: "AL2" },
  { id: "dev-2", platform: "ios", hostBuild: "1.3.0", assuranceLevel: "AL3", revoked: true },
];

const open = (path: string, state: Upstream) => renderApp(`${base}${path}`, { permissions: manage, ...state });

describe("STU-115 devices", () => {
  test("A31 lists devices and filters by ?q", async () => {
    open("/devices?q=ios", { security: { devices } });
    await screen.findByRole("link", { name: "dev-2" });
    expect(screen.queryByRole("link", { name: "dev-1" })).toBeNull();
  });

  test("A31 shows the empty and the error state", async () => {
    open("/devices", { security: { devices: [] } });
    await screen.findByText("No devices match.");
    cleanup();
    open("/devices", { security: { failing: true } });
    await screen.findByRole("alert");
  });

  test("A32 revokes a device with a reason that is sent to the server (D62)", async () => {
    const revoked: { deviceId: string; reason: string }[] = [];
    open("/devices/dev-1", { security: { devices, revoked } });
    fireEvent.click(await screen.findByRole("button", { name: "Revoke device" }));
    fireEvent.change(await screen.findByRole("textbox"), { target: { value: "Lost phone, reported by the owner" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(revoked).toEqual([{ deviceId: "dev-1", reason: "Lost phone, reported by the owner" }]));
  });

  test("A32 disables revoking without app.manage", async () => {
    renderApp(`${base}/devices/dev-1`, { permissions: ["app.read"], security: { devices } });
    const button = await screen.findByRole("button", { name: "Revoke device" });
    expect(button.getAttribute("aria-disabled")).toBe("true");
  });
});

describe("SEC-180 security profile", () => {
  test("A33 shows the effective configuration", async () => {
    open("/security", { security: { profile: "standard", version: 3, settings: { accessTokenLifetime: 900 } } });
    await screen.findByText("accessTokenLifetime");
    expect(screen.getByText("900")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Change profile" }).getAttribute("aria-disabled")).toBe("true");
  });

  test("A33 sends the profile change only after the step-up, with the stored overrides (D66, D3, SEC-103)", async () => {
    const credential = stubAssertion();
    const profiles: ProfileCall[] = [];
    const stepUps: string[] = [];
    open("/security", {
      security: {
        profile: "standard",
        version: 3,
        settings: { accessTokenLifetime: 900 },
        overrides: { inactivityLockTimeout: 60 },
        profiles,
        stepUps,
      },
    });
    await screen.findByText("accessTokenLifetime");
    fireEvent.click(screen.getByRole("button", { name: "Strict" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Change profile" }).getAttribute("aria-disabled")).not.toBe("true"),
    );
    fireEvent.click(screen.getByRole("button", { name: "Change profile" }));
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(profiles.length).toBe(1));
    expect(credential.get).toHaveBeenCalledTimes(1);
    expect(stepUps).toEqual(["begin", "complete:su-1"]);
    expect(profiles[0]).toEqual({
      profile: "strict",
      environmentId: "env-dev",
      expectedVersion: 3n,
      overrides: '{"inactivityLockTimeout":60}',
    });
  });

  test("A33 sends nothing before the step-up succeeds or when it is cancelled", async () => {
    const credential = stubAssertion();
    const profiles: ProfileCall[] = [];
    const stepUps: string[] = [];
    open("/security", { security: { profile: "standard", version: 3, profiles, stepUps } });
    fireEvent.click(await screen.findByRole("button", { name: "Maximum" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Change profile" }).getAttribute("aria-disabled")).not.toBe("true"),
    );
    fireEvent.click(screen.getByRole("button", { name: "Change profile" }));
    await screen.findByRole("button", { name: "Use security key or passkey" });
    expect(profiles).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Use security key or passkey" }) === null).toBe(true),
    );
    expect(profiles).toEqual([]);
    expect(credential.get).not.toHaveBeenCalled();
    expect(stepUps).toEqual(["begin"]);
  });

  test("A33 shows the error state", async () => {
    open("/security", { security: { failing: true } });
    await screen.findByRole("alert");
  });
});

describe("STU-115 keys, pins and grants", () => {
  const keys = [
    { keyId: "k1", role: "targets", algorithm: "ed25519", environmentType: "production" },
    { keyId: "p1", role: "pin", algorithm: "ed25519", environmentType: "production" },
  ];

  test("A34 lists the keys of the root metadata read-only with the rotation state and the runbook link", async () => {
    open("/security/keys", { security: { keys, roots: 2 } });
    await screen.findByText("k1");
    expect(screen.getByText("p1")).toBeTruthy();
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Key ceremony runbook" }).getAttribute("href")).toContain(
      "key-ceremony.md",
    );
    expect(within(screen.getByRole("region", { name: "Signing keys" })).queryByRole("button")).toBeNull();
  });

  test("A35 lists no pins and points to the signed root metadata and the runbook", async () => {
    open("/security/pins", { security: { keys, roots: 1 } });
    await screen.findByText(/part of the signed root metadata/);
    expect(screen.queryByText("p1")).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.getByRole("link", { name: "Key ceremony runbook" }).getAttribute("href")).toContain(
      "key-ceremony.md",
    );
  });

  test("A36 edits the grants through the app document (D65)", async () => {
    const putDocuments: { path: string; content: string; ifRevision: bigint }[] = [];
    open("/security/capabilities", {
      security: { putDocuments },
      appDocuments: [
        { kind: "app", path: "app.json", json: { id: "app1", capabilities: { networkDomains: ["a.example"] } } },
      ],
    });
    await screen.findByText("a.example");
    fireEvent.click(screen.getByRole("button", { name: "Edit grants" }));
    const boxes = await screen.findAllByRole("textbox");
    fireEvent.change(boxes[1] as HTMLElement, { target: { value: "a.example\nb.example" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(putDocuments.length).toBe(1));
    expect(JSON.parse(putDocuments[0]?.content ?? "{}").capabilities.networkDomains).toEqual([
      "a.example",
      "b.example",
    ]);
  });

  test("A36 disables editing without app.manage", async () => {
    renderApp(`${base}/security/capabilities`, {
      permissions: ["app.read"],
      appDocuments: [{ kind: "app", path: "app.json", json: { id: "app1" } }],
    });
    const button = await screen.findByRole("button", { name: "Edit grants" });
    expect(button.getAttribute("aria-disabled")).toBe("true");
  });
});
