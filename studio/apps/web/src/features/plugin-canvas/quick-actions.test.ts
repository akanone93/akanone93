// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  addStepRecipe,
  eventMarks,
  handlersOf,
  quickStep,
  ready,
  removeStepRecipe,
  snackbarOf,
  withSnackbar,
} from "./quick-actions.ts";
import { nodeIdOfHit } from "./select/tree.ts";

type Node = Record<string, unknown>;
const button = (): Node => ({
  id: "b",
  type: "FilledButton",
  slots: { child: { id: "t", type: "Text", props: { data: "Pay" } } },
});

describe("PC-27 events and quick actions [ACT-001] [ACT-002]", () => {
  test("a quick action writes its step, after the event's steps or in their place; removing one keeps the chain", () => {
    const root = { id: "col", type: "Column", children: [button()] };
    expect(ready("message", {})).toBe(false);
    addStepRecipe(
      "b",
      "onPressed",
      quickStep("message", { message: "Paid", actionLabel: "Undo" }, "s1"),
      "replace",
    )(root);
    addStepRecipe("b", "onPressed", quickStep("back", {}, "s2"), "append")(root);
    addStepRecipe("b", "onLongPress", quickStep("link", { url: "https://plux.dev" }, "s3"), "replace")(root);
    const b = root.children[0] as Node;
    expect(b["events"]).toEqual({
      onPressed: {
        steps: [
          { id: "s1", action: "showSnackbar", input: { message: "Paid", actionLabel: "Undo" }, next: "s2" },
          { id: "s2", action: "pop", input: {} },
        ],
      },
      onLongPress: { steps: [{ id: "s3", action: "openUrl", input: { url: "https://plux.dev" } }] },
    });
    expect(handlersOf(b).map((s) => `${s.event}:${s.action}:${s.detail ?? ""}`)).toEqual([
      "onPressed:showSnackbar:Paid",
      "onPressed:pop:",
      "onLongPress:openUrl:https://plux.dev",
    ]);
    expect(eventMarks([root]).get("b")).toHaveLength(3);
    removeStepRecipe("b", "onPressed", 1)(root);
    expect((b["events"] as Node)["onPressed"]).toEqual({
      steps: [{ id: "s1", action: "showSnackbar", input: { message: "Paid", actionLabel: "Undo" } }],
    });
    removeStepRecipe("b", "onLongPress", 0)(root);
    removeStepRecipe("b", "onPressed", 0)(root);
    expect("events" in b).toBe(false);
  });

  test("a selected widget that shows a message draws its snackbar in the scaffold, clickable as the widget", () => {
    const b = button();
    addStepRecipe(
      "b",
      "onPressed",
      quickStep("message", { message: "Saved" }, "s"),
      "replace",
    )({ id: "r", children: [b] });
    expect(snackbarOf(b)).toEqual({ message: "Saved" });
    const page = { id: "p", type: "Scaffold", slots: { body: b } };
    const drawn = withSnackbar(page, "b") as { slots: Record<string, Node> };
    expect(drawn.slots["snackBar"]?.["id"]).toBe("b~snack");
    expect(JSON.stringify(drawn.slots["snackBar"])).toContain("color.inverseSurface");
    expect(nodeIdOfHit("p/b~snack-message")).toBe("b");
    expect(withSnackbar(page, undefined)).toBe(page);
    expect(quickStep("submit", { form: "signup" }, "x")).toEqual({
      id: "x",
      action: "submitForm",
      input: { form: "signup" },
    });
  });

  test("a step id is a name: a UUID gives way to the action's name, numbered when it is taken", () => {
    const root = { id: "r", children: [button()] };
    const uuid = "01c0c450-6c00-7000-8b00-000000000009";
    addStepRecipe("b", "onPressed", quickStep("message", { message: "A" }, uuid), "replace")(root);
    addStepRecipe("b", "onPressed", quickStep("message", { message: "B" }, uuid), "append")(root);
    const steps = ((root.children[0] as Node)["events"] as { onPressed: { steps: Node[] } }).onPressed.steps;
    expect(steps.map((s) => s["id"])).toEqual(["showSnackbar", "showSnackbar2"]);
    expect(steps[0]?.["next"]).toBe("showSnackbar2");
  });
});
