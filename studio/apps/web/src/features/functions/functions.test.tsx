// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import type { RustViewProps } from "../rust-editor/rust-monaco-view.tsx";

/** Monaco needs a real browser; the editor page is tested against a stand-in with the same props. */
mock.module("../rust-editor/rust-monaco-view.tsx", () => ({
  default: (props: RustViewProps) => (
    <textarea
      aria-label={props.label}
      readOnly={props.readOnly}
      value={props.value}
      onChange={(e) => props.onChange(e.target.value)}
    />
  ),
}));

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
  Object.assign(globalThis, { PublicKeyCredential: StubCredential, AuthenticatorAssertionResponse: StubResponse });
  Object.defineProperty(navigator, "credentials", {
    value: { get: mock(async () => new StubCredential()) },
    configurable: true,
  });
}

const base = `/o/${orgA}/apps/app1/functions`;
const all = ["app.read", "function.deploy"];
const functions = [
  { name: "convert", version: 2 },
  { name: "risky", version: 1, testsPassed: false },
];
const open = (path: string, state: Upstream, permissions = all) =>
  renderApp(`${base}${path}`, { permissions, ...state });

describe("STU-110 functions list (A10)", () => {
  test("lists the functions with placement and test state", async () => {
    open("", { functions: { functions } });
    await screen.findByRole("link", { name: "convert" });
    expect(screen.getByText("Passed")).toBeTruthy();
    expect(screen.getByText("Failed")).toBeTruthy();
  });

  test("shows the empty and the error state", async () => {
    open("", { functions: { functions: [] } });
    await screen.findByText("No function is deployed in this environment.");
    cleanup();
    open("", { functions: { functions: [], failing: true } });
    await screen.findByRole("alert");
  });
});

describe("STU-110 function detail (A11)", () => {
  test("shows versions and aliases, and points an alias only after the step-up", async () => {
    stubAssertion();
    const calls: string[] = [];
    open("/convert", {
      functions: { functions, aliases: [{ name: "production", version: 1 }], calls },
      security: { stepUps: [] },
    });
    await screen.findByText("production points at version 1");
    fireEvent.click(await screen.findByRole("button", { name: "Point alias at this version 2" }));
    expect(calls).toEqual([]);
    fireEvent.click(await screen.findByRole("button", { name: "Use security key or passkey" }));
    await waitFor(() => expect(calls).toEqual(["production:2"]));
  });

  test("a person without function.deploy cannot point an alias", async () => {
    open("/convert", { functions: { functions } }, ["app.read"]);
    const button = await screen.findByRole("button", { name: "Point alias at this version 2" });
    expect(button.getAttribute("aria-disabled")).toBe("true");
  });

  test("the assurance and capabilities tabs show what was verified", async () => {
    open("/convert?tab=assurance", { functions: { functions } });
    await screen.findByText("module-convert");
    expect(screen.getByText("serde")).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Capabilities" }));
    await screen.findByText("Host ABI version");
    expect(screen.getByText("the server")).toBeTruthy();
  });

  test("the metrics tab shows counts and errors by kind", async () => {
    open("/convert?tab=metrics", { functions: { functions, metrics: { invocations: 42, errorsByKind: { trap: 3 } } } });
    await screen.findByText("42");
    expect(screen.getByText("trap")).toBeTruthy();
  });

  test("the logs tab shows the retained lines", async () => {
    open("/convert?tab=logs", { functions: { functions, logs: [{ level: "info", message: "converted" }] } });
    await screen.findByText("converted");
  });

  test("the tests tab runs the stored tests of a source", async () => {
    open("/convert?tab=tests", {
      functions: { functions, tests: { results: [{ name: "adds", passed: true, message: "" }] } },
    });
    fireEvent.change(await screen.findByLabelText("Uploaded source"), { target: { value: "src-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Run the tests" }));
    await screen.findByText("adds");
  });
});

describe("FN-033 editor page (A12)", () => {
  test("opens the Rust editor as a page", async () => {
    open("/convert/edit", { functions: { functions }, rust: { reply: () => [] } });
    const area = (await screen.findByLabelText(
      "Rust source of the function",
      {},
      { timeout: 5000 },
    )) as HTMLTextAreaElement;
    expect(area.value).toContain("plux_fn");
    expect(area.readOnly).toBe(false);
  });

  test("is read-only without function.deploy", async () => {
    open("/convert/edit", { functions: { functions }, rust: { reply: () => [] } }, ["app.read"]);
    const area = (await screen.findByLabelText(
      "Rust source of the function",
      {},
      { timeout: 5000 },
    )) as HTMLTextAreaElement;
    expect(area.readOnly).toBe(true);
  });
});
