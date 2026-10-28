// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import { buildDocument, initialDraft, parseAttributes, problems } from "./experiments.schema.ts";

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
}

const base = `/o/${orgA}/apps/app1/experiments`;
const all = ["app.read", "experiments.manage", "flags.manage"];
const DRAFT = 1;
const RUNNING = 2;
const definition = {
  scope: "prop",
  variants: [
    { name: "control", weight: 50, control: true },
    { name: "bold", weight: 50 },
  ],
};
const experiments = [
  { id: "e1", key: "ctaLabel", state: RUNNING, definition },
  { id: "e2", key: "heroLayout", state: DRAFT, definition },
];

const open = (path: string, state: Upstream, permissions = all) =>
  renderApp(`${base}${path}`, { permissions, ...state });
const click = (name: string) => fireEvent.click(screen.getByRole("button", { name }));
const type = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } });

describe("STU-103 experiments list (A6)", () => {
  test("lists experiments with their state", async () => {
    open("", { experiments: { experiments } });
    await screen.findByRole("link", { name: "ctaLabel" });
    expect(screen.getByRole("link", { name: "heroLayout" })).toBeTruthy();
    expect(screen.getByText("Running")).toBeTruthy();
  });

  test("filters by ?status", async () => {
    open("?status=1", { experiments: { experiments } });
    await screen.findByRole("link", { name: "heroLayout" });
    expect(screen.queryByRole("link", { name: "ctaLabel" })).toBeNull();
  });

  test("shows the empty and the error state", async () => {
    open("", { experiments: { experiments: [] } });
    await screen.findByText("No experiments yet.");
    cleanup();
    open("", { experiments: { experiments: [], failing: true } });
    await screen.findByRole("alert");
  });
});

describe("ABT-001 new experiment wizard (A7)", () => {
  test("takes the canvas selection as its subject and creates the experiment", async () => {
    const calls: string[] = [];
    open("/new?target=n1,n2", { experiments: { experiments: [], calls } });
    expect(((await screen.findByLabelText("Subject of the experiment")) as HTMLInputElement).value).toBe("n1,n2");
    type("Experiment key", "ctaLabel");
    for (const step of ["Variants", "Allocation", "Targeting", "Metrics", "Schedule"]) {
      click("Next");
      await screen.findByRole("heading", { name: step });
    }
    expect(screen.getByRole("button", { name: "Create experiment" }).getAttribute("aria-disabled")).toBe("true");
    click("Back");
    fireEvent.change(await screen.findByLabelText("Primary metric event"), { target: { value: "purchase" } });
    click("Next");
    await screen.findByLabelText("Start");
    click("Create experiment");
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]).toContain("create:ctaLabel:");
    expect(calls[0]).toContain('"scope":"subtree"');
    expect(calls[0]).toContain('"weight":50');
  });

  test("blocks a draft whose shares do not add up to 100", async () => {
    open("/new?step=allocation", { experiments: { experiments: [] } });
    const shares = await screen.findAllByLabelText("Share in percent");
    fireEvent.change(shares[0] as HTMLElement, { target: { value: "70" } });
    for (const step of ["Targeting", "Metrics", "Schedule"]) {
      click("Next");
      await screen.findByRole("heading", { name: step });
    }
    expect(screen.getByText("The shares must add up to 100.")).toBeTruthy();
  });
});

describe("ABT-001 targeting and metrics of a draft", () => {
  test("reads attribute conditions and keeps versions as text", () => {
    expect(
      parseAttributes("os eq ios\nlocale in de, de-AT\nbuild gte 42\nosVersion gte 17.10\nuser.vip eq true"),
    ).toEqual([
      { attribute: "os", operator: "eq", value: "ios" },
      { attribute: "locale", operator: "in", value: ["de", "de-AT"] },
      { attribute: "build", operator: "gte", value: 42 },
      { attribute: "osVersion", operator: "gte", value: "17.10" },
      { attribute: "user.vip", operator: "eq", value: true },
    ]);
    expect(parseAttributes("os ios")).toBeUndefined();
    expect(parseAttributes("os like ios")).toBeUndefined();
    expect(parseAttributes("locale in de,")).toBeUndefined();
  });

  test("writes several segments, attributes, primary metrics and guardrails", () => {
    const draft = {
      ...initialDraft(undefined),
      key: "cta",
      event: "purchase",
      segments: "beta-testers, germans",
      attributes: "os eq ios",
      primaries: "signup",
      guardrails: "refund\ncrash",
    };
    expect(problems(draft)).toEqual([]);
    const doc = JSON.parse(new TextDecoder().decode(buildDocument(draft, "id1")));
    expect(doc.segment).toBe("beta-testers");
    expect(doc.segments).toEqual(["germans"]);
    expect(doc.attributes).toEqual([{ attribute: "os", operator: "eq", value: "ios" }]);
    expect(doc.primaryMetric.event).toBe("purchase");
    expect(doc.primaryMetrics.map((x: { event: string }) => x.event)).toEqual(["signup"]);
    expect(doc.guardrailMetrics.map((x: { event: string }) => x.event)).toEqual(["refund", "crash"]);
  });

  test("refuses a draft past the limits, a repeated segment and a malformed condition", () => {
    const draft = { ...initialDraft(undefined), key: "cta", event: "purchase" };
    const many = Array.from({ length: 11 }, (_, i) => `seg-${i}`).join(",");
    expect(problems({ ...draft, segments: many })).toContain("limits");
    expect(problems({ ...draft, segments: "a,a" })).toContain("targeting");
    expect(problems({ ...draft, attributes: "nonsense" })).toContain("targeting");
    expect(problems({ ...draft, primaries: "a\nb\nc" })).toContain("limits");
    expect(problems({ ...draft, guardrails: "Bad Name" })).toContain("metrics");
  });

  test("the wizard sends them in the document", async () => {
    const calls: string[] = [];
    open("/new", { experiments: { experiments: [], calls } });
    type(await screen.findByLabelText("Experiment key").then(() => "Experiment key"), "ctaLabel");
    click("Next");
    await screen.findByRole("heading", { name: "Variants" });
    click("Next");
    await screen.findByRole("heading", { name: "Allocation" });
    click("Next");
    await screen.findByRole("heading", { name: "Targeting" });
    type("Audience segments", "beta-testers, germans");
    type("Attribute conditions", "os eq ios");
    click("Next");
    await screen.findByRole("heading", { name: "Metrics" });
    type("Primary metric event", "purchase");
    type("More primary metric events", "signup");
    type("Guardrail events, one per line", "refund");
    click("Next");
    await screen.findByLabelText("Start");
    click("Create experiment");
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]).toContain('"segment":"beta-testers"');
    expect(calls[0]).toContain('"segments":["germans"]');
    expect(calls[0]).toContain('"attributes":[{"attribute":"os","operator":"eq","value":"ios"}]');
    expect(calls[0]).toContain('"primaryMetrics":[{"event":"signup"');
    expect(calls[0]).toContain('"guardrailMetrics":[{"event":"refund"');
  });
});

describe("ABT-003 experiment results and decision (A8, D48)", () => {
  test("shows the definition, the comparison and the verdict", async () => {
    open("/e1", { experiments: { experiments } });
    await screen.findByRole("heading", { name: "Experiment ctaLabel" });
    await screen.findByText("Improved");
    expect(screen.getByText("control (Control)")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Primary metric: purchase" })).toBeTruthy();
  });

  test("shows every primary metric", async () => {
    open("/e4", { experiments: { experiments: [{ id: "e4", key: "multi", state: RUNNING, definition }] } });
    await screen.findByRole("heading", { name: "Primary metric: purchase" });
    expect(screen.getByRole("heading", { name: "Primary metric: signup" })).toBeTruthy();
  });

  test("warns on a sample ratio mismatch", async () => {
    open("/e3", { experiments: { experiments: [{ id: "e3", key: "mismatch", state: RUNNING, definition }] } });
    await screen.findByText(/Sample ratio mismatch/);
  });

  test("stopping needs the step-up and sends nothing before it verifies", async () => {
    stubAssertion();
    const calls: string[] = [];
    open("/e1", { experiments: { experiments, calls }, security: { stepUps: [] } });
    click(await screen.findByRole("button", { name: "Stop experiment" }).then((b) => b.textContent ?? ""));
    await screen.findByRole("button", { name: "Use security key or passkey" });
    expect(calls).toEqual([]);
    click("Use security key or passkey");
    await waitFor(() => expect(calls).toEqual(["stop:e1"]));
  });

  test("shipping a variant names it and needs the step-up", async () => {
    stubAssertion();
    const calls: string[] = [];
    open("/e1", { experiments: { experiments, calls }, security: { stepUps: [] } });
    type(await screen.findByLabelText("Release label").then(() => "Release label"), "ship bold");
    click("Ship bold");
    click(await screen.findByRole("button", { name: "Use security key or passkey" }).then((b) => b.textContent ?? ""));
    await waitFor(() => expect(calls).toEqual(["ship:e1:bold:ship bold"]));
    await screen.findByText("Shipped as a publish job.");
  });

  test("a person without experiments.manage cannot decide", async () => {
    open("/e1", { experiments: { experiments } }, ["app.read"]);
    const stop = await screen.findByRole("button", { name: "Stop experiment" });
    expect(stop.getAttribute("aria-disabled")).toBe("true");
  });
});

describe("STU-103 feature flags (A9, D49)", () => {
  const flags = [{ key: "newCheckout", type: 1, defaultJson: "false" }];

  test("lists flags and edits the default of one", async () => {
    const calls: string[] = [];
    open("/flags", { experiments: { experiments: [], flags, calls } });
    await screen.findByText("newCheckout");
    click("Edit newCheckout");
    type("Default value (JSON)", "true");
    click("Save flag");
    await waitFor(() => expect(calls).toEqual(["updateFlag:newCheckout:true"]));
    await screen.findByText("true");
  });

  test("creates a flag and rejects invalid JSON", async () => {
    const calls: string[] = [];
    open("/flags", { experiments: { experiments: [], flags: [], calls } });
    await screen.findByText("No flags yet.");
    type("Flag key", "dark");
    type("Default value (JSON)", "{");
    await screen.findByText("The default value must be valid JSON.");
    expect(screen.getByRole("button", { name: "Save flag" }).getAttribute("aria-disabled")).toBe("true");
    type("Default value (JSON)", "true");
    click("Save flag");
    await waitFor(() => expect(calls).toEqual(["createFlag:dark:true"]));
  });

  test("keeps the values per segment of a flag and adds one", async () => {
    const calls: string[] = [];
    const withSegments = [
      { key: "newCheckout", type: 1, defaultJson: "false", segments: [{ segment: "germans", valueJson: "true" }] },
    ];
    open("/flags", { experiments: { experiments: [], flags: withSegments, calls } });
    await screen.findByText("germans: true");
    click("Edit newCheckout");
    expect((screen.getByLabelText("Segment key") as HTMLInputElement).value).toBe("germans");
    click("Add segment value");
    const keys = screen.getAllByLabelText("Segment key");
    fireEvent.change(keys[1] as HTMLElement, { target: { value: "beta-testers" } });
    fireEvent.change(screen.getAllByLabelText("Value for the segment (JSON)")[1] as HTMLElement, {
      target: { value: "false" },
    });
    click("Save flag");
    await waitFor(() =>
      expect(calls).toEqual(["updateFlag:newCheckout:false", "segments:newCheckout:germans=true,beta-testers=false"]),
    );
  });

  test("creates a flag with a value for a segment and blocks an invalid one", async () => {
    const calls: string[] = [];
    open("/flags", { experiments: { experiments: [], flags: [], calls } });
    await screen.findByText("No flags yet.");
    type("Flag key", "dark");
    click("Add segment value");
    expect(screen.getByRole("button", { name: "Save flag" }).getAttribute("aria-disabled")).toBe("true");
    type("Segment key", "germans");
    type("Value for the segment (JSON)", "{");
    expect(screen.getByRole("button", { name: "Save flag" }).getAttribute("aria-disabled")).toBe("true");
    type("Value for the segment (JSON)", "true");
    click("Save flag");
    await waitFor(() => expect(calls).toEqual(["createFlag:dark:false", "segments:dark:germans=true"]));
  });

  test("deletes a flag", async () => {
    const calls: string[] = [];
    open("/flags", { experiments: { experiments: [], flags, calls } });
    await screen.findByText("newCheckout");
    click("Delete newCheckout");
    await waitFor(() => expect(calls).toEqual(["deleteFlag:newCheckout"]));
  });

  test("a person without flags.manage cannot edit", async () => {
    open("/flags", { experiments: { experiments: [], flags } }, ["app.read"]);
    const edit = await screen.findByRole("button", { name: "Edit newCheckout" });
    expect(edit.getAttribute("aria-disabled")).toBe("true");
  });
});
