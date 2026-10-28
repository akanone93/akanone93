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

  test("the assurance and capabilities tabs show what was verified and granted", async () => {
    open("/convert?tab=assurance", {
      functions: {
        functions: [
          {
            name: "convert",
            version: 2,
            assurance: "hardware",
            sourceId: "src-9",
            capabilities: { http: ["api.example.com"], kv: true, secrets: ["rates-key"] },
          },
        ],
      },
    });
    await screen.findByText("module-convert");
    expect(screen.getByText("serde")).toBeTruthy();
    expect(screen.getByText("hardware")).toBeTruthy();
    expect(screen.getByText("src-9")).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Capabilities" }));
    await screen.findByText("Host ABI version");
    expect(screen.getByText("the server")).toBeTruthy();
    expect(screen.getByText("api.example.com")).toBeTruthy();
    expect(screen.getByText("rates-key")).toBeTruthy();
    expect(screen.getByText("Key-value store").nextSibling?.textContent).toBe("Granted");
    expect(screen.getByText("User claims").nextSibling?.textContent).toBe("Not granted");
    expect(screen.getByText("Functions it may call").nextSibling?.textContent).toBe("None");
  });

  test("a version with no recorded declaration shows the default assurance and no grants", async () => {
    open("/convert?tab=assurance", { functions: { functions } });
    await screen.findByText("The environment's default");
    fireEvent.click(screen.getByRole("tab", { name: "Capabilities" }));
    await screen.findByText("This version recorded no declaration, so it is granted nothing.");
    expect(screen.queryByText("Key-value store")).toBeNull();
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

  test("the logs tab reads the environment chosen from the app's own", async () => {
    const logEnvironments: string[] = [];
    open("/convert?tab=logs", {
      functions: {
        functions,
        logEnvironments,
        logs: [
          { level: "info", message: "from development", environmentId: "env-dev" },
          { level: "info", message: "from production", environmentId: "env-prod" },
        ],
      },
    });
    await screen.findByText("from development");
    expect(screen.queryByText("from production")).toBeNull();
    fireEvent.click(screen.getByRole("combobox", { name: "Environment" }));
    const option = await screen.findByRole("option", { name: "production" });
    await new Promise((resolve) => setTimeout(resolve, 300));
    fireEvent.pointerDown(option);
    fireEvent.mouseDown(option);
    fireEvent.pointerUp(option);
    fireEvent.mouseUp(option);
    fireEvent.click(option);
    await screen.findByText("from production");
    expect(screen.queryByText("from development")).toBeNull();
    expect(logEnvironments.at(0)).toBe("env-dev");
    expect(logEnvironments.at(-1)).toBe("env-prod");
  });

  test("the tests tab runs the stored tests of the version's own source", async () => {
    const testSources: string[] = [];
    open("/convert?tab=tests", {
      functions: {
        functions: [{ name: "convert", version: 2, sourceId: "src-9" }],
        testSources,
        tests: { results: [{ name: "adds", passed: true, message: "" }] },
      },
    });
    await screen.findByText("src-9");
    expect(screen.queryByRole("textbox", { name: "Uploaded source" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Run the tests" }));
    await screen.findByText("adds");
    expect(testSources).toEqual(["src-9"]);
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
