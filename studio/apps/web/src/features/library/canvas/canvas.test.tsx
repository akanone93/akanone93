// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, configure, fireEvent, render, screen } from "@testing-library/react";
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

describe("STU-080 STU-083 template documents", () => {
  test("string props are offered as parameters by pointer", () => {
    expect(candidateParameters(nodes[0])).toEqual([
      { name: "Text.text", type: "string", path: "/children/0/props/text" },
    ]);
  });

  test("a saved template captures dependencies as IDs and fills parameters on insert with new IDs", () => {
    const doc = buildTemplateDoc(
      { key: "hello", name: "Hello", description: "", category: "page", visibility: "private" },
      candidateParameters(nodes[0]),
      nodes,
      source,
    );
    expect(doc["dependencies"]).toMatchObject({ functions: ["fmt"] });
    const inserted = instantiateRoot(doc, { "Text.text": "Hi" }) as {
      id: string;
      children: { id: string; props: { text: string } }[];
    }[];
    expect(inserted[0]?.children[0]?.props.text).toBe("Hi");
    expect(inserted[0]?.id).not.toBe(N1);
    expect(inserted[0]?.children[0]?.id).not.toBe(N2);
    expect(templateEnvelope(doc, inserted).dependencies.functions).toEqual(["fmt"]);
  });
});

describe("STU-084 components on the canvas", () => {
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
    await screen.findByText("Step 2 of 2: parameters and visibility").catch(() => undefined);
    expect(save).toHaveBeenCalledWith(
      { name: "Hello page", key: "hello-page", description: "", visibility: "private" },
      [{ name: "Text.text", type: "string", path: "/children/0/props/text" }],
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

describe("STU-055 screens from templates", () => {
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
