// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import {
  ConfigTier,
  ConfigType,
  ConfigValueSchema,
  UpstreamApiKeySchema,
  UpstreamOperationSchema,
  UpstreamSchema,
  UserIdentityProviderSchema,
} from "@plux/api";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import { draftOf, emptyDraft, upstreamOf } from "./upstreams-page.tsx";

afterEach(cleanup);

const base = `/o/${orgA}/apps/app1/gateway`;
const manage = ["app.read", "gateway.manage"];

/** Picks an option of a Select, as a person does. */
async function choose(label: string, option: string): Promise<void> {
  fireEvent.click(screen.getByRole("combobox", { name: label }));
  const item = await screen.findByRole("option", { name: option });
  await new Promise((resolve) => setTimeout(resolve, 300));
  fireEvent.pointerDown(item);
  fireEvent.mouseDown(item);
  fireEvent.pointerUp(item);
  fireEvent.mouseUp(item);
  fireEvent.click(item);
}

const type = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } });

const loans = create(UpstreamSchema, {
  id: "up-1",
  appId: "app1",
  environmentId: "env-dev",
  name: "loans",
  baseUrl: "https://loans.example",
  auth: { case: "apiKey", value: create(UpstreamApiKeySchema, { header: "X-Key", secretRef: "loans-key" }) },
  allowedMethods: ["GET", "POST"],
});

function gateway(over: Partial<NonNullable<NonNullable<Upstream["security"]>["gateway"]>> = {}) {
  return { upstreams: [loans], values: [], calls: [] as string[], ...over };
}

describe("[SEC-030] the gateway's upstreams (build log row 199)", () => {
  test("lists the environment's upstreams with how the gateway signs in to each", async () => {
    renderApp(base, { permissions: manage, security: { gateway: gateway() } });
    const row = (await screen.findByText("loans")).closest("tr") as HTMLElement;
    expect(within(row).getByText("https://loans.example")).toBeTruthy();
    expect(within(row).getByText("API key")).toBeTruthy();
    expect(within(row).getByText("GET, POST")).toBeTruthy();
  });

  test("adds an upstream, edits it and removes it; each change is sent", async () => {
    const g = gateway({ upstreams: [] });
    renderApp(base, { permissions: manage, security: { gateway: g } });
    fireEvent.click(await screen.findByRole("button", { name: "Add upstream" }));
    type("Name", "cards");
    type("Base URL", "https://cards.example");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(g.calls).toEqual(["create:cards"]));
    expect(g.upstreams[0]?.environmentId).toBe("env-dev");
    expect(g.upstreams[0]?.allowedMethods).toEqual(["GET"]);
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    type("Base URL", "https://cards2.example");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(g.calls).toEqual(["create:cards", "update:cards"]));
    expect(g.upstreams[0]?.baseUrl).toBe("https://cards2.example");
    fireEvent.click(await screen.findByRole("button", { name: "Remove" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(g.calls.at(-1)).toBe("delete:up-1"));
  });

  test("an upstream without a name is not sent, and a private address is warned about", async () => {
    const g = gateway({ upstreams: [] });
    renderApp(base, { permissions: manage, security: { gateway: g } });
    fireEvent.click(await screen.findByRole("button", { name: "Add upstream" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Allow private and loopback addresses" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Unsafe");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findAllByText("Required.")).not.toHaveLength(0);
    expect(g.calls).toEqual([]);
  });

  test("without gateway.manage every change is disabled", async () => {
    renderApp(base, { permissions: ["app.read"], security: { gateway: gateway() } });
    const add = await screen.findByRole("button", { name: "Add upstream" });
    expect(add.getAttribute("aria-disabled") ?? String((add as HTMLButtonElement).disabled)).toMatch(/true/);
  });

  test("a draft keeps every field of a stored upstream, its operations as JSON", () => {
    const stored = create(UpstreamSchema, {
      ...loans,
      timeoutMs: 2500,
      requiresAssurance: "AL2",
      operations: [
        create(UpstreamOperationSchema, { name: "pay", methods: ["POST"], pathPrefix: "/v1/pay", plugins: ["bank"] }),
      ],
    });
    const d = draftOf(stored);
    expect(d).toMatchObject({ auth: "apiKey", header: "X-Key", secretRef: "loans-key", timeoutMs: "2500" });
    const back = upstreamOf(d, "app1", "env-dev");
    expect("upstream" in back && back.upstream.operations[0]?.pathPrefix).toBe("/v1/pay");
    expect("upstream" in back && back.upstream.requiresAssurance).toBe("AL2");
    expect(upstreamOf({ ...emptyDraft, name: "x", operations: "{" }, "app1", "env-dev")).toEqual({
      error: "operations",
    });
  });
});

describe("[DAT-090] the environment's configuration values (build log row 199)", () => {
  const values = [
    create(ConfigValueSchema, {
      key: "apiRegion",
      type: ConfigType.STRING,
      tier: ConfigTier.LIVE,
      value: "eu",
      version: 3n,
    }),
    create(ConfigValueSchema, {
      key: "partnerToken",
      type: ConfigType.STRING,
      tier: ConfigTier.SERVER_BOUND,
      sensitive: true,
      value: "s3cr3t",
      version: 1n,
    }),
  ];

  test("lists the values; a sensitive one is sealed, never shown", async () => {
    renderApp(`${base}/config`, { permissions: manage, security: { gateway: gateway({ values }) } });
    expect(await screen.findByText("apiRegion")).toBeTruthy();
    expect(screen.getByText("Sealed")).toBeTruthy();
    expect(screen.queryByText("s3cr3t")).toBeNull();
  });

  test("adds a sealed value, changes one with the version read, and removes one", async () => {
    const g = gateway({ values: [...values] });
    renderApp(`${base}/config`, { permissions: manage, security: { gateway: g } });
    fireEvent.click(await screen.findByRole("button", { name: "Add value" }));
    type("Key (a letter, then letters, digits and underscores)", "bankSecret");
    fireEvent.click(screen.getByRole("checkbox", { name: "Sensitive: sealed at rest and never shown again" }));
    type("Value", "hunter2");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(g.calls).toEqual(["set:bankSecret:0"]));
    expect(g.values.find((v) => v.key === "bankSecret")?.sensitive).toBe(true);
    const row = (await screen.findByText("apiRegion")).closest("tr") as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Edit" }));
    type("Value", "us");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(g.calls.at(-1)).toBe("set:apiRegion:3"));
    const again = (await screen.findByText("apiRegion")).closest("tr") as HTMLElement;
    fireEvent.click(within(again).getByRole("button", { name: "Remove" }));
    fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(g.calls.at(-1)).toBe("unset:apiRegion"));
  });

  test("a key that is not one is refused before it is sent", async () => {
    const g = gateway();
    renderApp(`${base}/config`, { permissions: manage, security: { gateway: g } });
    fireEvent.click(await screen.findByRole("button", { name: "Add value" }));
    type("Key (a letter, then letters, digits and underscores)", "9lives");
    type("Value", "x");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findAllByText("Required.")).not.toHaveLength(0);
    expect(g.calls).toEqual([]);
  });
});

describe("[SEC-026] the environment's user identity provider (build log row 199)", () => {
  test("says when none is trusted, then saves one and removes it", async () => {
    const g = gateway();
    renderApp(`${base}/identity`, { permissions: manage, security: { gateway: g } });
    expect(await screen.findByText("This environment trusts no user identity provider.")).toBeTruthy();
    type("Issuer", "https://id.example");
    type("JWKS URL (HTTPS)", "https://id.example/jwks");
    type("Audiences, one per line", "bank-app\nbank-web");
    fireEvent.click(screen.getByRole("button", { name: "Save provider" }));
    await waitFor(() => expect(g.calls).toEqual(["provider:https://id.example"]));
    expect(g.provider?.audiences).toEqual(["bank-app", "bank-web"]);
    fireEvent.click(await screen.findByRole("button", { name: "Remove provider" }));
    fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(g.calls.at(-1)).toBe("provider:removed"));
  });

  test("shows the trusted provider; one without an audience is not sent", async () => {
    const provider = create(UserIdentityProviderSchema, {
      appId: "app1",
      environmentId: "env-dev",
      issuer: "https://id.example",
      jwksUrl: "https://id.example/jwks",
      audiences: ["bank-app"],
      forwardClaims: ["email"],
    });
    const g = gateway({ provider });
    renderApp(`${base}/identity`, { permissions: manage, security: { gateway: g } });
    await waitFor(() => expect((screen.getByLabelText("Issuer") as HTMLInputElement).value).toBe("https://id.example"));
    type("Audiences, one per line", "");
    fireEvent.click(screen.getByRole("button", { name: "Save provider" }));
    expect(await screen.findAllByText("Required.")).not.toHaveLength(0);
    expect(g.calls).toEqual([]);
  });
});

describe("the Gateway area's environment (build log row 199)", () => {
  test("the pages follow the environment chosen", async () => {
    const prod = create(UpstreamSchema, { ...loans, id: "up-9", environmentId: "env-prod", name: "ledger" });
    renderApp(base, { permissions: manage, security: { gateway: gateway({ upstreams: [loans, prod] }) } });
    expect(await screen.findByText("loans")).toBeTruthy();
    await choose("Environment", "production");
    expect(await screen.findByText("ledger")).toBeTruthy();
    expect(screen.queryByText("loans")).toBeNull();
  });
});
