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
