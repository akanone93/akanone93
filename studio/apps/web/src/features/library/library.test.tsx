// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { BinaryWriter } from "@bufbuild/protobuf/wire";
import { Code, ConnectError } from "@connectrpc/connect";
import { cleanup, configure, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { LibraryFixture } from "../../test-upstream.ts";
import { decodeMetadata, scanFindings } from "./library-model.ts";

configure({ asyncUtilTimeout: 5000 });

afterEach(cleanup);

const items: LibraryFixture["items"] = [
  { id: "c1", kind: "component", key: "card", name: "Card", visibility: "organization", createdByName: "Ada" },
  {
    id: "t1",
    kind: "template",
    key: "login",
    name: "Login page",
    visibility: "private",
    templateKind: "page",
    description: "#auth",
  },
  { id: "t2", kind: "template", key: "wizard", name: "Wizard flow", visibility: "public", templateKind: "flow" },
];

describe("STU-086 library browser (O4)", () => {
  test("lists components and templates and narrows them by the URL filters", async () => {
    renderApp(`/o/${orgA}/library`, { library: { items } });
    await screen.findByRole("link", { name: "Card" });
    expect(screen.getByRole("link", { name: "Login page" })).toBeTruthy();
    cleanup();
    renderApp(`/o/${orgA}/library?kind=template&category=flow`, { library: { items } });
    await screen.findByRole("link", { name: "Wizard flow" });
    expect(screen.queryByRole("link", { name: "Card" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Login page" })).toBeNull();
  });

  test("a tag filter matches the hashtags of the description", async () => {
    renderApp(`/o/${orgA}/library?tag=auth`, { library: { items } });
    await screen.findByRole("link", { name: "Login page" });
    expect(screen.queryByRole("link", { name: "Wizard flow" })).toBeNull();
  });

  test("shows the empty and the error state", async () => {
    renderApp(`/o/${orgA}/library`, { library: { items: [] } });
    await screen.findByText(/Nothing in the library matches/);
    cleanup();
    renderApp(`/o/${orgA}/library`, { library: { items, failList: true } });
    await screen.findByRole("alert");
  });

  test("import is offered to holders of library.manage only and sends the file", async () => {
    const calls: string[] = [];
    renderApp(`/o/${orgA}/library`, { library: { items, calls } });
    await screen.findByRole("link", { name: "Card" });
    expect(screen.queryByTestId("import-template")).toBeNull();
    cleanup();
    renderApp(`/o/${orgA}/library`, { library: { items, calls }, permissions: ["library.manage"] });
    const input = await screen.findByTestId("import-template");
    fireEvent.change(input, { target: { files: [new File(["{}"], "x.plux-template")] } });
    await waitFor(() => expect(calls).toEqual(["importTemplate:2"]));
  });
});

describe("STU-084 STU-085 component detail (O5)", () => {
  const library: LibraryFixture = {
    items,
    usages: 3,
    versions: {
      c1: [
        { version: 1, content: '{"props":{"a":{},"b":{}}}' },
        { version: 2, content: '{"props":{"a":{}}}' },
      ],
    },
    conflicts: [{ file: "plugins/p/pages/home.json", path: "/root/0", removedProps: ["b"] }],
    calls: [],
  };

  test("shows versions and the instances per app, and exports a version", async () => {
    renderApp(`/o/${orgA}/library/components/c1`, { library });
    await screen.findByRole("heading", { name: "Card" });
    await screen.findByText("Shop: 3 instances");
    expect(screen.getByText("Version 2 by Ada")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: "Export" })[0] as HTMLElement);
    await waitFor(() => expect(library.calls).toContain("exportComponent:c1:2"));
  });

  test("adopting shows the diff and flags the overrides of removed props", async () => {
    library.calls = [];
    renderApp(`/o/${orgA}/library/components/c1`, { library, permissions: ["plugin.edit"] });
    fireEvent.click(await screen.findByRole("button", { name: "Adopt a version" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText("New props:", { exact: false });
    fireEvent.click(within(dialog).getByRole("button", { name: "Adopt version" }));
    await within(dialog).findByText("Version 2 is adopted.");
    expect(within(dialog).getByText("plugins/p/pages/home.json/root/0")).toBeTruthy();
    expect(library.calls).toContain("adopt:app1:2");
  });

  test("adopting is hidden without plugin.edit", async () => {
    renderApp(`/o/${orgA}/library/components/c1`, { library });
    await screen.findByText("Shop: 3 instances");
    expect(screen.queryByRole("button", { name: "Adopt a version" })).toBeNull();
  });

  test("a missing item is an error state", async () => {
    renderApp(`/o/${orgA}/library/components/nope`, { library });
    await screen.findByRole("alert");
  });
});

describe("STU-080 STU-081 template detail (O6) and D14", () => {
  const scan = "the content cannot be public: /a/b (secret), /c (url)";

  test("shows parameters and revisions", async () => {
    renderApp(`/o/${orgA}/library/templates/t1`, {
      library: { items, parameters: [{ name: "title", type: "string", required: true }] },
    });
    await screen.findByText("title (string), required");
    expect(screen.getByText("Version 1 by Ada")).toBeTruthy();
  });

  test("making a template public lists the scan findings by pointer and category", async () => {
    const calls: string[] = [];
    renderApp(`/o/${orgA}/library/templates/t1`, {
      library: { items, scanRefusal: scan, calls },
      permissions: ["library.publish"],
    });
    fireEvent.click(await screen.findByRole("button", { name: "Change visibility" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("radio", { name: "Public" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Apply" }));
    await within(dialog).findByText("/a/b (secret)");
    expect(within(dialog).getByText("/c (url)")).toBeTruthy();
    expect(calls).toEqual(["visibility:template:t1:public"]);
  });

  test("public is unavailable without library.publish", async () => {
    renderApp(`/o/${orgA}/library/templates/t1`, { library: { items }, permissions: ["library.manage"] });
    fireEvent.click(await screen.findByRole("button", { name: "Change visibility" }));
    const dialog = await screen.findByRole("dialog");
    expect((within(dialog).getByRole("radio", { name: "Public" }) as HTMLInputElement).disabled).toBe(true);
  });

  test("the findings come from the ErrorInfo metadata, else from the message", () => {
    const entry = new BinaryWriter()
      .uint32(10)
      .string("findings")
      .uint32(18)
      .string('[{"pointer":"/x","category":"secret"}]')
      .finish();
    const info = new BinaryWriter().uint32(26).bytes(entry).finish();
    expect(JSON.parse(decodeMetadata(info).get("findings") ?? "[]")).toEqual([{ pointer: "/x", category: "secret" }]);
    expect(scanFindings(new ConnectError(scan, Code.FailedPrecondition))).toEqual([
      { pointer: "/a/b", category: "secret" },
      { pointer: "/c", category: "url" },
    ]);
  });
});
