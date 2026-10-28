// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { DevEventSchema } from "@plux/api";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import type { EditSession } from "../editor/index.ts";

afterEach(cleanup);

const base = `/o/${orgA}/apps/app1/tests`;
const scenarios = {
  schemaVersion: "1.0.0",
  kind: "scenarios",
  scenarios: [
    { name: "Pays", page: "checkout", steps: [{ tap: "pay" }], expect: [{ visible: "done" }] },
    { name: "Empty cart", page: "cart", expect: [{ visible: "empty" }] },
  ],
};
const appDocuments = [{ kind: "scenarios", path: "tests/checkout.scenario.json", json: scenarios }];
const testRuns = {
  runs: [
    {
      id: "r1",
      label: "main",
      scenarios: [
        {
          id: "pays",
          name: "Pays",
          status: "failed",
          durationMs: 40,
          steps: [
            { description: "tap pay", status: "passed" },
            { description: "expect text Paid", status: "failed", message: "found Failed" },
          ],
        },
        { id: "cart", name: "Empty cart", status: "passed" },
      ],
    },
  ],
};
const open = (path: string, state: Upstream, permissions = ["app.read", "plugin.edit"]) =>
  renderApp(`${base}${path}`, { permissions, appDocuments, testRuns, ...state });
const disabled = (el: HTMLElement) => el.matches(":disabled, [aria-disabled=true]");
const wait = { timeout: 5000 };
const type = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } });

describe("TST-001 Plux Test scenarios (A45)", () => {
  test("lists the scenarios and the runs, and filters by ?q", async () => {
    open("?q=cart", {});
    await screen.findByText("Empty cart", {}, wait);
    expect(screen.queryByText("Pays")).toBeNull();
    await screen.findByRole("link", { name: "main" }, wait);
  });

  test("shows the empty states", async () => {
    open("", { appDocuments: [], testRuns: { runs: [] } });
    await screen.findByText("No scenarios yet.", {}, wait);
    await screen.findByText("No runs reported yet.", {}, wait);
  });

  test("appends a scenario to a file through the editing core, and removes one", async () => {
    let edit: EditSession | undefined;
    const sent: NonNullable<Upstream["sent"]> = [];
    open("", { sent, editor: (e) => (edit = e) });
    await screen.findByText("Pays", {}, wait);
    const file = await screen.findByLabelText("File", {}, wait);
    fireEvent.change(file, { target: { value: "checkout" } });
    type("Name", "Refund");
    type("Page route", "refund");
    const save = screen.getByRole("button", { name: "Save scenario" });
    expect(disabled(save)).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Add step" }));
    type("Steps 1 · Test ID", "back");
    type("Expectations 1 · Test ID", "done");
    expect(disabled(save)).toBe(false);
    fireEvent.click(save);
    await screen.findByText("Scenario saved.", {}, wait);
    edit?.flushAll();
    await waitFor(() => expect(sent.length).toBeGreaterThan(0));
    expect(sent[0]?.path).toBe("tests/checkout.scenario.json");
    expect(sent[0]?.ops.some((o) => o.op !== "test" && o.path.startsWith("/scenarios"))).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Remove Empty cart" }));
    await screen.findByText("Scenario removed.", {}, wait);
  });

  test("a person without plugin.edit reads but cannot author", async () => {
    open("", {}, ["app.read"]);
    await screen.findByText("You may read scenarios but not change them.", {}, wait);
    expect(screen.queryByRole("form")).toBeNull();
  });
});

const homePage = {
  kind: "page",
  path: "pages/home.page.json",
  json: { id: "home", route: "/home", root: { id: "r", children: [{ id: "a", testId: "pay" }, { id: "b" }] } },
};
const trace = (sequence: bigint, runId: string, nodePath: string, trigger = "onTap") =>
  create(DevEventSchema, {
    sequence,
    deviceId: "d1",
    payload: { case: "actionTrace", value: { runId, graph: "g", trigger, nodePath } },
  });

describe("TST-001 scenario steps from the device session (A45)", () => {
  test("prefills the new scenario from the taps the held session recorded", async () => {
    const events = [
      trace(1n, "r1", "/pages/home/0"),
      trace(2n, "r2", "/pages/home/1"),
      trace(3n, "r3", "/pages/home/0", "lifecycle"),
      trace(4n, "r1", "/pages/home/0"),
    ];
    const view = open("", { pluginDocuments: { p1: [homePage] }, devSession: { devices: [], events } });
    view.session.queryClient.setQueryData(["studio-dev-session", "app1"], "s1");
    await screen.findByText("1 tap recorded in the device session", {}, wait);
    fireEvent.click(screen.getByRole("button", { name: "Create test from device session" }));
    await waitFor(() => expect((screen.getByLabelText("Page route") as HTMLInputElement).value).toBe("home"));
    expect((screen.getByLabelText("Steps 1 · Test ID") as HTMLInputElement).value).toBe("pay");
    expect(screen.queryByLabelText("Steps 2 · Test ID")).toBeNull();
  });

  test("offers nothing when the device loop was not opened for the app", async () => {
    open("", { pluginDocuments: { p1: [homePage] } });
    await screen.findByText("Pays", {}, wait);
    expect(screen.queryByRole("button", { name: "Create test from device session" })).toBeNull();
  });
});

describe("TST-002 Plux Test run results (A46)", () => {
  test("shows the result of every scenario and step", async () => {
    open("/r1", {});
    await screen.findByText("found Failed", {}, wait);
    expect(document.body.textContent).toContain("Passed: 1 · Failed: 1 · Skipped: 0");
    expect(screen.getByText("tap pay")).toBeTruthy();
  });

  test("shows an error for an unknown run", async () => {
    open("/nope", {});
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });
});
