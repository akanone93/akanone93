// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `Variant` nodes on the canvas (ABT-001, STU-103, build log row 304). A Variant holds its experiment's arms outside
 * `children` and `slots`: the `control` subtree and one subtree per variant under `variants`. The canvas lists every
 * arm in Layers, edits each arm in place, draws the arm picked in the inspector and wraps a selection in a new Variant.
 */

import { type PageDoc, remapIds } from "./screens.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The reserved node type of a variant subtree. */
export const VARIANT_TYPE = "Variant";

/** One arm of a Variant: its variant name (null for the control), its JSON Pointer below the Variant, its subtree. */
export interface VariantArm {
  readonly name: string | null;
  readonly pointer: string;
  readonly node: Obj;
}

/** The pointer segment of a variant name (RFC 6901 escapes `~` and `/`). */
const pointerSegment = (name: string) => name.replaceAll("~", "~0").replaceAll("/", "~1");

/** The arms of `node` when it is a Variant: the control first, then the variants by name, as the compiler orders them. */
export function variantArms(node: unknown): readonly VariantArm[] {
  if (!isObj(node) || node["type"] !== VARIANT_TYPE) {
    return [];
  }
  const out: VariantArm[] = [];
  if (isObj(node["control"])) {
    out.push({ name: null, pointer: "/control", node: node["control"] });
  }
  const variants = isObj(node["variants"]) ? node["variants"] : {};
  for (const name of Object.keys(variants).sort()) {
    const arm = variants[name];
    if (isObj(arm)) {
      out.push({ name, pointer: `/variants/${pointerSegment(name)}`, node: arm });
    }
  }
  return out;
}

/** The label an arm goes by in Layers and the inspector: its variant name, or `control`. */
export const armLabel = (arm: VariantArm) => arm.name ?? "control";

/**
 * A copy of `node` with a fresh ID for every identifier defined in it and its references to them following, so a
 * variant arm made from the control shares no ID with it (node IDs are unique in a page, SCH-005). References to what
 * is defined elsewhere — a component, a translation — stay, as Duplicate and Paste keep them.
 */
const freshCopy = (node: unknown, fresh: () => string): unknown =>
  (remapIds({ root: node } as unknown as PageDoc, fresh) as unknown as { root: unknown }).root;

/**
 * The Variant that wraps `node` for experiment `experiment`: `node` is the control, unchanged, and each variant starts
 * as a copy of it with fresh IDs, to be changed on the canvas.
 */
export function variantOf(node: unknown, experiment: string, variants: readonly string[], fresh: () => string): Obj {
  const arms: Obj = {};
  for (const name of variants) {
    arms[name] = freshCopy(node, fresh);
  }
  return { id: fresh(), type: VARIANT_TYPE, experiment, control: node, variants: arms };
}
