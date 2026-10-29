// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, configure, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { produce } from "immer";
import { I18nProvider } from "../../../i18n-provider.tsx";
import { CommandsProvider, createRegistry, useCommands } from "../../../lib/commands/index.ts";
import { createSession } from "../../../lib/session.ts";
import { AppContextProvider } from "../../../lib/use-app.tsx";
import { createUpstream, loadSourceCatalogue } from "../../../test-harness.tsx";
import type { LibraryFixture } from "../../../test-upstream.ts";
import { type ClipboardInput, targetHas } from "../../plugin-canvas/clipboard/use-clipboard.tsx";
import { InstanceOverrides } from "./instance-overrides.tsx";
import { TemplateStartField } from "./template-start.tsx";
import { useLibraryActions } from "./use-library-actions.tsx";

configure({ asyncUtilTimeout: 5000 });
afterEach(cleanup);

const N0 = "01900000-0000-7000-8000-000000000010";
const N1 = "01900000-0000-7000-8000-000000000011";
const I1 = "01900000-0000-7000-8000-000000000012";
const text = { id: N1, type: "Text", props: { text: "Hello" } };
const instance = { id: I1, component: { id: "cdoc", version: 1 }, props: { text: "Mine" } };
const tree = () => ({ id: N0, children: [text, instance] });

interface EditCall {
  readonly doc: string;
  readonly recipe: (draft: unknown) => void;
  readonly options: { group?: string; step?: string } | undefined;
}

function Runner() {
  const { states } = useCommands();
  return (
    <div>
      {states.map((s) => (
        <button key={s.command.id} type="button" onClick={() => void s.command.run()}>
          {s.command.id}
        </button>
      ))}
    </div>
  );
}

function Harness({ input }: { input: ClipboardInput }) {
  return (
    <>
      <Runner />
      <Dialogs input={input} />
    </>
  );
}

function Dialogs({ input }: { input: ClipboardInput }) {
  return useLibraryActions(input);
}

function mount(
  library: LibraryFixture,
  selected: string,
  has = targetHas([]),
  extra: Partial<ClipboardInput> = {},
  screenRoot: unknown = tree(),
) {
  const edits: EditCall[] = [];
  const session = createSession("/api", createUpstream({ library }));
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  const registry = createRegistry([]);
  const input: ClipboardInput = {
    screens: [{ id: "s1", root: screenRoot }],
    screenOf: new Map([
      [N0, "s1"],
      [N1, "s1"],
      [I1, "s1"],
    ]),
    selected: () => [selected],
    edit: (doc, recipe, options) => edits.push({ doc, recipe, options }),
    canEdit: true,
    source: { app: "a", plugin: "p" },
    has,
    validatePage: undefined,
    ...extra,
  };
  render(
    <I18nProvider studio={studio}>
      <AppContextProvider value={{ session, studio }}>
        <QueryClientProvider client={session.queryClient}>
          <CommandsProvider value={{ registry, states: registry.stateFor([]) }}>
            <Harness input={input} />
          </CommandsProvider>
        </QueryClientProvider>
      </AppContextProvider>
    </I18nProvider>,
  );
  return edits;
}

const run = async (id: string) => fireEvent.click(await screen.findByRole("button", { name: id }));
const applied = (e: EditCall) => produce(tree(), (d) => e.recipe(d)) as { children: { id: string }[] };

const templateDoc = JSON.stringify({
  id: "tdoc",
  kind: "template",
  parameters: [{ name: "title", type: "string", path: "/props/title" }],
  dependencies: { functions: ["fmt"] },
  root: { id: "r", type: "Text", function: "fmt", props: { title: "x" } },
});

describe("[STU-080] [STU-083] [STU-084] canvas library commands", () => {
  test("save as template creates the template with the selection and the chosen visibility", async () => {
    const calls: string[] = [];
    mount({ items: [], calls }, N1);
    await screen.findByRole("button", { name: "library.saveTemplate" });
    await run("library.saveTemplate");
    fireEvent.change(await screen.findByRole("textbox", { name: "Name" }), { target: { value: "Hello page" } });
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    fireEvent.click(await screen.findByRole("button", { name: "Save template" }));
    await waitFor(() => expect(calls).toEqual(["createTemplate:hello-page"]));
  });

  test("save as template turns page state into parameters, flags the rest and lists the dependencies [STU-082]", async () => {
    const created: NonNullable<LibraryFixture["created"]> = [];
    const bound = {
      id: N1,
      type: "Text",
      props: {
        value: { $expr: "page.amount" },
        color: { $token: "color.primary" },
        label: { $expr: "page.amount + 1" },
      },
    };
    mount(
      { items: [], created },
      N1,
      targetHas([]),
      { pageState: (id) => (id === "s1" ? [{ name: "amount", type: "int", default: 5 }] : []) },
      { id: N0, children: [bound] },
    );
    await run("library.saveTemplate");
    fireEvent.change(await screen.findByRole("textbox", { name: "Name" }), { target: { value: "Amount" } });
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("page.amount becomes the parameter amount");
    await screen.findByText("page.amount + 1 is part of a larger expression");
    fireEvent.click(screen.getByRole("button", { name: "Save template" }));
    await waitFor(() => expect(created).toHaveLength(1));
    const doc = JSON.parse(created[0]?.content ?? "{}") as Record<string, unknown>;
    expect(doc["parameters"]).toEqual([
      { name: "amount", type: "int", path: "/props/value", description: "page.amount", default: 5 },
    ]);
    expect(doc["dependencies"]).toEqual({ tokens: ["color.primary"], assets: [], components: [] });
    expect(doc["x-flagged"]).toEqual([{ path: "/props/label", expression: "page.amount + 1", reason: "expression" }]);
    expect((doc["root"] as { props: { value: unknown } }).props.value).toBe(5);
  });

  test("insert template fills the parameters and places new nodes in one grouped edit when nothing is missing", async () => {
    const library: LibraryFixture = {
      items: [{ id: "t1", kind: "template", key: "login", name: "Login", visibility: "private", templateKind: "page" }],
      versions: { t1: [{ version: 1, content: templateDoc }] },
    };
    const edits = mount(library, N1, targetHas([{ content: new TextEncoder().encode('{"id":"fmt"}') }]));
    await run("library.insertTemplate");
    fireEvent.click(await screen.findByRole("combobox"));
    fireEvent.click(await screen.findByRole("option", { name: "Login" }));
    fireEvent.change(await screen.findByRole("textbox", { name: "title" }), { target: { value: "Hi" } });
    fireEvent.click(screen.getByRole("button", { name: "Insert" }));
    await waitFor(() => expect(edits).toHaveLength(1));
    const after = applied(edits[0] as EditCall);
    expect(after.children.map((c) => c.id).slice(0, 2)).toEqual([N1, expect.not.stringMatching(N1)]);
    expect(JSON.stringify(after)).toContain('"title":"Hi"');
    expect(edits[0]?.options?.step).toBe("insert-template");
  });

  test("insert template reports a dependency the target lacks before placing anything", async () => {
    const library: LibraryFixture = {
      items: [{ id: "t1", kind: "template", key: "login", name: "Login", visibility: "private", templateKind: "page" }],
      versions: { t1: [{ version: 1, content: templateDoc }] },
    };
    const edits = mount(library, N1);
    await run("library.insertTemplate");
    fireEvent.click(await screen.findByRole("combobox"));
    fireEvent.click(await screen.findByRole("option", { name: "Login" }));
    fireEvent.click(await screen.findByRole("button", { name: "Insert" }));
    await screen.findByText("fmt", { exact: false });
    expect(edits).toEqual([]);
  });

  test("extract to component creates it and swaps the selection for an instance in one step", async () => {
    const calls: string[] = [];
    const edits = mount({ items: [], calls }, N1);
    await run("library.extractComponent");
    fireEvent.change(await screen.findByRole("textbox", { name: "Name" }), { target: { value: "Card" } });
    fireEvent.click(screen.getByRole("button", { name: "Extract" }));
    await waitFor(() => expect(edits).toHaveLength(2));
    expect(calls).toEqual(["createComponent:card"]);
    expect(edits[0]?.options?.group).toBe(edits[1]?.options?.group);
    const after = applied(edits[0] as EditCall);
    expect(JSON.stringify(after)).toContain('"component"');
    expect(applied(edits[1] as EditCall).children.map((c) => c.id)).not.toContain(N1);
  });

  test("detach replaces an instance by the component's nodes carrying its overrides", async () => {
    const library: LibraryFixture = {
      items: [{ id: "c1", kind: "component", key: "card", name: "Card", visibility: "private", documentId: "cdoc" }],
      versions: { c1: [{ version: 1, content: '{"root":{"id":"x","type":"Text","props":{"text":"Base"}}}' }] },
    };
    const edits = mount(library, I1);
    await run("library.detach");
    fireEvent.click(await screen.findByRole("button", { name: "Detach" }));
    await waitFor(() => expect(edits).toHaveLength(2));
    const placed = JSON.stringify(applied(edits[0] as EditCall));
    expect(placed).toContain('"text":"Mine"');
    expect(applied(edits[1] as EditCall).children.map((c) => c.id)).not.toContain(I1);
    expect(edits[0]?.options?.group).toBe(edits[1]?.options?.group);
  });
});

describe("[STU-084] instance overrides in the Properties panel", () => {
  const library: LibraryFixture = {
    items: [{ id: "c1", kind: "component", key: "card", name: "Card", visibility: "private", documentId: "cdoc" }],
    versions: { c1: [{ version: 1, content: '{"props":{"text":{"type":"string"}},"slots":{"body":{}}}' }] },
  };

  function mountOverrides(node: object, edits: EditCall[]) {
    const session = createSession("/api", createUpstream({ library }));
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    render(
      <I18nProvider studio={studio}>
        <AppContextProvider value={{ session, studio }}>
          <QueryClientProvider client={session.queryClient}>
            <InstanceOverrides
              node={node}
              doc="s1"
              canEdit
              edit={(doc, recipe, options) => edits.push({ doc, recipe, options })}
            />
          </QueryClientProvider>
        </AppContextProvider>
      </I18nProvider>,
    );
  }

  test("editing an exposed prop sets the override through the editing core, one group per burst", async () => {
    const edits: EditCall[] = [];
    mountOverrides(instance, edits);
    const field = await screen.findByRole("textbox", { name: "text" });
    fireEvent.change(field, { target: { value: "A" } });
    fireEvent.change(field, { target: { value: "AB" } });
    expect(edits[0]?.options?.group).toBe(edits[1]?.options?.group);
    const after = produce(tree(), (d) => edits[1]?.recipe(d)) as {
      children: { id: string; props?: { text?: string } }[];
    };
    expect(after.children.find((c) => c.id === I1)?.props?.text).toBe("AB");
    fireEvent.blur(field);
    fireEvent.change(field, { target: { value: "C" } });
    expect(edits[2]?.options?.group).not.toBe(edits[1]?.options?.group);
  });

  test("an overridden slot can be reset, and a plain node shows no section", async () => {
    const edits: EditCall[] = [];
    mountOverrides({ ...instance, slots: { body: [] } }, edits);
    fireEvent.click(await screen.findByRole("button", { name: "Reset body" }));
    const root = { id: N0, children: [{ ...instance, slots: { body: [] } }] };
    const after = produce(root, (d) => edits[0]?.recipe(d));
    expect(JSON.stringify(after)).not.toContain("body");
    cleanup();
    mountOverrides(text, edits);
    expect(screen.queryByText("Component overrides")).toBeNull();
  });
});

describe("[STU-055] start from template in the new-screen dialog", () => {
  test("offers blank and the page templates and reports the choice", async () => {
    const library: LibraryFixture = {
      items: [
        { id: "t1", kind: "template", key: "login", name: "Login", visibility: "private", templateKind: "page" },
        { id: "t2", kind: "template", key: "wiz", name: "Wizard", visibility: "private", templateKind: "flow" },
      ],
    };
    const session = createSession("/api", createUpstream({ library }));
    const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
    const chosen: string[] = [];
    render(
      <I18nProvider studio={studio}>
        <AppContextProvider value={{ session, studio }}>
          <QueryClientProvider client={session.queryClient}>
            <TemplateStartField value="" onChange={(id) => chosen.push(id)} />
          </QueryClientProvider>
        </AppContextProvider>
      </I18nProvider>,
    );
    fireEvent.click(await screen.findByRole("combobox"));
    await screen.findByRole("listbox");
    expect(screen.queryByRole("option", { name: "Wizard" })).toBeNull();
    const login = await screen.findByRole("option", { name: "Login" });
    fireEvent.pointerDown(login);
    fireEvent.pointerUp(login);
    fireEvent.mouseUp(login);
    fireEvent.click(login);
    await waitFor(() => expect(chosen).toEqual(["t1"]));
  });
});
