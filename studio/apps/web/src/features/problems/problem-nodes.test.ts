// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { nodeOfPointer, problemsByNode } from "./problem-nodes.ts";

const doc = {
  kind: "page",
  root: {
    id: "sc",
    type: "Scaffold",
    slots: {
      body: {
        id: "col",
        type: "Column",
        children: [
          { id: "t1", type: "Text", props: { data: { $expr: "page.x" } } },
          { id: "a/b", type: "Text" },
        ],
      },
    },
  },
};

describe("PC-16 problems pinned to widgets [STU-067]", () => {
  test("a pointer is owned by the deepest widget it passes through", () => {
    expect(nodeOfPointer(doc, "/root/slots/body/children/0/props/data")).toBe("t1");
    expect(nodeOfPointer(doc, "/root/slots/body")).toBe("col");
    expect(nodeOfPointer(doc, "/root/slots/body/children/9")).toBe("col");
    expect(nodeOfPointer(doc, "/params/0")).toBeUndefined();
  });

  test("each widget's problems, the most severe first", () => {
    const byNode = problemsByNode(
      [
        { code: "PLX-1", severity: "warning", file: "p.json", path: "/root/slots/body/children/0", message: "w" },
        {
          code: "PLX-2",
          severity: "error",
          file: "p.json",
          path: "/root/slots/body/children/0/props/data",
          message: "e",
        },
        { code: "PLX-3", severity: "error", file: "other.json", path: "/root", message: "x" },
      ],
      new Map([["p.json", doc]]),
    );
    expect(byNode.get("t1")?.map((p) => p.code)).toEqual(["PLX-2", "PLX-1"]);
    expect(byNode.size).toBe(1);
  });
});

describe("PC-34 animation problems pinned to the widgets they animate [ANI-008] [SCH-040]", () => {
  test("a timeline's track and driver pin to the widget they name; an action's input does not", () => {
    const page = {
      ...doc,
      animations: [
        {
          id: "tl",
          tracks: [{ node: "t1", prop: "opacity", keyframes: [{ at: 0 }, { at: 1 }] }],
          driver: { kind: "scroll", node: "col" },
        },
      ],
    };
    expect(nodeOfPointer(page, "/animations/0/tracks/0/keyframes/1")).toBe("t1");
    expect(nodeOfPointer(page, "/animations/0/driver")).toBe("col");
    expect(nodeOfPointer(page, "/animations/0/id")).toBeUndefined();
    const acting = {
      root: {
        id: "b",
        type: "FilledButton",
        events: { onPressed: { steps: [{ id: "s", action: "focus", input: { node: "t1" } }] } },
      },
    };
    expect(nodeOfPointer(acting, "/root/events/onPressed/steps/0/input")).toBe("b");
  });
});
