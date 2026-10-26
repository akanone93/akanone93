// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import dagre from "@dagrejs/dagre";
import type { GraphEdge, Step } from "./graph-model.ts";

/** Width of a step node in layout units; the node's class sets the same width. */
export const NODE_WIDTH = 288;
const BASE_HEIGHT = 168;
const ROW_HEIGHT = 56;

/** The height reserved for a step that shows `rows` input and port rows, until React Flow has measured the real one. */
export function estimatedHeight(rows: number): number {
  return BASE_HEIGHT + ROW_HEIGHT * rows;
}

export interface Placed {
  readonly x: number;
  readonly y: number;
}

/**
 * Left-to-right layout of a graph with dagre (A4.5): pure and deterministic, so the same graph always
 * lays out the same. A step's height is estimated from the rows it shows; React Flow measures the real one.
 */
export function layoutSteps(
  steps: readonly Step[],
  edges: readonly GraphEdge[],
  rows: (step: Step) => number,
): ReadonlyMap<string, Placed> {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "LR", nodesep: 32, ranksep: 96, marginx: 24, marginy: 24 });
  g.setDefaultEdgeLabel(() => ({}));
  for (const s of steps) {
    g.setNode(s.id, { width: NODE_WIDTH, height: estimatedHeight(rows(s)) });
  }
  for (const e of edges) {
    g.setEdge(e.from, e.to);
  }
  dagre.layout(g);
  const out = new Map<string, Placed>();
  for (const s of steps) {
    const n = g.node(s.id);
    out.set(s.id, { x: n.x - n.width / 2, y: n.y - n.height / 2 });
  }
  return out;
}
