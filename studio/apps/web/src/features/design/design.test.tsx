// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, setDefaultTimeout, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { LibraryFixture } from "../../test-upstream.ts";
import { flattenTokens, parseDtcg, putToken, scriptCoverage, setMode, validName } from "./design-model.ts";

setDefaultTimeout(30000);
afterEach(cleanup);

const theme = {
  schemaVersion: 1,
  kind: "theme",
  id: "01a0c450-6c00-7014-8000-000000000c01",
  key: "main",
  name: "Main",
  tokens: {
    colors: {
      $type: "color",
      primary: { $value: "#0055ff", $extensions: { "dev.plux.dark": "#88aaff" } },
      surface: { $value: "#ffffff" },
    },
    typography: {
      latin: { $type: "fontFamily", $value: ["Inter", "sans-serif"] },
      arabic: { $type: "fontFamily", $value: "Noto Naskh" },
    },
    icons: { core: { home: { $value: "e001" } } },
  },
};
const appDocuments = [
  { kind: "theme", path: "theme.json", json: theme },
  { kind: "app", path: "app.json", json: { kind: "app", environments: [] } },
];
const state = { permissions: ["app.read"], documents: [], appDocuments, pluginDocuments: { pl: [] } };
const base = `/o/${orgA}/apps/app1/design`;

describe("pure helpers", () => {
  test("names, modes and tokens", () => {
    expect(validName("a.b-c")).toBe(true);
    expect(validName("__proto__")).toBe(false);
    const tokens: Record<string, unknown> = {};
    putToken(tokens, ["a", "b"], "1", "number");
    setMode(tokens, ["a", "b"], "contrast", "2");
    expect(flattenTokens(tokens)[0]?.type).toBe("number");
    setMode(tokens, ["a", "b"], "contrast", undefined);
    expect(JSON.stringify(tokens)).toBe('{"a":{"b":{"$value":"1","$type":"number"}}}');
    expect(scriptCoverage(flattenTokens(theme.tokens)).latin).toBe(2);
  });

  test("a DTCG file is data: types and names are checked and what is left out is reported", () => {
    const ok = parseDtcg(
      JSON.stringify({
        c: {
          $type: "color",
          a: { $value: "#fff" },
          b: { $value: 5, $type: "number" },
          bad: { $value: 1, $type: "color" },
        },
        __proto__: { x: { $value: "1", $type: "number" } },
        "bad name": { y: { $value: "1", $type: "number" } },
        u: { z: { $value: "1", $type: "magic" } },
      }),
    );
    expect(ok.ok).toBe(true);
    if (ok.ok) {
      expect(ok.imported).toBe(2);
      expect(ok.skipped.map((s) => s.reason).sort()).toEqual(["name", "type", "value"]);
      expect(Object.getPrototypeOf(ok.tokens)).toBe(Object.prototype);
    }
    expect(parseDtcg("{")).toEqual({ ok: false, error: "notJson" });
    expect(parseDtcg("[]")).toEqual({ ok: false, error: "notObject" });
    expect(parseDtcg(" ".repeat(20_000_000))).toEqual({ ok: false, error: "tooLarge" });
  });
});

describe("STU-112 A15 tokens and D57, D58", () => {
  test("lists the tokens, edits one in every mode and imports DTCG tokens", async () => {
    renderApp(base, state);
    expect(await screen.findByText("colors.primary", {}, { timeout: 10000 })).toBeTruthy();
    expect(screen.queryByText("typography.latin")).toBeNull();
    const edit = screen.getByRole("button", { name: "Edit colors.primary" });
    await waitFor(() => expect((edit as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(edit);
    fireEvent.change(await screen.findByLabelText("High contrast value"), { target: { value: "#000000" } });
    await waitFor(() => expect(screen.getAllByText("#000000").length).toBeGreaterThan(0));
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: "Import tokens" }));
    fireEvent.change(await screen.findByLabelText(/Design tokens file/), {
      target: {
        value: JSON.stringify({
          space: { $type: "dimension", m: { $value: "8px" }, x: { $value: "1", $type: "nope" } },
        }),
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Read the file" }));
    expect(await screen.findByText("1 tokens can be added, 1 are skipped.")).toBeTruthy();
    expect(screen.getByText("space.x")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add to the theme" }));
    expect(await screen.findByText("The tokens were added to the theme.")).toBeTruthy();
  });
});

describe("STU-112 A16 to A18", () => {
  test("themes count the colours of each mode", async () => {
    renderApp(`${base}/themes`, state);
    expect(await screen.findByText("Light: 2 colours", {}, { timeout: 10000 })).toBeTruthy();
    expect(screen.getByText("Dark: 1 colours")).toBeTruthy();
    expect(screen.getByText("No brand overlays yet.")).toBeTruthy();
  });

  test("a brand overlay is created from a base token", async () => {
    renderApp(`${base}/themes`, state);
    const open = await screen.findByRole("button", { name: "Create a brand overlay" }, { timeout: 10000 });
    await waitFor(() => expect((open as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(open);
    fireEvent.change(await screen.findByLabelText("Brand name"), { target: { value: "acme" } });
    fireEvent.change(screen.getByLabelText("Value for the brand"), { target: { value: "#ff0000" } });
    fireEvent.click(screen.getByRole("button", { name: "Create overlay" }));
    expect(await screen.findByText("acme: 1 overrides")).toBeTruthy();
  });

  test("the brand overlay dialog asks for a name without dots, a token and a value before it creates anything", async () => {
    renderApp(`${base}/themes`, state);
    const open = await screen.findByRole("button", { name: "Create a brand overlay" }, { timeout: 10000 });
    await waitFor(() => expect((open as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(open);
    const dialog = await screen.findByRole("dialog");
    const create = () => within(dialog).getByRole("button", { name: "Create overlay" }) as HTMLButtonElement;
    const disabled = () => create().disabled || create().getAttribute("aria-disabled") === "true";
    expect(disabled()).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("Value for the brand"), { target: { value: "#ff0000" } });
    expect(disabled()).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("Brand name"), { target: { value: "ac.me" } });
    expect(disabled()).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("Brand name"), { target: { value: "__proto__" } });
    expect(disabled()).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("Brand name"), { target: { value: "acme" } });
    expect(disabled()).toBe(false);
    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    expect(screen.getByText("No brand overlays yet.")).toBeTruthy();
  });

  test("an icon is not added without a valid set and name and a source", async () => {
    renderApp(`${base}/icons`, state);
    await screen.findByText("home", {}, { timeout: 10000 });
    const set = screen.getByLabelText("Icon set");
    await waitFor(() => expect((set as HTMLInputElement).disabled).toBe(false));
    const add = () => screen.getByRole("button", { name: "Add icon" }) as HTMLButtonElement;
    const disabled = () => add().disabled || add().getAttribute("aria-disabled") === "true";
    fireEvent.change(set, { target: { value: "core" } });
    fireEvent.change(screen.getByLabelText("Icon name"), { target: { value: "bad name" } });
    fireEvent.change(screen.getByLabelText(/Icon source/), { target: { value: "e002" } });
    expect(disabled()).toBe(true);
    fireEvent.change(screen.getByLabelText("Icon name"), { target: { value: "search" } });
    fireEvent.change(screen.getByLabelText(/Icon source/), { target: { value: "" } });
    expect(disabled()).toBe(true);
    fireEvent.change(screen.getByLabelText(/Icon source/), { target: { value: "e002" } });
    expect(disabled()).toBe(false);
  });

  test("the server refusing the autosave of a brand overlay is reported and the overlay is undone", async () => {
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(`${base}/themes`, { ...state, sent, patchRefusal: { message: "not allowed", paths: ["theme.json"] } });
    const open = await screen.findByRole("button", { name: "Create a brand overlay" }, { timeout: 10000 });
    await waitFor(() => expect((open as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(open);
    fireEvent.change(await screen.findByLabelText("Brand name"), { target: { value: "acme" } });
    fireEvent.change(screen.getByLabelText("Value for the brand"), { target: { value: "#ff0000" } });
    fireEvent.click(screen.getByRole("button", { name: "Create overlay" }));
    const alert = await screen.findByRole("alert", {}, { timeout: 10000 });
    expect(alert.textContent).toBe("The server refused this change. It was undone.");
    expect(sent?.some((s) => s.path === "theme.json")).toBe(true);
    await waitFor(() => expect(screen.queryByText("acme: 1 overrides") === null).toBe(true));
    expect(screen.getByText("No brand overlays yet.")).toBeTruthy();
  });

  test("typography flags a script without a fallback (THM-004)", async () => {
    renderApp(`${base}/typography`, state);
    expect(await screen.findByText("Latin: 2 fonts", {}, { timeout: 10000 })).toBeTruthy();
    expect(screen.getByText("Arabic has no fallback font.")).toBeTruthy();
    expect(screen.getByText("Cyrillic has no fallback font.")).toBeTruthy();
  });

  test("icons lists the sets and adds an icon", async () => {
    renderApp(`${base}/icons`, state);
    expect(await screen.findByText("home", {}, { timeout: 10000 })).toBeTruthy();
    const set = screen.getByLabelText("Icon set");
    await waitFor(() => expect((set as HTMLInputElement).disabled).toBe(false));
    fireEvent.change(set, { target: { value: "core" } });
    fireEvent.change(screen.getByLabelText("Icon name"), { target: { value: "search" } });
    fireEvent.change(screen.getByLabelText(/Icon source/), { target: { value: "e002" } });
    fireEvent.click(screen.getByRole("button", { name: "Add icon" }));
    expect(await screen.findByText("search")).toBeTruthy();
  });
});

describe("STU-112 A18 icon form: server refusal", () => {
  test("the server refusing the autosave of an icon is reported and the icon is undone", async () => {
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(`${base}/icons`, { ...state, sent, patchRefusal: { message: "not allowed" } });
    await screen.findByText("home", {}, { timeout: 10000 });
    const set = screen.getByLabelText("Icon set");
    await waitFor(() => expect((set as HTMLInputElement).disabled).toBe(false));
    fireEvent.change(set, { target: { value: "core" } });
    fireEvent.change(screen.getByLabelText("Icon name"), { target: { value: "search" } });
    fireEvent.change(screen.getByLabelText(/Icon source/), { target: { value: "e002" } });
    fireEvent.click(screen.getByRole("button", { name: "Add icon" }));
    const alert = await screen.findByRole("alert", {}, { timeout: 10000 });
    expect(alert.textContent).toBe("The server refused this change. It was undone.");
    expect(sent?.length).toBeGreaterThan(0);
    await waitFor(() => expect(screen.queryByText("search") === null).toBe(true));
    expect(screen.getByText("home")).toBeTruthy();
  });
});

describe("STU-112 A19 component library", () => {
  test("lists the library components with the version this app adopted", async () => {
    renderApp(`${base}/library`, {
      ...state,
      library: {
        items: [
          { id: "c1", kind: "component", key: "card", name: "Card", visibility: "organization", createdByName: "Ada" },
        ],
      },
    });
    expect(await screen.findByText("Card", {}, { timeout: 10000 })).toBeTruthy();
    expect(screen.getByText("Not adopted")).toBeTruthy();
  });
});

describe("STU-112 STU-085 A19 adopt dialog (D32)", () => {
  const library = (more: Partial<LibraryFixture> = {}): LibraryFixture => ({
    items: [
      { id: "c1", kind: "component", key: "card", name: "Card", visibility: "organization", createdByName: "Ada" },
    ],
    versions: {
      c1: [
        { version: 1, content: '{"props":{"a":{},"b":{}},"slots":{"icon":{}}}' },
        { version: 2, content: '{"props":{"a":{},"c":{}}}' },
      ],
    },
    adoptions: [{ itemId: "c1", version: 1 }],
    conflicts: [{ file: "plugins/p/pages/home.json", path: "/root/0", removedProps: ["b"] }],
    calls: [],
    ...more,
  });

  const openDialog = async (lib: LibraryFixture, adopted = 1) => {
    renderApp(`${base}/library`, { ...state, library: lib });
    const open = await screen.findByRole("button", { name: "Adopt a version of Card" }, { timeout: 10000 });
    await waitFor(() => expect((open as HTMLButtonElement).disabled).toBe(false));
    expect(screen.getByText(`Version ${adopted}`)).toBeTruthy();
    fireEvent.click(open);
    return screen.findByRole("dialog");
  };

  test("adopts the newest version, shows what its interface changes and flags the overrides of removed props", async () => {
    const lib = library();
    const dialog = await openDialog(lib);
    await within(dialog).findByText("c", { selector: "code" });
    expect(within(dialog).getByText("b", { selector: "code" })).toBeTruthy();
    expect(within(dialog).getByText("icon", { selector: "code" })).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Adopt version" }));
    await within(dialog).findByText("Version 2 is adopted.");
    expect(within(dialog).getByText("plugins/p/pages/home.json/root/0")).toBeTruthy();
    expect(lib.calls).toEqual(["adopt:app1:2"]);
    fireEvent.click(within(dialog).getAllByRole("button", { name: "Close" })[0] as HTMLElement);
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  });

  test("the version this app already adopted cannot be adopted again", async () => {
    const lib = library({ adoptions: [{ itemId: "c1", version: 2 }] });
    const dialog = await openDialog(lib, 2);
    const confirm = within(dialog).getByRole("button", { name: "Adopt version" }) as HTMLButtonElement;
    expect(confirm.disabled || confirm.getAttribute("aria-disabled") === "true").toBe(true);
    fireEvent.click(confirm);
    expect(lib.calls).toEqual([]);
  });

  test("a refusal by the server is reported in the dialog and nothing is shown as adopted", async () => {
    const lib = library({ adoptRefusal: "the component is no longer visible to this app" });
    const dialog = await openDialog(lib);
    fireEvent.click(within(dialog).getByRole("button", { name: "Adopt version" }));
    expect(await within(dialog).findByRole("alert")).toBeTruthy();
    expect(within(dialog).queryByText("Version 2 is adopted.") === null).toBe(true);
    expect(lib.calls).toEqual(["adopt:app1:2"]);
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  });

  test("a reader of the app cannot open the dialog", async () => {
    renderApp(`${base}/library`, { ...state, permissions: ["app.read"], lockHeldElsewhere: true, library: library() });
    const open = await screen.findByRole("button", { name: "Adopt a version of Card" }, { timeout: 10000 });
    expect((open as HTMLButtonElement).disabled || open.getAttribute("aria-disabled") === "true").toBe(true);
  });
});

describe("STU-006 host app fonts", () => {
  const withFonts = (extra: Record<string, unknown>) => ({
    ...state,
    permissions: ["app.read", "app.manage"],
    fontAssets: ["fonts/Brand.ttf", "fonts/Serif.ttf"],
    ...extra,
  });

  test("the typography page lists the host fonts with their files", async () => {
    renderApp(
      `${base}/typography`,
      withFonts({ hostFonts: [{ family: "Brand Sans", assetId: "font-0" }] }) as typeof state,
    );
    expect(await screen.findByText("Brand Sans", {}, { timeout: 10000 })).toBeTruthy();
    expect(screen.getByText("fonts/Brand.ttf")).toBeTruthy();
  });

  test("a font asset is listed under a family and then removed", async () => {
    const data = withFonts({}) as typeof state & { hostFonts?: { family: string; assetId: string }[] };
    renderApp(`${base}/typography`, data);
    expect(await screen.findByText("No host fonts are listed.", {}, { timeout: 10000 })).toBeTruthy();
    fireEvent.change(await screen.findByLabelText("Font family"), { target: { value: "Brand Serif" } });
    fireEvent.click(await screen.findByRole("combobox", { name: "Font file" }));
    const opt = await screen.findByRole("option", { name: "fonts/Serif.ttf" });
    // Base UI ignores a pick made at once after the popup opens: wait, then press and release as a pointer does.
    await new Promise((r) => setTimeout(r, 400));
    fireEvent.pointerDown(opt);
    fireEvent.mouseDown(opt);
    fireEvent.pointerUp(opt);
    fireEvent.mouseUp(opt);
    fireEvent.click(opt);
    await waitFor(() => expect(screen.getAllByRole("combobox").at(-1)?.textContent).toBe("fonts/Serif.ttf"));
    fireEvent.click(screen.getByRole("button", { name: "Add font" }));
    expect(await screen.findByText("Brand Serif")).toBeTruthy();
    expect(data.hostFonts).toEqual([{ family: "Brand Serif", assetId: "font-1" }]);
    fireEvent.click(screen.getByRole("button", { name: "Remove Brand Serif" }));
    await waitFor(() => expect(data.hostFonts).toEqual([]));
    expect(await screen.findByText("No host fonts are listed.")).toBeTruthy();
  });

  test("without permission to manage the app the list is read-only", async () => {
    renderApp(`${base}/typography`, withFonts({ permissions: ["app.read"] }) as typeof state);
    expect(
      await screen.findByText("You need permission to manage this app to change the list.", {}, { timeout: 10000 }),
    ).toBeTruthy();
    expect((await screen.findByRole("button", { name: "Add font" })).getAttribute("aria-disabled")).toBe("true");
  });
});
