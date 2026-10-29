// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, setDefaultTimeout, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import { localeProblem } from "./area-actions.ts";
import { filterKeys, matchKeys, missingByPlugin, screensByKey, statusOf } from "./grid-model.ts";
import {
  csvCell,
  csvRecords,
  exportTranslations,
  FORMATS,
  formatOfName,
  parseTranslations,
  type TranslationFormat,
} from "./translation-formats.ts";

setDefaultTimeout(30000);
afterEach(cleanup);

const BIG = 1 << 20;
const samples: Record<TranslationFormat, string> = {
  json: '{"home.title":"Willkommen","home.cta":"Los"}',
  arb: '{"@@locale":"de","home.title":"Willkommen","@home.title":{"description":"x"},"home.cta":"Los"}',
  csv: 'key,de\nhome.title,Willkommen\n"home.cta","Los"\n',
  xliff:
    '<?xml version="1.0"?><xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="de"><file id="f"><unit id="home.title"><segment><source>Welcome</source><target>Willkommen</target></segment></unit><unit id="home.cta"><segment><source>Go</source><target>Los</target></segment></unit></file></xliff>',
};

describe("[I18N-009] translation formats", () => {
  test.each([...FORMATS])("reads a %s file as data", (format) => {
    const r = parseTranslations(format, samples[format], BIG);
    expect(r.ok && r.entries).toEqual({ "home.title": "Willkommen", "home.cta": "Los" });
  });

  test.each([...FORMATS])("rejects a malformed %s file", (format) => {
    const bad: Record<TranslationFormat, string> = {
      json: "{not json",
      arb: '["a"]',
      csv: 'key,de\n"unterminated,x\n',
      xliff: "<xliff><unit id=",
    };
    expect(parseTranslations(format, bad[format], BIG)).toEqual({ ok: false, reason: "malformed" });
  });

  test("reads XLIFF's XML: prefixes, comments, CDATA, references and inline elements", () => {
    const x = (body: string) =>
      `<?xml version="1.0"?>\n<!-- export --><x:xliff xmlns:x="urn:oasis:names:tc:xliff:document:2.0" version='2.0'><x:file id="f">${body}</x:file></x:xliff>\n`;
    const r = parseTranslations(
      "xliff",
      x(
        '<x:unit id="a"><x:segment><x:source>A</x:source><x:target>Fish &amp; chips &#x263A;&#65;</x:target></x:segment></x:unit>' +
          '<x:unit id="b"><x:segment><x:target><![CDATA[<b>bold</b>]]> and <pc id="1">inline</pc></x:target><x:target>second</x:target></x:segment></x:unit>' +
          '<x:unit id="c"><x:segment><x:source>no target</x:source></x:segment></x:unit>' +
          '<x:unit id="d"><x:segment><x:target/></x:segment></x:unit>',
      ),
      BIG,
    );
    expect(r.ok && r.entries).toEqual({ a: "Fish & chips \u263AA", b: "<b>bold</b> and inline", d: "" });
  });

  test.each([
    ["mismatched tags", '<xliff><unit id="a"><target>x</unit></target></xliff>'],
    ["an unclosed root", '<xliff><unit id="a"><target>x</target></unit>'],
    ["another root", '<other><unit id="a"><target>x</target></unit></other>'],
    ["two roots", "<xliff></xliff><xliff></xliff>"],
    ["a unit without id", "<xliff><unit><target>x</target></unit></xliff>"],
    ["an unknown entity", '<xliff><unit id="a"><target>&nbsp;</target></unit></xliff>'],
    ["a reference without semicolon", '<xliff><unit id="a"><target>&amp</target></unit></xliff>'],
    ["a document type", '<!DOCTYPE xliff [<!ENTITY e "x">]><xliff><unit id="a"><target>&e;</target></unit></xliff>'],
    ["text outside the root", 'text<xliff><unit id="a"><target>x</target></unit></xliff>'],
  ])("rejects XLIFF with %s", (_name, text) => {
    expect(parseTranslations("xliff", text, BIG)).toEqual({ ok: false, reason: "malformed" });
  });

  test("rejects a file over the size cap, and one without entries", () => {
    expect(parseTranslations("json", samples.json, 5)).toEqual({ ok: false, reason: "tooLarge" });
    expect(parseTranslations("json", "{}", BIG)).toEqual({ ok: false, reason: "empty" });
    expect(parseTranslations("json", '{"a":1}', BIG).ok).toBe(false);
  });

  test("keeps markup as text and does not pollute prototypes", () => {
    const r = parseTranslations("json", '{"__proto__":"x","a":"<img src=x onerror=alert(1)>"}', BIG);
    expect(r.ok && r.entries["a"]).toBe("<img src=x onerror=alert(1)>");
    expect(({} as Record<string, unknown>)["x"]).toBeUndefined();
  });

  test.each([...FORMATS])("exports %s and reads it back", (format) => {
    const text = exportTranslations(format, {
      locale: "de",
      sourceLocale: "en",
      keys: ["a.b", "c.d"],
      source: { "a.b": "A", "c.d": "C" },
      target: { "a.b": '=1+1, "q" <b> & more', "c.d": "multi\nline" },
    });
    const r = parseTranslations(format, text, BIG);
    expect(r.ok && r.entries).toEqual({ "a.b": '=1+1, "q" <b> & more', "c.d": "multi\nline" });
  });

  test("names formats, quotes cells and defuses formulas", () => {
    expect(formatOfName("de.XLF")).toBe("xliff");
    expect(formatOfName("de.txt")).toBeUndefined();
    expect(csvCell("=SUM(A1)")).toBe("'=SUM(A1)");
    expect(csvRecords('a,"b ""c"""\r\nd,e')).toEqual([
      ["a", 'b "c"'],
      ["d", "e"],
    ]);
  });
});

describe("grid model", () => {
  const keys = [
    { id: "k1", key: "home.title", description: "" },
    { id: "k2", key: "home.cta", description: "button" },
  ];
  test("statuses, filters, matching and the heatmap counts", () => {
    expect(statusOf("k1", { k1: "x" }, {})).toBe("translated");
    expect(statusOf("k1", { k1: "x" }, { k1: "approved" })).toBe("approved");
    expect(statusOf("k1", {}, {})).toBe("missing");
    const cell = (id: string) => (id === "k1" ? "approved" : "missing");
    const texts = (id: string) => (id === "k1" ? ["Welcome"] : []);
    const all = { status: "all", query: "", used: undefined } as const;
    expect(filterKeys(keys, { ...all, status: "missing" }, cell, texts).map((k) => k.id)).toEqual(["k2"]);
    expect(filterKeys(keys, { ...all, query: "welcome" }, cell, texts).map((k) => k.id)).toEqual(["k1"]);
    expect(filterKeys(keys, { ...all, query: "BUTTON" }, cell, texts).map((k) => k.id)).toEqual(["k2"]);
    expect(filterKeys(keys, { ...all, used: new Set(["k2"]) }, cell, texts).map((k) => k.id)).toEqual(["k2"]);
    expect(matchKeys({ "home.cta": "Los", nope: "x" }, keys)).toEqual({ matched: { k2: "Los" }, unknown: ["nope"] });
    expect(missingByPlugin(new Map([["pl", new Set(["k1", "k2"])]]), ["de"], new Map([["de", { k1: "a" }]]))).toEqual([
      { plugin: "pl", total: 2, missing: { de: 1 } },
    ]);
    expect(screensByKey([{ path: "pages/home.page.json", json: { a: { $t: "k1" } } }]).get("k1")).toEqual(["home"]);
    expect(localeProblem("de", ["en"])).toBeUndefined();
    expect(localeProblem("de", ["de"])).toBe("taken");
    expect(localeProblem("Deutsch!", [])).toBe("invalid");
  });
});

const base = `/o/${orgA}/apps/app1/localisation`;
const state = (permissions: string[] = ["app.read", "translations.edit", "translations.approve"]) => ({
  permissions,
  assets: ["l10n-context/screen-home.png"],
  documents: [],
  appDocuments: [
    {
      kind: "app",
      path: "app.json",
      json: { kind: "app", defaultLocale: "en", supportedLocales: ["en", "de", "ar"], schemaVersion: 1 },
    },
    {
      kind: "translationKeys",
      path: "translations/keys.json",
      json: {
        kind: "translationKeys",
        keys: [
          { id: "k1", key: "home.title" },
          { id: "k2", key: "home.cta", description: "main button" },
        ],
      },
    },
    {
      kind: "translations",
      path: "translations/en.json",
      json: { kind: "translations", locale: "en", messages: { k1: "Welcome", k2: "Go" } },
    },
    {
      kind: "translations",
      path: "translations/de.json",
      json: { kind: "translations", locale: "de", messages: { k1: "Willkommen" } },
    },
  ],
  pluginDocuments: {
    pl: [
      {
        kind: "page",
        path: "pages/home.page.json",
        json: { kind: "page", root: { id: "n1", type: "text", text: { $t: "k1" }, other: { $t: "k2" } } },
      },
    ],
  },
});
const off = (el: HTMLElement) => el.matches(":disabled, [aria-disabled=true]");
const ready = () => screen.findByRole("table", { name: "Translation keys by locale" }, { timeout: 10000 });

describe("[STU-111] A13 localisation grid", () => {
  test("lists keys with source, translation, status and screenshot context, and filters them", async () => {
    renderApp(`${base}?locale=de`, state());
    const table = await ready();
    expect(within(table).getByText("home.title")).toBeTruthy();
    await screen.findByText("Translation (de)");
    expect(within(table).getAllByText("Screenshot: home").length).toBe(2);
    expect(within(table).getByText("Missing")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Search keys and texts"), { target: { value: "willkommen" } });
    await waitFor(() => expect(within(table).queryByText("home.cta") === null).toBe(true));
    expect(within(table).getByText("home.title")).toBeTruthy();
  });

  test("approves the selected messages after a confirmation (D56)", async () => {
    renderApp(`${base}?locale=de`, state());
    const table = await ready();
    await screen.findByText("Translation (de)");
    const approve = screen.getByRole("button", { name: "Approve" });
    await waitFor(() => expect(off(approve)).toBe(true));
    fireEvent.click(within(table).getByRole("checkbox", { name: "Select home.title" }));
    await waitFor(() => expect(off(approve)).toBe(false));
    fireEvent.click(approve);
    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText("Apply this to 1 messages?")).toBeTruthy();
    expect(within(table).queryByText("Approved")).toBeNull();
    fireEvent.click(within(dialog).getByRole("button", { name: "Apply" }));
    await waitFor(() => expect(within(table).getByText("Approved")).toBeTruthy());
  });

  test("keeps approving behind translations.approve", async () => {
    renderApp(`${base}?locale=de`, state(["app.read", "translations.edit"]));
    const table = await ready();
    await screen.findByText("Translation (de)");
    fireEvent.click(within(table).getByRole("checkbox", { name: "Select home.title" }));
    expect(off(screen.getByRole("button", { name: "Approve" }))).toBe(true);
    await screen.findByText("Approving needs the translations.approve permission.");
  });

  test("imports a file of each format into the locale and rejects a bad one (D54)", async () => {
    renderApp(`${base}?locale=de`, state());
    const table = await ready();
    await screen.findByText("Translation (de)");
    fireEvent.click(screen.getByRole("button", { name: "Import translations" }));
    const dialog = await screen.findByRole("dialog");
    const input = within(dialog).getByLabelText("File");
    fireEvent.change(input, { target: { files: [new File(["{broken"], "x.json")] } });
    await within(dialog).findByText("The file is not a valid file of this format.");
    expect(off(within(dialog).getByRole("button", { name: "Import" }))).toBe(true);
    const preview = "2 messages will be imported; 0 keys of the file are not keys of this app and are skipped.";
    for (const [name, text] of [
      ["de.json", samples.json],
      ["de.arb", samples.arb],
      ["de.csv", samples.csv],
      ["de.xlf", samples.xliff],
    ] as const) {
      fireEvent.change(input, { target: { files: [new File([text], name)] } });
      await within(dialog).findByText(preview, {}, { timeout: 5000 });
    }
    fireEvent.click(within(dialog).getByRole("button", { name: "Import" }));
    await waitFor(() => expect(within(table).getByText("Los")).toBeTruthy());
  });

  test("exports the locale as text (D55)", async () => {
    renderApp(`${base}?locale=de`, state());
    await ready();
    await screen.findByText("Translation (de)");
    fireEvent.click(screen.getByRole("button", { name: "Export translations" }));
    const dialog = await screen.findByRole("dialog");
    const out = within(dialog).getByRole("textbox") as HTMLTextAreaElement;
    expect(JSON.parse(out.value)).toEqual({ "home.title": "Willkommen", "home.cta": "" });
  });

  test("adds a locale (D53)", async () => {
    renderApp(`${base}?locale=de`, state());
    await ready();
    await screen.findByText("Translation (de)");
    fireEvent.click(screen.getByRole("button", { name: "Add a locale" }));
    const add = await screen.findByRole("dialog");
    fireEvent.change(within(add).getByLabelText("Locale code"), { target: { value: "Deutsch!" } });
    fireEvent.click(within(add).getByRole("button", { name: "Add" }));
    await within(add).findByText("Enter a locale code such as de or pt-BR.");
    fireEvent.change(within(add).getByLabelText("Locale code"), { target: { value: "fr" } });
    fireEvent.click(within(add).getByRole("button", { name: "Add" }));
    await screen.findByText("Translation (fr)");
  });
});

describe("[STU-111] A14 heatmap", () => {
  test("shows how many keys each plugin lacks per locale", async () => {
    renderApp(`${base}/heatmap`, state());
    const table = await screen.findByRole(
      "table",
      { name: "Missing translations by plugin and locale" },
      { timeout: 10000 },
    );
    expect(within(table).getByText("pl")).toBeTruthy();
    expect(within(table).getByText("1 of 2 missing")).toBeTruthy();
    expect(within(table).getByText("2 of 2 missing")).toBeTruthy();
    expect(within(table).getByText("0 of 2 missing")).toBeTruthy();
  });
});
