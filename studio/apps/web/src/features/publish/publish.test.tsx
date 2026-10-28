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

const summaries = [
  { id: "p1", key: "home", name: "Home" },
  { id: "p2", key: "shared", name: "Shared" },
];
const typeInto = (label: string, index: number, value: string) =>
  fireEvent.change(screen.getAllByLabelText(label)[index] as HTMLElement, { target: { value } });
const continueButton = () => screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement;
const isDisabled = (button: HTMLButtonElement) => button.disabled || button.getAttribute("aria-disabled") === "true";

describe("STU-100 rollout plan form (REL-050 to REL-053)", () => {
  test("the default plan is one stage at 100 % and goes to the server as typed stages", async () => {
    const release = fixture("rollout");
    open("/publish/r1", release);
    await step("Rollout plan");
    expect((screen.getAllByLabelText("Share of devices (%)")[0] as HTMLInputElement).value).toBe("100");
    press("Continue");
    await waitFor(() => expect(release.plan).toEqual({ stages: [{ basisPoints: 10000, durationSeconds: 0 }] }));
  });

  test("stages, schedule and health gate are sent in the fields the rollout service reads", async () => {
    const release = fixture("rollout");
    open("/publish/r1", release);
    await step("Rollout plan");
    typeInto("Share of devices (%)", 0, "5");
    typeInto("Duration (minutes)", 0, "90");
    press("Add stage");
    typeInto("Share of devices (%)", 1, "100");
    typeInto("Start (empty: at once)", 0, "2031-03-01T10:00");
    typeInto("End (empty: when complete)", 0, "2031-03-02T10:00");
    typeInto("Fewest sessions before a verdict", 0, "200");
    typeInto("Largest drop in crash-free sessions (basis points, 0: off)", 0, "150");
    press("Add custom gate");
    typeInto("Event name", 0, "checkout_failed");
    typeInto("Largest rise per session (basis points)", 0, "300");
    press("Continue");
    await waitFor(() => expect(release.plan !== undefined).toBe(true));
    expect(release.plan).toEqual({
      stages: [
        { basisPoints: 500, durationSeconds: 5400 },
        { basisPoints: 10000, durationSeconds: 0 },
      ],
      startAt: new Date("2031-03-01T10:00").toISOString(),
      endAt: new Date("2031-03-02T10:00").toISOString(),
      health: {
        minSample: 200,
        maxCrashFreeDropBasisPoints: 150,
        custom: [{ event: "checkout_failed", maxIncreaseBasisPoints: 300 }],
      },
    });
  });

  test("a stage outside the bounds of the rollout service blocks Continue and says why", async () => {
    const release = fixture("rollout");
    open("/publish/r1", release);
    await step("Rollout plan");
    typeInto("Share of devices (%)", 0, "0");
    await screen.findByText("Use a share from 0.01 to 100.");
    expect(isDisabled(continueButton())).toBe(true);
    typeInto("Share of devices (%)", 0, "50");
    typeInto("Duration (minutes)", 0, "30");
    await screen.findByText(
      "The last stage holds until the rollout ends unless it reaches 100 %. Give it no duration.",
    );
    expect(isDisabled(continueButton())).toBe(true);
    press("Add stage");
    typeInto("Share of devices (%)", 1, "20");
    await screen.findByText("A stage cannot lower the share of the stage before it.");
    typeInto("Share of devices (%)", 1, "100");
    typeInto("Duration (minutes)", 1, "0");
    await waitFor(() => expect(isDisabled(continueButton())).toBe(false));
    press("Remove stage 2");
    await screen.findByText(
      "The last stage holds until the rollout ends unless it reaches 100 %. Give it no duration.",
    );
    expect(release.calls).toEqual([]);
  });

  test("a schedule that ends before it starts, and a custom gate without a name, are refused", async () => {
    open("/publish/r1", fixture("rollout"));
    await step("Rollout plan");
    typeInto("Start (empty: at once)", 0, "2031-03-02T10:00");
    typeInto("End (empty: when complete)", 0, "2031-03-01T10:00");
    await screen.findByText("The end must come after the start.");
    press("Add custom gate");
    await screen.findByText("Name an event of up to 64 bytes.");
    expect(screen.getByText("Enter a threshold above zero.")).toBeTruthy();
    expect(isDisabled(continueButton())).toBe(true);
  });

  test("a stored plan fills the form and is read-only after the step", async () => {
    const plan = {
      stages: [
        { basisPoints: 1000, durationSeconds: 7200 },
        { basisPoints: 10000, durationSeconds: 0 },
      ],
      health: { windowSeconds: 3600, action: "HEALTH_ACTION_ROLLBACK", maxErrorRateIncreaseBasisPoints: 40 },
    };
    open("/publish/r1?step=rollout", fixture("release", { rolloutPlan: plan }));
    await step("Rollout plan");
    expect((screen.getAllByLabelText("Share of devices (%)")[0] as HTMLInputElement).value).toBe("10");
    expect((screen.getAllByLabelText("Duration (minutes)")[0] as HTMLInputElement).value).toBe("120");
    expect((screen.getByLabelText("Comparison window (minutes, 0: six hours)") as HTMLInputElement).value).toBe("60");
    expect((screen.getByLabelText("Largest rise in error rate (basis points, 0: off)") as HTMLInputElement).value).toBe(
      "40",
    );
    expect((screen.getAllByLabelText("Share of devices (%)")[0] as HTMLInputElement).readOnly).toBe(true);
    expect(screen.queryByRole("button", { name: "Add stage" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Continue" })).toBeNull();
  });
});

describe("STU-100 build evidence and diffs", () => {
  const evidence: NonNullable<ReleaseFixture["evidence"]> = {
    versions: {
      p1: [
        { version: 0, size: 1024, snapshot: "s0" },
        { version: 1, size: 3072, snapshot: "s1" },
      ],
      p2: [{ version: 1, size: 512, snapshot: "s2" }],
    },
    diagnostics: {
      "job-home": [
        { code: "PLX-1403", message: "Contrast below WCAG 2.2 AA", severity: 2 },
        { code: "PLX-2001", message: "Unrelated finding", severity: 2 },
      ],
    },
    changes: [
      {
        path: "pages/home.page.json",
        change: "changed",
        patch: [{ op: "replace", path: "/root/title", value: "Welcome" }],
      },
      { path: "pages/new.page.json", change: "added", patch: [] },
    ],
    screens: [{ screen: "main", before: "bb".repeat(32), after: "cc".repeat(32) }],
  };

  test("the reports step shows the size against the version before and the accessibility findings of each plugin", async () => {
    open("/publish/r1", fixture("reports", {}, { evidence }), { summaries });
    await step("Reports");
    await screen.findByText("3.0 KiB");
    expect(screen.getByText("+2.0 KiB against the version before")).toBeTruthy();
    expect(screen.getByText("0.5 KiB")).toBeTruthy();
    expect(screen.getByText("First version")).toBeTruthy();
    expect(screen.getByText("1 accessibility findings")).toBeTruthy();
    expect(screen.getByText("PLX-1403")).toBeTruthy();
    expect(screen.queryByText("Unrelated finding")).toBeNull();
    expect(screen.getByText("No accessibility findings")).toBeTruthy();
  });

  test("the diffs step shows the before and after of each screen and the JSON diff of the changed documents", async () => {
    open("/publish/r1", fixture("diffs", {}, { evidence, diff: { state: "succeeded" } }), { summaries });
    await step("Visual diffs");
    await screen.findByText("Image stored (bbbbbbbb)");
    expect(screen.getByText("Image stored (cccccccc)")).toBeTruthy();
    await screen.findByText("home: 2 changed documents");
    expect(screen.getByText("pages/home.page.json")).toBeTruthy();
    expect(screen.getByText("/root/title")).toBeTruthy();
    expect(screen.getByText('"Welcome"')).toBeTruthy();
    expect(screen.getByText("Added")).toBeTruthy();
    expect(screen.getByText(/First version: every document is new\./)).toBeTruthy();
  });

  test("a render that is still queued says that the thumbnails follow", async () => {
    open("/publish/r1", fixture("diffs", {}, { evidence, diff: { state: "queued" } }), { summaries });
    await step("Visual diffs");
    await screen.findByText("Thumbnails follow when the render finishes.");
    expect(screen.queryByRole("button", { name: "Continue" })).toBeNull();
  });

  test("a failure to read the document changes is reported and does not hide the screens", async () => {
    open(
      "/publish/r1",
      fixture("diffs", {}, { evidence: { ...evidence, failing: true }, diff: { state: "succeeded" } }),
      {
        summaries,
      },
    );
    await step("Visual diffs");
    await screen.findByText("The build details could not be loaded.");
    expect(screen.getByText("Image stored (cccccccc)")).toBeTruthy();
  });
});
