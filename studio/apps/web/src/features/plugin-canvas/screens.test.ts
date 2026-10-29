// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { blankPage, deleteBlockers, duplicatePage, type PageDoc, remapIds, takenNames, uuidv7 } from "./screens.ts";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
let n = 0;
const seq = () => `01e0c450-6c00-7000-8000-${String(++n).padStart(12, "0")}`;
const none = { keys: new Set<string>(), routes: new Set<string>() };

describe("[SCH-002] identifiers", () => {
  test("uuidv7 is canonical lower-case v7 with the time in front", () => {
    expect(uuidv7()).toMatch(uuid);
    expect(uuidv7(0x0123456789ab, () => new Uint8Array(16)).startsWith("01234567-89ab-7")).toBe(true);
  });
});

describe("[SCH-022] blank screen", () => {
  test("is a page with its own ids, a scaffold root, and key and route unique among the taken names", () => {
    const page = blankPage("Loan calculator", {
      keys: new Set(["loan-calculator"]),
      routes: new Set(["loan-calculator-2"]),
    });
    expect(page["kind"]).toBe("page");
    expect(page["id"]).toMatch(uuid);
    expect(page["key"]).toBe("loan-calculator-2");
    expect(page["route"]).toBe("loan-calculator-2-2");
    expect(page["pageKind"]).toBe("screen");
    expect((page["root"] as { id: string; type: string }).type).toBe("Scaffold");
    expect((page["root"] as { id: string }).id).not.toBe(page["id"]);
  });
  test("a title with nothing usable still gets a slug", () => {
    expect(blankPage("???", none)["key"]).toBe("screen");
  });
});

describe("[SCH-002] duplicate with ID remapping", () => {
  const source: PageDoc = {
    schemaVersion: "1.0.0",
    kind: "page",
    id: "01e0c450-6c00-7000-8000-00000000000a",
    key: "home",
    route: "home",
    pageKind: "screen",
    title: "Home",
    root: {
      id: "01e0c450-6c00-7000-8000-00000000000b",
      type: "Scaffold",
      props: { label: { $t: "01a14cd1-caec-7130-999e-81fbfc99a49a" } },
      children: [
        {
          id: "01e0c450-6c00-7000-8000-00000000000c",
          type: "Text",
          semantics: { labelledBy: "01e0c450-6c00-7000-8000-00000000000b" },
        },
      ],
    },
    a11yWaivers: { "01e0c450-6c00-7000-8000-00000000000c": "decorative" },
  };

  test("every node id is fresh, internal references follow, external ones stay", () => {
    n = 0;
    const copy = remapIds(source, seq) as typeof source & {
      root: { id: string; children: { id: string; semantics: { labelledBy: string } }[]; props: unknown };
    };
    const ids = [copy["id"], copy.root.id, copy.root.children[0]?.id];
    expect(new Set(ids).size).toBe(3);
    for (const id of ids) {
      expect([
        "01e0c450-6c00-7000-8000-00000000000a",
        "01e0c450-6c00-7000-8000-00000000000b",
        "01e0c450-6c00-7000-8000-00000000000c",
      ]).not.toContain(id);
    }
    expect(copy.root.children[0]?.semantics.labelledBy).toBe(copy.root.id);
    expect(Object.keys(copy["a11yWaivers"] as object)).toEqual([copy.root.children[0]?.id as string]);
    expect(copy.root.props).toEqual({ label: { $t: "01a14cd1-caec-7130-999e-81fbfc99a49a" } });
  });

  test("the source is untouched and the copy's key, route and title are made distinct", () => {
    const before = JSON.stringify(source);
    const copy = duplicatePage(source, takenNames([{ json: source }]), "(copy)");
    expect(JSON.stringify(source)).toBe(before);
    expect(copy["key"]).toBe("home-copy");
    expect(copy["route"]).toBe("home-copy");
    expect(copy["title"]).toBe("Home (copy)");
    expect(duplicatePage(source, takenNames([{ json: source }, { json: copy }]), "(copy)")["key"]).toBe("home-copy-2");
  });
});

describe("delete safety", () => {
  const go = (route: string) => ({
    id: "x",
    events: { onPressed: { steps: [{ action: "navigate", id: "s", input: { route } }] } },
  });
  const home: PageDoc = { id: "h", title: "Home", route: "home", root: go("detail") };
  const detail: PageDoc = { id: "d", title: "Detail", route: "detail", root: { id: "r" } };
  test("the entry page is refused", () => {
    expect(deleteBlockers(home, "h", [home, detail])).toEqual([{ kind: "entry" }]);
  });
  test("a page another page navigates to is refused and names that page", () => {
    expect(deleteBlockers(detail, "h", [home, detail])).toEqual([{ kind: "navigate", pageId: "h", title: "Home" }]);
  });
  test("an unreferenced page that is not the entry can go; its own navigate steps do not block it", () => {
    expect(deleteBlockers({ ...home, root: go("home") }, "other", [{ ...home, root: go("home") }, detail])).toEqual([]);
  });
});
