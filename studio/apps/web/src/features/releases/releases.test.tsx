// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";

afterEach(cleanup);

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

const base = `/o/${orgA}/apps/app1/releases`;
const all = ["app.read", "rollout.manage", "release.promote"];
const ACTIVE = 2;
const PAUSED = 3;
const check = {
  decided: true,
  regressions: [{ metric: "crashFreeSessions", worseningBasisPoints: 80 }],
  candidate: { Sessions: 1000, Crashed: 40, Errors: 3 },
  previous: { Sessions: 2000, Crashed: 10, Errors: 1 },
};
const releases = [{ sequence: 2 }, { sequence: 1, notes: "first" }];
const rollout = (state: number) => ({
  id: "ro-1",
  candidate: 2,
  state,
  basisPoints: 1000,
  stages: [1000, 5000, 10000],
  lastCheck: check,
});

const open = (path: string, state: Upstream, permissions = all) =>
  renderApp(`${base}${path}`, { permissions, ...state });

describe("STU-101 releases timeline (A3)", () => {
  test("lists releases with the share of their rollout", async () => {
    open("", { releases: { releases, rollouts: [rollout(ACTIVE)] } });
    await screen.findByRole("link", { name: "2" });
    expect(screen.getByRole("link", { name: "1" })).toBeTruthy();
    expect(screen.getByText("10%")).toBeTruthy();
    expect(screen.getByText("first")).toBeTruthy();
  });

  test("shows the empty and the error state", async () => {
    open("", { releases: { releases: [] } });
    await screen.findByText("No releases yet.");
    cleanup();
    open("", { releases: { releases: [], failing: true } });
    await screen.findByRole("alert");
  });
});

describe("STU-101 release detail (A4) and rollout controls (D44)", () => {
  test("shows adoption, the health comparison and the regression", async () => {
    open("/2", { releases: { releases, rollouts: [rollout(ACTIVE)] } });
    await screen.findByRole("heading", { name: "Health against the previous release" });
    expect(screen.getByText("1000")).toBeTruthy();
    expect(screen.getByText("2000")).toBeTruthy();
    expect(screen.getByText("Regressed beyond the gate: crashFreeSessions")).toBeTruthy();
    expect(screen.getByText("Stage 1 of 3")).toBeTruthy();
  });

  test("pause, advance and reschedule act at once", async () => {
    const calls: string[] = [];
    open("/2", { releases: { releases, rollouts: [rollout(ACTIVE)], calls } });
    fireEvent.click(await screen.findByRole("button", { name: "Pause" }));
    fireEvent.click(screen.getByRole("button", { name: "Advance to next stage" }));
    fireEvent.click(screen.getByRole("button", { name: "No end" }));
    await waitFor(() => expect(calls).toEqual(["pause:ro-1", "advance:ro-1", "reschedule:ro-1:clear"]));
  });

  test("resume is offered only while paused", async () => {
    open("/2", { releases: { releases, rollouts: [rollout(PAUSED)] } });
    const resume = await screen.findByRole("button", { name: "Resume" });
    expect(resume.getAttribute("aria-disabled")).not.toBe("true");
    expect(screen.getByRole("button", { name: "Pause" }).getAttribute("aria-disabled")).toBe("true");
  });

  test("a person without rollout.manage cannot act", async () => {
    open("/2", { releases: { releases, rollouts: [rollout(ACTIVE)] } }, ["app.read"]);
    const pause = await screen.findByRole("button", { name: "Pause" });
    expect(pause.getAttribute("aria-disabled")).toBe("true");
  });

  test("aborting a rollout sends nothing before the step-up verifies (D45)", async () => {
    const credential = stubAssertion();
    const calls: string[] = [];
    const stepUps: string[] = [];
    open("/2", { releases: { releases, rollouts: [rollout(ACTIVE)], calls }, security: { stepUps } });
    fireEvent.click(await screen.findByRole("button", { name: "Abort rollout" }));
    await screen.findByRole("button", { name: "Use security key or passkey" });
    expect(calls).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(calls).toEqual(["abort:ro-1"]));
    expect(credential.get).toHaveBeenCalledTimes(1);
    expect(stepUps).toEqual(["begin", "complete:su-1"]);
  });

  test("rolling back asks for the step-up, then names the release (D45)", async () => {
    stubAssertion();
    const calls: string[] = [];
    open("/1", { releases: { releases, calls }, security: { stepUps: [] } });
    fireEvent.click(await screen.findByRole("button", { name: "Roll back to this release" }));
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(calls).toEqual(["rollback:1:production"]));
  });
});

describe("REL-060 kill switch (A5, D46)", () => {
  test("lists the plugins switched off and switches one on again after the step-up", async () => {
    stubAssertion();
    const calls: string[] = [];
    open("/kill-switch", { releases: { releases, killed: ["loans"], calls }, security: { stepUps: [] } });
    await screen.findByText("loans");
    fireEvent.click(screen.getByRole("button", { name: "Switch on loans" }));
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(calls).toEqual(["unkill:loans"]));
    await screen.findByText("No plugin is switched off.");
  });

  test("switching a plugin off is destructive and needs the step-up", async () => {
    stubAssertion();
    const calls: string[] = [];
    open("/kill-switch", { releases: { releases, calls }, security: { stepUps: [] } });
    fireEvent.change(await screen.findByLabelText("Plugin key"), { target: { value: "payments" } });
    fireEvent.click(screen.getByRole("button", { name: "Switch off" }));
    expect(calls).toEqual([]);
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(calls).toEqual(["kill:payments"]));
  });

  test("a person without release.promote cannot switch anything", async () => {
    open("/kill-switch", { releases: { releases, killed: ["loans"] } }, ["app.read"]);
    const clear = await screen.findByRole("button", { name: "Switch on loans" });
    expect(clear.getAttribute("aria-disabled")).toBe("true");
  });
});
