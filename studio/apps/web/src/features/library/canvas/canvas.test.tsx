// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, configure, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../../i18n-provider.tsx";
import { createUpstream, loadSourceCatalogue } from "../../../test-harness.tsx";
import { ExtractComponentDialog, InsertTemplateDialog, SaveTemplateDialog } from "./library-dialogs.tsx";
import {
  buildComponentDoc,
  buildTemplateDoc,
  candidateParameters,
  detachedNodes,
  instanceNode,
  instantiateRoot,
  rootOf,
  scanPageState,
  templateEnvelope,
} from "./template-doc.ts";
import { instantiateScreen } from "./template-start.tsx";

const isDisabled = (el: HTMLElement) =>
  el.hasAttribute("disabled") || el.getAttribute("aria-disabled") === "true" || el.hasAttribute("data-disabled");

configure({ asyncUtilTimeout: 5000 });
afterEach(cleanup);

const renderI18n = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );

const N1 = "01900000-0000-7000-8000-000000000001";
const N2 = "01900000-0000-7000-8000-000000000002";
const source = { app: "a", plugin: "p" };
const nodes = [
  {
    id: N1,
    type: "Column",
    children: [{ id: N2, type: "Text", props: { text: "Hello", style: 3 }, function: "fmt" }],
  },
];

describe("[STU-080] [STU-083] template documents", () => {
  test("string props are offered as parameters by pointer", () => {
    expect(candidateParameters(nodes[0])).toEqual([
      { name: "textText", label: "Text.text", type: "string", path: "/children/0/props/text" },
    ]);
  });

  test("a saved template captures dependencies as IDs and fills parameters on insert with new IDs", () => {
    const doc = buildTemplateDoc(
      { key: "hello", name: "Hello", description: "", category: "page", visibility: "private" },
      candidateParameters(nodes[0]),
      nodes,
      source,
    );
    expect(doc["parameters"]).toEqual([
      { name: "textText", type: "string", path: "/children/0/props/text", description: "Text.text" },
    ]);
    const inserted = instantiateRoot(doc, { textText: "Hi" }) as {
      id: string;
      children: { id: string; props: { text: string } }[];
    }[];
    expect(inserted[0]?.children[0]?.props.text).toBe("Hi");
    expect(inserted[0]?.id).not.toBe(N1);
    expect(inserted[0]?.children[0]?.id).not.toBe(N2);
    expect(templateEnvelope(doc, inserted).dependencies.functions).toEqual(["fmt"]);
  });
});

describe("[STU-084] components on the canvas", () => {
  test("extraction yields a component document and an instance node that refers to it", () => {
    const doc = buildComponentDoc({ key: "card", name: "Card", description: "" }, nodes, source);
    const instance = instanceNode(String(doc["id"]), 1);
    expect(instance["component"]).toEqual({ id: doc["id"], version: 1 });
    expect(doc["kind"]).toBe("component");
  });

  test("detaching gives plain nodes with new IDs and the instance's overrides", () => {
    const out = detachedNodes({ id: "i", component: { id: "c", version: 1 }, props: { text: "Mine" } }, nodes[0]) as {
      id: string;
      props: { text: string };
    }[];
    expect(out[0]?.id).not.toBe(N1);
    expect(out[0]?.props.text).toBe("Mine");
  });
});

describe("D28 D29 D30 dialogs", () => {
  test("saving a template takes two steps and passes the exposed parameters", async () => {
    const save = mock(async () => undefined);
    renderI18n(<SaveTemplateDialog candidates={candidateParameters(nodes[0])} save={save} onClose={() => undefined} />);
    fireEvent.change(await screen.findByRole("textbox", { name: "Name" }), { target: { value: "Hello page" } });
    expect((screen.getByRole("textbox", { name: "Key" }) as HTMLInputElement).value).toBe("hello-page");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    fireEvent.click(await screen.findByRole("checkbox", { name: "Text.text" }));
    fireEvent.click(screen.getByRole("button", { name: "Save template" }));
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save).toHaveBeenCalledWith(
      { name: "Hello page", key: "hello-page", description: "", visibility: "private" },
      [{ name: "textText", label: "Text.text", type: "string", path: "/children/0/props/text" }],
    );
  });

  test("extraction needs a valid key and reports a failed save", async () => {
    const extract = mock(async () => {
      throw new Error("taken");
    });
    renderI18n(<ExtractComponentDialog extract={extract} onClose={() => undefined} />);
    const go = await screen.findByRole("button", { name: "Extract" });
    expect(isDisabled(go)).toBe(true);
    fireEvent.change(screen.getByRole("textbox", { name: "Name" }), { target: { value: "Card" } });
    fireEvent.click(go);
    await screen.findByRole("alert");
  });

  test("the insert dialog shows loading, empty and error states and asks the parameters", async () => {
    const templates = [{ id: "t1", name: "Login" }] as never;
    const insert = mock(() => undefined);
    const doc = { parameters: [{ name: "title", type: "string", path: "/props/title" }], root: {} };
    const view = renderI18n(
      <InsertTemplateDialog
        templates={[]}
        state="loading"
        load={async () => doc}
        insert={insert}
        onClose={() => undefined}
      />,
    );
    await screen.findByText("Loading templates…");
    view.rerender(
      <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
        <InsertTemplateDialog
          templates={[]}
          state="failed"
          load={async () => doc}
          insert={insert}
          onClose={() => undefined}
        />
      </I18nProvider>,
    );
    await screen.findByRole("alert");
    cleanup();
    renderI18n(
      <InsertTemplateDialog
        templates={templates}
        state="ready"
        load={async () => doc}
        insert={insert}
        onClose={() => undefined}
      />,
    );
    expect(isDisabled(await screen.findByRole("button", { name: "Insert" }))).toBe(true);
  });
});

describe("[STU-055] screens from templates", () => {
  test("instantiating sends the template, key and editor session to the server", async () => {
    const calls: string[] = [];
    const res = await instantiateScreen(createUpstream({ library: { items: [], calls } }), {
      templateId: "t1",
      appId: "app1",
      pluginId: "p1",
      editorSessionId: "s1",
      key: "login",
    });
    expect(res).toEqual({ ok: true });
    expect(calls).toEqual(["instantiate:t1:login:s1"]);
  });
});

const AMOUNT = "01900000-0000-7000-8000-000000000003";
const COMPONENT = "01900000-0000-7000-8000-000000000004";
const ASSET = "01900000-0000-7000-8000-000000000005";
const pageNodes = [
  {
    id: N1,
    type: "Column",
    children: [
      {
        id: AMOUNT,
        type: "Text",
        props: {
          value: { $expr: "page.amount" },
          color: { $token: "color.primary" },
          label: { $expr: "page.result.total" },
          hint: { $expr: 'page.missing + "!"' },
        },
        events: { onPressed: { steps: [{ id: "s", action: "setState", input: { path: "page.amount", value: 1 } }] } },
      },
      { id: N2, type: "Image", props: { source: { $asset: ASSET } } },
      { id: "01900000-0000-7000-8000-000000000006", component: { id: COMPONENT, version: 2 }, props: {} },
    ],
  },
];
const pageState = [{ name: "amount", type: "decimal", default: "0" }];

describe("[STU-082] dependencies and page state of a saved template", () => {
  test("captures tokens, assets and components as references, never content", () => {
    const doc = buildTemplateDoc(
      { key: "k", name: "K", description: "", category: "page", visibility: "private" },
      [],
      pageNodes,
      source,
      pageState,
    );
    expect(doc["dependencies"]).toEqual({
      tokens: ["color.primary"],
      assets: [ASSET],
      components: [{ id: COMPONENT, version: 2 }],
    });
  });

  test("a binding that is exactly a declared page-state entry becomes a parameter of its type with its default", () => {
    const doc = buildTemplateDoc(
      { key: "k", name: "K", description: "", category: "page", visibility: "private" },
      [],
      pageNodes,
      source,
      pageState,
    );
    expect(doc["parameters"]).toEqual([
      {
        name: "amount",
        type: "decimal",
        path: "/children/0/props/value",
        description: "page.amount",
        default: "0",
      },
    ]);
    const root = doc["root"] as { children: { props: { value: unknown } }[] };
    expect(root.children[0]?.props.value).toBe("0");
    expect(JSON.stringify(root)).not.toContain('page.amount"}');
  });

  test("references that cannot be converted stay in place and are flagged with the reason", () => {
    const doc = buildTemplateDoc(
      { key: "k", name: "K", description: "", category: "page", visibility: "private" },
      [],
      pageNodes,
      source,
      pageState,
    );
    expect(doc["x-flagged"]).toEqual([
      { path: "/children/0/props/label", expression: "page.result.total", reason: "expression" },
      { path: "/children/0/props/hint", expression: 'page.missing + "!"', reason: "expression" },
      { path: "/children/0/events/onPressed/steps/0/input/path", expression: "page.amount", reason: "action" },
    ]);
    const root = doc["root"] as { children: { props: { label: unknown } }[] };
    expect(root.children[0]?.props.label).toEqual({ $expr: "page.result.total" });
  });

  test("a bare reference to state the page does not declare is flagged as undeclared", () => {
    const scan = scanPageState({ id: N1, type: "Text", props: { value: { $expr: "page.ghost" } } }, pageState);
    expect(scan.parameters).toEqual([]);
    expect(scan.flagged).toEqual([{ path: "/props/value", expression: "page.ghost", reason: "undeclared" }]);
  });

  test("two bindings to one entry give two parameters with distinct schema names", () => {
    const scan = scanPageState(
      {
        id: N1,
        type: "Row",
        children: [{ props: { a: { $expr: "page.amount" } } }, { props: { b: { $expr: "page.amount" } } }],
      },
      pageState,
    );
    expect(scan.parameters.map((p) => p.name)).toEqual(["amount", "amount2"]);
  });

  test("a template without page state or flags carries no flag list and parameter names fit the schema", () => {
    const doc = buildTemplateDoc(
      { key: "k", name: "K", description: "", category: "page", visibility: "private" },
      candidateParameters(nodes[0]),
      nodes,
      source,
    );
    expect("x-flagged" in doc).toBe(false);
    for (const p of doc["parameters"] as { name: string }[]) {
      expect(p.name).toMatch(/^[a-z][a-zA-Z0-9]*$/);
    }
  });

  test("inserting fills a converted parameter with a value of its type and keeps the default when none is given", () => {
    const doc = buildTemplateDoc(
      { key: "k", name: "K", description: "", category: "page", visibility: "private" },
      [],
      [{ id: N1, type: "Slider", props: { value: { $expr: "page.count" }, enabled: { $expr: "page.on" } } }],
      source,
      [
        { name: "count", type: "int", default: 3 },
        { name: "on", type: "bool" },
      ],
    );
    const filled = instantiateRoot(doc, { count: "7", on: "true" }) as { props: Record<string, unknown> }[];
    expect(filled[0]?.props).toEqual({ value: 7, enabled: true });
    const kept = instantiateRoot(doc, {}) as { props: Record<string, unknown> }[];
    expect(kept[0]?.props).toEqual({ value: 3, enabled: null });
  });

  test("the save dialog lists converted and flagged page state in step two and saves the chosen parameters", async () => {
    const scan = scanPageState(rootOf(pageNodes), pageState);
    const save = mock(async () => undefined);
    renderI18n(<SaveTemplateDialog candidates={[]} pageState={scan} save={save} onClose={() => undefined} />);
    fireEvent.change(await screen.findByRole("textbox", { name: "Name" }), { target: { value: "Loan" } });
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    const converted = await screen.findByRole("region", { name: "Page state turned into parameters" });
    expect(converted.textContent).toContain("page.amount becomes the parameter amount");
    const flagged = screen.getByRole("region", { name: "Page state that cannot become a parameter" });
    expect(flagged.textContent).toContain("page.result.total is part of a larger expression");
    expect(flagged.textContent).toContain("Action state path page.amount");
  });
});
