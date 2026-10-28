// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import type { ReleaseFixture } from "../../test-upstream.ts";

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

const base = `/o/${orgA}/apps/app1`;
const permissions = ["app.read", "release.publish"];
const plugins = [
  { pluginKey: "home", reason: "changed" },
  { pluginKey: "shared", reason: "dependency" },
];

function fixture(state: string, extra: Partial<ReleaseFixture["request"]> = {}, rest: Partial<ReleaseFixture> = {}) {
  return { request: { id: "r1", state, plugins, ...extra }, calls: [], keys: [], ...rest } satisfies ReleaseFixture;
}

const open = (path: string, release: ReleaseFixture, more: Upstream = {}) =>
  renderApp(`${base}${path}`, { permissions, release, summaries: [], ...more });

const step = (name: string) => screen.findByRole("heading", { name }, { timeout: 5000 });
const press = (name: string) => fireEvent.click(screen.getByRole("button", { name }));

describe("STU-100 publish flow", () => {
  test("A2 walks the request from selection to release, with a step-up before the release", async () => {
    const credential = stubAssertion();
    const release = fixture("select");
    const stepUps: string[] = [];
    open("/publish/r1", release, {
      summaries: [
        { id: "p1", key: "home", name: "Home" },
        { id: "p2", key: "shared", name: "Shared" },
      ],
      security: { stepUps },
    });
    await step("Select plugins");
    expect(screen.getByText("Added because another plugin needs it")).toBeTruthy();
    press("Continue");
    await step("Freeze and build");
    press("Freeze and build");
    await waitFor(() => expect(release.keys).toEqual(["r1:home:1", "r1:shared:1"]));
    await waitFor(() => expect(screen.getByRole("button", { name: "Continue" })).toBeTruthy());
    press("Continue");
    await step("Reports");
    await screen.findByText("No findings", {}, { timeout: 3000 }).catch(() => undefined);
    press("Continue");
    await step("Visual diffs");
    press("Render visual diff");
    await screen.findByText("Rendered");
    press("Continue");
    await step("Changelog");
    press("Continue");
    await step("Target");
    press("Continue");
    await step("Approval");
    press("Request approval");
    await step("Rollout plan");
    press("Continue");
    await step("Release");
    press("Release");
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await screen.findByText("Released as release 7.");
    expect(credential.get).toHaveBeenCalledTimes(1);
    expect(stepUps).toEqual(["begin", "complete:su-1"]);
    expect(release.calls).toEqual([
      "advance:select",
      "advance:build",
      "advance:reports",
      "diff",
      "advance:diffs",
      "advance:changelog",
      "advance:target",
      "approval",
      "advance:rollout",
      "complete",
    ]);
  });

  test("A2 resumes at the server state after a reload and shows earlier steps read-only", async () => {
    const release = fixture("approval", {}, { diff: { state: "succeeded" } });
    open("/publish/r1", release);
    await step("Approval");
    expect(screen.getByRole("button", { name: "3. Reports" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "8. Rollout plan" }).getAttribute("aria-disabled")).toBe("true");
    cleanup();
    open("/publish/r1?step=select", release);
    await step("Select plugins");
    expect(screen.queryByRole("button", { name: "Continue" })).toBeNull();
  });

  test("A2 shows an approval that was rejected and the code of a step that cannot run (PLX-8062)", async () => {
    const release = fixture("approval", {}, { rejectApproval: true });
    open("/publish/r1", release);
    await step("Approval");
    press("Request approval");
    await screen.findByText("The visual diff has not finished for this content. Request it and wait.");
    cleanup();
    open("/publish/r1", fixture("approval", {}, { rejectApproval: true, diff: { state: "succeeded" } }));
    await step("Approval");
    press("Request approval");
    await screen.findByText("Rejected");
  });

  test("A2 refuses a stale step with PLX-8060 and reloads the request", async () => {
    const release = fixture("select");
    open("/publish/r1", release);
    await step("Select plugins");
    release.request.state = "build";
    press("Continue");
    await screen.findByText("The request was at another step. It has been reloaded.");
    await step("Freeze and build");
  });

  test("D42 acknowledges the warnings one by one before the reports step passes", async () => {
    const release = fixture("reports", { warnings: ["w-size", "w-label"] });
    open("/publish/r1", release);
    await step("Reports");
    press("Acknowledge warnings");
    const dialog = await screen.findByRole("dialog");
    const confirm = within(dialog).getByRole("button", { name: "Acknowledge" }) as HTMLButtonElement;
    expect(confirm.getAttribute("aria-disabled") === "true" || confirm.disabled).toBe(true);
    fireEvent.click(within(dialog).getByRole("button", { name: "w-size" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "w-label" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Acknowledge" }));
    await waitFor(() => expect(release.request.acknowledged).toEqual(["w-size", "w-label"]));
    await step("Visual diffs");
  });

  test("D3 a cancelled step-up sends nothing", async () => {
    stubAssertion();
    const release = fixture("release");
    const stepUps: string[] = [];
    open("/publish/r1", release, { security: { stepUps } });
    await step("Release");
    press("Release");
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    expect(release.calls).toEqual([]);
    expect(stepUps).toEqual(["begin"]);
  });

  test("D43 cancels the request after confirming", async () => {
    const release = fixture("build");
    open("/publish/r1", release);
    await step("Freeze and build");
    press("Cancel request");
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel request" }));
    await screen.findByText("Cancelled");
    expect(release.calls).toEqual(["cancel"]);
  });

  test("someone else's request is read-only and names the owner", async () => {
    open("/publish/r1", fixture("select", { createdBy: "u2", createdByName: "Anna" }));
    await screen.findByText("Anna is publishing. You can view this request.");
    expect(screen.queryByRole("button", { name: "Continue" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Cancel request" })).toBeNull();
  });

  test("A1 opens a request with the plugins that changed and goes to the flow", async () => {
    const release = fixture("select");
    open("/publish", release, {
      summaries: [
        { id: "p1", key: "home", name: "Home", unpublishedChanges: true },
        { id: "p2", key: "other", name: "Other" },
      ],
    });
    await step("Select plugins");
    expect(release.calls).toEqual(["create:home"]);
  });

  test("A1 says so when nothing changed and nothing is open", async () => {
    open("/publish", fixture("released"));
    await screen.findByText("No plugin has unpublished changes.");
  });
});
