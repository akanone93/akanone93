// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { createRouterTransport } from "@connectrpc/connect";
import { type ApplyChangeSetRequest, ApplyChangeSetResponseSchema, DocumentService } from "@plux/api";
import { applyPatch, type JsonValue, PatchError } from "../editor/json-patch.ts";
import {
  applyChangeSet,
  type ChangeSet,
  moveScreen,
  redirectIncoming,
  removeOutgoing,
  undoStep,
} from "./change-set.ts";
import {
  buildWhereUsed,
  checkEntityDelete,
  checkPluginDelete,
  checkScreenDelete,
  checkStateDelete,
  componentKeyEffects,
  isBlocked,
  type LoadedDoc,
  routeChangeEffects,
} from "./where-used.ts";

const nav = (route: string) => ({ id: `go-${route}`, action: "navigate", input: { route } });

function page(id: string, key: string, steps: object[], extra: object = {}) {
  return {
    kind: "page",
    id,
    key,
    route: key,
    title: key,
    root: { id: `${id}-root`, type: "Button", events: { onPressed: { steps } }, ...extra },
  };
}

function docs(): LoadedDoc[] {
  return [
    { pluginId: "", path: "app.json", kind: "app", revision: 1, json: { kind: "app", entryRoute: "home" } },
    {
      pluginId: "L",
      path: "plugins/loans/plugin.json",
      kind: "plugin",
      revision: 2,
      json: { kind: "plugin", key: "loans", entryPage: "p-home", pages: ["p-home", "p-offers"] },
    },
    {
      pluginId: "C",
      path: "plugins/cards/plugin.json",
      kind: "plugin",
      revision: 3,
      json: { kind: "plugin", key: "cards", entryPage: "p-cards", pages: ["p-cards"] },
    },
    {
      pluginId: "L",
      path: "plugins/loans/pages/home.page.json",
      kind: "page",
      revision: 4,
      json: page("p-home", "home", [nav("offers"), nav("cards-home")]),
    },
    {
      pluginId: "L",
      path: "plugins/loans/pages/offers.page.json",
      kind: "page",
      revision: 5,
      json: page("p-offers", "offers", [], { component: { id: "c-1", version: 1 }, icon: { $asset: "a-1" } }),
    },
    {
      pluginId: "C",
      path: "plugins/cards/pages/cards-home.page.json",
      kind: "page",
      revision: 6,
      json: page("p-cards", "cards-home", [nav("offers"), nav("home")]),
    },
    {
      pluginId: "L",
      path: "plugins/loans/components/card.component.json",
      kind: "component",
      revision: 7,
      json: { kind: "component", id: "c-1", key: "card", exported: true },
    },
  ];
}

/** Applies a change set to the loaded documents, as the server would, all or nothing. */
function applyAll(all: readonly LoadedDoc[], changeSet: ChangeSet): LoadedDoc[] {
  let out = [...all];
  for (const draft of changeSet) {
    for (const e of draft.edits) {
      if ("delete" in e) {
        out = out.filter((d) => d.path !== e.path);
      } else if ("content" in e) {
        out.push({ pluginId: draft.pluginId, path: e.path, kind: "page", revision: 1, json: e.content });
      } else {
        out = out.map((d) => (d.path === e.path ? { ...d, json: applyPatch(d.json as JsonValue, e.patch) } : d));
      }
    }
  }
  return out;
}

describe("[SCH-041] the where-used index (F13)", () => {
  test("indexes navigation within and across plugins, components, assets and entries", () => {
    const index = buildWhereUsed(docs());
    expect(index.usesOf("page", "p-offers").map((u) => u.fromId)).toEqual(["p-home", "p-cards"]);
    expect(index.usesOf("component", "c-1")[0]?.pointer).toBe("/root/component/id");
    expect(index.usesOf("asset", "a-1")[0]?.pointer).toBe("/root/icon/$asset");
    expect(index.incomingToPlugin("L").map((u) => u.pointer)).toEqual([
      "/root/events/onPressed/steps/0/input/route",
      "/root/events/onPressed/steps/1/input/route",
    ]);
    expect(index.outgoingFromPlugin("L").map((u) => u.targetPluginId)).toEqual(["C"]);
  });
});

describe("[STU-070] [STU-071] [STU-072] blocked deletes (D19, D24, D26)", () => {
  test("a screen with incoming links or that is an entry is blocked", () => {
    const index = buildWhereUsed(docs());
    const offers = checkScreenDelete(index, "p-offers");
    expect(offers.entry).toBe(false);
    expect(offers.incoming.map((u) => u.fromTitle)).toEqual(["home", "cards-home"]);
    expect(isBlocked(offers)).toBe(true);
    expect(checkScreenDelete(index, "p-home").entry).toBe(true);
  });

  test("a plugin is blocked by links entering or leaving it and by the app's entry route", () => {
    const index = buildWhereUsed(docs());
    const loans = checkPluginDelete(index, "L");
    expect(loans.entry).toBe(true);
    expect(loans.incoming).toHaveLength(2);
    expect(loans.outgoing).toHaveLength(1);
    expect(checkPluginDelete(index, "C").entry).toBe(false);
  });

  test("components and assets are blocked while used", () => {
    const index = buildWhereUsed(docs());
    expect(isBlocked(checkEntityDelete(index, "component", "c-1"))).toBe(true);
    expect(isBlocked(checkEntityDelete(index, "asset", "a-2"))).toBe(false);
  });

  test("redirecting incoming links clears the block, guarded by test operations", () => {
    const all = docs();
    const index = buildWhereUsed(all);
    const offers = index.pages.get("p-offers");
    if (offers === undefined) {
      throw new Error("no offers page");
    }
    const fix = redirectIncoming(all, checkScreenDelete(index, "p-offers").incoming, offers, "home");
    expect(fix.map((d) => d.pluginId).sort()).toEqual(["C", "L"]);
    const after = buildWhereUsed(applyAll(all, fix));
    expect(isBlocked(checkScreenDelete(after, "p-offers"))).toBe(false);
    // A document changed since it was read fails its test instead of being overwritten.
    const edit = fix[0]?.edits[0];
    if (edit === undefined || !("patch" in edit)) {
      throw new Error("no patch");
    }
    const moved = { root: { events: { onPressed: { steps: [nav("elsewhere"), nav("elsewhere")] } } } };
    expect(() => applyPatch(moved, edit.patch)).toThrow(PatchError);
  });

  test("removing outgoing links removes each step, later ones first", () => {
    const all = docs();
    const extra = all.map((d) =>
      d.path.endsWith("home.page.json")
        ? { ...d, json: page("p-home", "home", [nav("cards-home"), nav("offers"), nav("cards-home")]) }
        : d,
    );
    const index = buildWhereUsed(extra);
    const fix = removeOutgoing(extra, index.outgoingFromPlugin("L"));
    const after = buildWhereUsed(applyAll(extra, fix));
    expect(after.outgoingFromPlugin("L")).toHaveLength(0);
    expect(after.usesOf("page", "p-offers").map((u) => u.fromId)).toContain("p-home");
  });
});

describe("[STU-076] rename warnings (D27)", () => {
  test("a route change lists the deep link and the generated route builder", () => {
    expect(routeChangeEffects("loan-offers", "offers")).toEqual([
      { kind: "deepLink", before: "/loan-offers", after: "/offers" },
      { kind: "generatedRoute", before: "loanOffers", after: "offers" },
    ]);
    expect(routeChangeEffects("same", "same")).toEqual([]);
    expect(componentKeyEffects("rate-card", "card", true)).toEqual([
      { kind: "generatedComponent", before: "rateCard", after: "card" },
    ]);
    expect(componentKeyEffects("rate-card", "card", false)).toEqual([]);
  });
});

describe("[STU-055] move a screen to another plugin (D23) and undo it as one step (F3)", () => {
  test("the screen leaves one plugin and joins the other with its links intact", () => {
    const all = docs();
    const moved = moveScreen(all, buildWhereUsed(all), "p-offers", "C");
    if (!moved.ok) {
      throw new Error(moved.reason);
    }
    const after = applyAll(all, moved.changeSet);
    const index = buildWhereUsed(after);
    expect(index.pages.get("p-offers")?.path).toBe("plugins/cards/pages/offers.page.json");
    expect(index.usesOf("page", "p-offers").map((u) => u.fromId)).toEqual(["p-home", "p-cards"]);
    const plugins = after.filter((d) => d.kind === "plugin").map((d) => (d.json as { pages: string[] }).pages);
    expect(plugins).toEqual([["p-home"], ["p-cards", "p-offers"]]);
  });

  test("entry screens, taken keys and the same plugin are refused", () => {
    const all = docs();
    const index = buildWhereUsed(all);
    expect(moveScreen(all, index, "p-home", "C")).toEqual({ ok: false, reason: "entry" });
    expect(moveScreen(all, index, "p-offers", "L")).toEqual({ ok: false, reason: "same-plugin" });
    expect(moveScreen(all, index, "p-none", "C")).toEqual({ ok: false, reason: "unknown" });
    const taken = [...all, { ...all[4], pluginId: "C", path: "plugins/cards/pages/offers.page.json" } as LoadedDoc];
    expect(moveScreen(taken, index, "p-offers", "C")).toEqual({ ok: false, reason: "key-taken" });
    const priv = all.map((d) =>
      d.kind === "component" ? { ...d, json: { kind: "component", id: "c-1", key: "card" } } : d,
    );
    expect(moveScreen(priv, buildWhereUsed(priv), "p-offers", "C")).toEqual({ ok: false, reason: "private-component" });
  });

  test("the server's inverses of one step become one change set across plugins", () => {
    const revisions = new Map([
      ["plugins/cards/pages/offers.page.json", 1],
      ["plugins/loans/plugin.json", 8],
    ]);
    const undo = undoStep(
      [
        {
          pluginId: "C",
          path: "plugins/cards/pages/offers.page.json",
          step: "s1",
          inverse: [{ op: "remove", path: "" }],
        },
        {
          pluginId: "L",
          path: "plugins/loans/pages/offers.page.json",
          step: "s1",
          inverse: [{ op: "add", path: "", value: { id: "p-offers" } }],
        },
        {
          pluginId: "L",
          path: "plugins/loans/plugin.json",
          step: "s1",
          inverse: [{ op: "replace", path: "/pages", value: ["p-home", "p-offers"] }],
        },
        {
          pluginId: "L",
          path: "plugins/loans/plugin.json",
          step: "s0",
          inverse: [{ op: "replace", path: "/name", value: "x" }],
        },
      ],
      "s1",
      revisions,
    );
    expect(undo).toEqual([
      { pluginId: "C", edits: [{ path: "plugins/cards/pages/offers.page.json", ifRevision: 1, delete: true }] },
      {
        pluginId: "L",
        edits: [
          { path: "plugins/loans/pages/offers.page.json", ifRevision: 0, content: { id: "p-offers" } },
          {
            path: "plugins/loans/plugin.json",
            ifRevision: 8,
            patch: [{ op: "replace", path: "/pages", value: ["p-home", "p-offers"] }],
          },
        ],
      },
    ]);
  });

  test("a change set is sent as one ApplyChangeSet with each draft's lock session", async () => {
    let sent: ApplyChangeSetRequest | undefined;
    const transport = createRouterTransport(({ service }) => {
      service(DocumentService, {
        applyChangeSet: (req) => {
          sent = req;
          return create(ApplyChangeSetResponseSchema, {});
        },
      });
    });
    const all = docs();
    const moved = moveScreen(all, buildWhereUsed(all), "p-offers", "C");
    if (!moved.ok) {
      throw new Error(moved.reason);
    }
    await applyChangeSet(
      transport,
      { appId: "app", editorSessionId: "tab", stepId: "s1" },
      moved.changeSet,
      async (id) => `lock-${id}`,
    );
    expect(sent?.stepId).toBe("s1");
    expect(sent?.drafts.map((d) => [d.pluginId, d.session, d.edits.length])).toEqual([
      ["L", "lock-L", 2],
      ["C", "lock-C", 2],
    ]);
    expect(sent?.drafts[0]?.edits[0]?.delete).toBe(true);
    expect(sent?.drafts[0]?.edits[0]?.ifRevision).toBe(5n);
    expect(new TextDecoder().decode(sent?.drafts[1]?.edits[0]?.content)).toContain('"p-offers"');
  });
});

describe("[STU-063] state entries in the where-used index", () => {
  const docs: LoadedDoc[] = [
    {
      pluginId: "p1",
      path: "plugins/p1/plugin.json",
      kind: "plugin",
      revision: 1,
      json: { id: "p1", state: [{ id: "s1", name: "count", type: "int", computed: "state.base + 1" }] },
    },
    {
      pluginId: "p1",
      path: "plugins/p1/pages/home.page.json",
      kind: "page",
      revision: 1,
      json: {
        id: "home",
        key: "home",
        root: { id: "n1", props: { text: { $expr: "state.count + state.countdown" } } },
      },
    },
    {
      pluginId: "p2",
      path: "plugins/p2/pages/x.page.json",
      kind: "page",
      revision: 1,
      json: { id: "x", key: "x", root: { id: "n2", props: { text: { $expr: "state.count" } } } },
    },
  ];

  test("a PXL read of state.<name> is a use, and only in the entry's own plugin", () => {
    const index = buildWhereUsed(docs);
    const check = checkStateDelete(index, "p1", "count");
    expect(check.incoming.map((u) => u.path)).toEqual(["plugins/p1/pages/home.page.json"]);
    expect(isBlocked(check)).toBe(true);
    expect(isBlocked(checkStateDelete(index, "p1", "unused"))).toBe(false);
  });

  test("a computed expression of another entry counts as a use", () => {
    const check = checkStateDelete(buildWhereUsed(docs), "p1", "base");
    expect(check.incoming.map((u) => u.pointer)).toEqual(["/state/0/computed"]);
  });
});
