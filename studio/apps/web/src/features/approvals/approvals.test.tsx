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

const base = `/o/${orgA}/approvals`;
const decide = ["app.read", "approvals.decide", "audit.read", "policies.manage"];
const requests = [
  {
    id: "r1",
    appId: "app1",
    status: "open",
    requestedBy: "u2",
    summary: "Promote release 12",
    diff: '{"op":"add"}',
    risk: "high",
  },
  { id: "r2", appId: "app1", status: "open", requestedBy: "u1", summary: "Promote release 13" },
  {
    id: "r3",
    appId: "app1",
    status: "approved",
    requestedBy: "u2",
    summary: "Promote release 9",
    decisions: [{ id: "d1", decider: "Grace", decision: "approve", comment: "Looks right", auditSequence: 77 }],
  },
];

const open = (path: string, state: Upstream) => renderApp(`${base}${path}`, { permissions: decide, ...state });

describe("STU-102 inbox and history", () => {
  test("O7 lists the open requests and O9 the decided ones", async () => {
    open("", { approvals: { requests } });
    await screen.findByRole("link", { name: "Promote release 12" });
    expect(screen.queryByRole("link", { name: "Promote release 9" })).toBeNull();
    cleanup();
    open("/history", { approvals: { requests } });
    await screen.findByRole("link", { name: "Promote release 9" });
    expect(screen.queryByRole("link", { name: "Promote release 12" })).toBeNull();
  });

  test("O7 limits the list to the requests I made with ?mine", async () => {
    open("?mine=true", { approvals: { requests } });
    await screen.findByRole("link", { name: "Promote release 13" });
    expect(screen.queryByRole("link", { name: "Promote release 12" })).toBeNull();
  });

  test("O7 shows the empty and the error state", async () => {
    open("", { approvals: { requests: [] } });
    await screen.findByText("No approval requests match.");
    cleanup();
    open("", { approvals: { failing: true } });
    await screen.findByRole("alert");
  });

  test("O7 is refused without app.read", async () => {
    renderApp(base, { permissions: [], approvals: { requests } });
    await screen.findByText("You do not have access");
  });
});

describe("STU-102 request and decision", () => {
  test("O8 shows the evidence, the decisions and the audit record", async () => {
    open("/r3?app=app1", { approvals: { requests } });
    await screen.findByText("Looks right");
    expect(screen.getByText("Audit record 77")).toBeTruthy();
    expect(screen.getByText("Fix the checkout total")).toBeTruthy();
    cleanup();
    open("/r1?app=app1", { approvals: { requests } });
    await screen.findByText('{"op":"add"}');
  });

  test("D47 sends the decision with its comment only after the step-up (SEC-103)", async () => {
    const credential = stubAssertion();
    const decided: { appId: string; id: string; decision: string; comment: string }[] = [];
    const stepUps: string[] = [];
    open("/r1?app=app1", { security: { stepUps }, approvals: { requests, decided } });
    await screen.findByText('{"op":"add"}');
    fireEvent.change(screen.getByRole("textbox", { name: "Comment" }), { target: { value: "Reviewed the diff" } });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await screen.findByRole("button", { name: "Use security key or passkey" });
    expect(decided).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(decided.length).toBe(1));
    expect(credential.get).toHaveBeenCalledTimes(1);
    expect(stepUps).toEqual(["begin", "complete:su-1"]);
    expect(decided[0]).toEqual({ appId: "app1", id: "r1", decision: "approve", comment: "Reviewed the diff" });
  });

  test("D47 sends nothing when the step-up is cancelled", async () => {
    const credential = stubAssertion();
    const decided: { appId: string; id: string; decision: string; comment: string }[] = [];
    open("/r1?app=app1", { approvals: { requests, decided } });
    await screen.findByText('{"op":"add"}');
    fireEvent.click(screen.getByRole("button", { name: "Reject" }));
    await screen.findByRole("button", { name: "Use security key or passkey" });
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Use security key or passkey" })).toBeNull());
    expect(decided).toEqual([]);
    expect(credential.get).not.toHaveBeenCalled();
  });

  test("D47 shows the refusal of the server when the author decides their own request", async () => {
    stubAssertion();
    const message = "PLX-8003: you made this change, so you cannot approve it";
    open("/r2?app=app1", { approvals: { requests, refusal: message, decided: [] } });
    await screen.findByText("Promote release 13");
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("you cannot approve it");
  });

  test("D47 disables the decision without approvals.decide", async () => {
    renderApp(`${base}/r1?app=app1`, { permissions: ["app.read"], approvals: { requests } });
    await screen.findByText('{"op":"add"}');
    expect(screen.getByRole("button", { name: "Approve" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("button", { name: "Reject" }).getAttribute("aria-disabled")).toBe("true");
  });
});

describe("GOV-024 break-glass reviews", () => {
  const breakGlass = [
    {
      id: "b1",
      action: "killSwitch",
      subject: "function orders.create",
      invokedBy: "Ada",
      reviewStatus: "open",
      overdue: true,
    },
    { id: "b2", action: "rollback", subject: "release 7", invokedBy: "Grace", reviewStatus: "reviewed" },
  ];

  test("O10 lists the actions and flags the overdue review", async () => {
    open("/break-glass", { approvals: { breakGlass } });
    await screen.findByText("killSwitch: function orders.create");
    expect(screen.getByText("Overdue")).toBeTruthy();
    expect(screen.getByText("Reviewed")).toBeTruthy();
  });

  test("O10 reviews an action with a comment after the step-up", async () => {
    stubAssertion();
    const reviewed: { id: string; comment: string }[] = [];
    open("/break-glass", { approvals: { breakGlass, reviewed } });
    await screen.findByText("killSwitch: function orders.create");
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    fireEvent.change(await screen.findByRole("textbox", { name: "Review comment" }), {
      target: { value: "Justified" },
    });
    fireEvent.click(screen.getAllByRole("button", { name: "Review" }).at(-1) as HTMLElement);
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(reviewed).toEqual([{ id: "b1", comment: "Justified" }]));
  });

  test("O10 needs audit.read", async () => {
    renderApp(`${base}/break-glass`, { permissions: ["app.read"], approvals: { breakGlass } });
    await screen.findByText("You do not have access");
  });
});

describe("GOV-020 policies and the simulator", () => {
  const policies = [{ key: "mine", name: "My policy", approvers: 2 }];

  test("O11 lists the policies in force", async () => {
    open("/policies?app=app1", { approvals: { policies } });
    await screen.findByRole("link", { name: "mine" });
    expect(screen.getByText("2 approvals")).toBeTruthy();
  });

  test("O12 tries the draft against a sample change without saving it", async () => {
    const simulated: { document: string; resource: string; environment: string }[] = [];
    const saved: string[] = [];
    open("/policies/mine?app=app1", { approvals: { policies, simulated, saved } });
    await screen.findByRole("heading", { name: "Simulator" });
    fireEvent.change(screen.getByRole("spinbutton", { name: "Approvals needed" }), { target: { value: "3" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Environment key" }), { target: { value: "production" } });
    fireEvent.click(screen.getByRole("button", { name: "Simulate" }));
    await screen.findByText("The draft applies to this change.");
    expect(screen.getByText("3 approvals needed in all.")).toBeTruthy();
    expect(simulated.length).toBe(1);
    expect(simulated[0]?.environment).toBe("production");
    expect(JSON.parse(simulated[0]?.document ?? "{}").require.approvers).toBe(3);
    expect(saved).toEqual([]);
  });

  test("O12 saves the policy document", async () => {
    const saved: string[] = [];
    open("/policies/new?app=app1", { approvals: { policies, saved } });
    await screen.findByRole("heading", { name: "Simulator" });
    fireEvent.change(screen.getByRole("textbox", { name: "Key" }), { target: { value: "four-eyes" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Name" }), { target: { value: "Four eyes" } });
    fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
    await screen.findByText("Policy saved.");
    const doc = JSON.parse(saved[0] ?? "{}");
    expect(doc).toMatchObject({ kind: "policy", key: "four-eyes", name: "Four eyes", require: { approvers: 1 } });
  });

  test("O12 is read-only without policies.manage", async () => {
    renderApp(`${base}/policies/mine?app=app1`, { permissions: ["app.read"], approvals: { policies } });
    await screen.findByText("You need the permission to manage policies.");
    expect(screen.getByRole("button", { name: "Save policy" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("button", { name: "Simulate" }).getAttribute("aria-disabled")).toBe("true");
  });
});
