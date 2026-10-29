// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { limit } from "@plux/schema";
import {
  blankOptions,
  type Draft,
  initialDraft,
  inspectArchive,
  maxLocales,
  problemOf,
  stepsOf,
  themeOf,
} from "./draft.ts";
import { packArchive } from "./plux-archive.ts";

const draft = (patch: Partial<Draft>): Draft => ({ ...initialDraft, name: "Shop", key: "shop", ...patch });

describe("the steps and problems of the new-app wizard [STU-024]", () => {
  test("a blank app asks for languages, theme, security and environments; a template and an archive bring their own", () => {
    expect(stepsOf("blank")).toEqual(["source", "details", "locales", "theme", "security", "environments", "done"]);
    expect(stepsOf("template")).toEqual(["source", "details", "done"]);
    expect(stepsOf("import")).toEqual(["source", "details", "done"]);
  });

  test("a template or an archive must be chosen", () => {
    expect(problemOf(draft({ source: "template" }), "source")).toBe("template");
    expect(problemOf(draft({ source: "template", sampleId: "starter" }), "source")).toBeUndefined();
    expect(problemOf(draft({ source: "import" }), "source")).toBe("archive");
    const bad = { name: "x.zip", bytes: new Uint8Array(), issue: { kind: "extension" } as const };
    expect(problemOf(draft({ source: "import", archive: bad }), "source")).toBe("archive");
    expect(
      problemOf(draft({ source: "import", archive: { name: "x.plux", bytes: new Uint8Array() } }), "source"),
    ).toBeUndefined();
    expect(problemOf(draft({}), "source")).toBeUndefined();
  });

  test("the name and key are checked like the server checks them; an import may leave both empty", () => {
    expect(problemOf(draft({ name: "  " }), "details")).toBe("name");
    expect(problemOf(draft({ key: "-bad-" }), "details")).toBe("key");
    expect(problemOf(draft({}), "details")).toBeUndefined();
    expect(problemOf(draft({ source: "import", name: "", key: "" }), "details")).toBeUndefined();
    expect(problemOf(draft({ source: "import", name: "", key: "Bad" }), "details")).toBe("key");
    expect(problemOf(draft({ source: "import", name: "x".repeat(201), key: "" }), "details")).toBe("name");
  });

  test("languages: at least one, the default among them, at most the registry's app.locales", () => {
    expect(problemOf(draft({ locales: [] }), "locales")).toBe("locales");
    expect(problemOf(draft({ locales: ["de"], defaultLocale: "en" }), "locales")).toBe("locales");
    expect(problemOf(draft({}), "locales")).toBeUndefined();
    const many = Array.from({ length: maxLocales + 1 }, (_, i) => `x${i}`);
    expect(problemOf(draft({ locales: many, defaultLocale: "x0" }), "locales")).toBe("tooManyLocales");
  });

  test("a custom theme needs a valid colour; a ready-made one does not", () => {
    expect(problemOf(draft({ themeId: "custom", customPrimary: "blue" }), "theme")).toBe("colour");
    expect(problemOf(draft({ themeId: "custom", customPrimary: "#112233" }), "theme")).toBeUndefined();
    expect(problemOf(draft({ themeId: "blue" }), "theme")).toBeUndefined();
    expect(problemOf(draft({}), "security")).toBeUndefined();
  });

  test("the blank options and the theme follow the draft", () => {
    expect(themeOf(draft({ themeId: "green" })).name).toBe("Green");
    expect(themeOf(draft({ themeId: "custom", customPrimary: "#aabbcc" })).primary).toBe("#AABBCC");
    expect(blankOptions(draft({ name: " Shop ", description: " d " }))).toMatchObject({
      name: "Shop",
      description: "d",
    });
  });
});

describe("looking at a chosen archive before it is sent [STU-024]", () => {
  const app = { key: "a", name: "A", description: "", tags: [] };

  test("a good archive has no issue", async () => {
    const bytes = await packArchive(app, [{ path: "app.json", content: new Uint8Array([1]) }]);
    expect(inspectArchive("app.plux", bytes)).toBeUndefined();
    expect(inspectArchive("APP.PLUX", bytes)).toBeUndefined();
  });

  test("the extension, the ZIP form and the registry maxima are checked", async () => {
    const bytes = await packArchive(app, [{ path: "app.json", content: new Uint8Array([1]) }]);
    expect(inspectArchive("app.zip", bytes)).toEqual({ kind: "extension" });
    expect(inspectArchive("app.plux", new TextEncoder().encode("plain text, not a zip"))).toEqual({
      kind: "unreadable",
    });
    const max = limit("archive.bytes").max;
    expect(inspectArchive("app.plux", new Uint8Array(max + 1))).toEqual({ kind: "bytes", size: max + 1, max });
  });

  test("an archive with more entries or a larger expanded size than any organisation may import is refused", async () => {
    const many = await packArchive(
      app,
      Array.from({ length: limit("archive.entries").max + 1 }, (_, i) => ({
        path: `f${String(i).padStart(6, "0")}`,
        content: new Uint8Array(),
      })),
    );
    expect(inspectArchive("a.plux", many)).toMatchObject({ kind: "entries", max: limit("archive.entries").max });
    const small = await packArchive(app, [{ path: "a", content: new Uint8Array([1]) }]);
    const claimed = new Uint8Array(small);
    const view = new DataView(claimed.buffer);
    // The central directory of the second entry declares a huge size, which the server would count as read.
    const centre = view.getUint32(claimed.length - 22 + 16, true);
    const first = 46 + view.getUint16(centre + 28, true);
    view.setUint32(centre + first + 24, limit("archive.expandedBytes").max + 1, true);
    expect(inspectArchive("a.plux", claimed)).toMatchObject({
      kind: "expanded",
      max: limit("archive.expandedBytes").max,
    });
  });
});
