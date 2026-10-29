// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem } from "@plux/validator";
import { createContext, useContext } from "react";
import type { PxlEditorSource } from "../panels/pxl-editor.tsx";
import type { ActionGraphMessages } from "./action-graph.messages.ts";
import type { CatalogueEntry, Recipe, Step, StepField } from "./graph-model.ts";

/** What the step nodes need from the overlay; nodes read it from here so React Flow's node objects stay plain data. */
export interface GraphContextValue {
  readonly steps: readonly Step[];
  readonly catalogue: readonly CatalogueEntry[];
  readonly canEdit: boolean;
  readonly m: ActionGraphMessages;
  /** Applies a recipe through the edit pipeline; `group` makes consecutive keystrokes one undo step. */
  readonly apply: (recipe: Recipe, group?: string) => void;
  /** Findings of the validator per step id. */
  readonly problems: ReadonlyMap<string, readonly Problem[]>;
  /** The PXL language service source for one input of a step; undefined when the service is not ready. */
  readonly pxlSource: (step: Step, field: StepField) => PxlEditorSource | undefined;
  /** Adds the catalogue entry with this key where the pointer was dropped; one undo step. */
  readonly dropEntry: (key: string, at: import("./graph-model.ts").Point) => void;
  readonly location: import("./graph-model.ts").GraphLocation;
}

export const GraphContext = createContext<GraphContextValue | null>(null);

export function useGraph(): GraphContextValue {
  const v = useContext(GraphContext);
  if (v === null) {
    throw new Error("useGraph outside the action graph editor");
  }
  return v;
}
