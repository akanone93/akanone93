// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  applyNodeChanges,
  type Connection,
  Controls,
  type Edge,
  type Node,
  type NodeChange,
  ReactFlow,
  ReactFlowProvider,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import "./graph-theme.css";
import { type ReactElement, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useGraph } from "./graph-context.ts";
import { estimatedHeight, layoutSteps, NODE_WIDTH, type Placed } from "./graph-layout.ts";
import type { Step } from "./graph-model.ts";
import { canConnect, connectRecipe, edgesOf, entryOf, portsOf, removeStepRecipe } from "./graph-model.ts";
import { StepNode } from "./step-node.tsx";

const nodeTypes = { step: StepNode };

/** How many input rows a step shows, which sets the height dagre reserves for it. */
function rowsOf(inputs: number, ports: number): number {
  return inputs + ports;
}

/**
 * The drawing of the graph: React Flow over the steps, laid out by dagre (A4.5). It owns only what the
 * document does not: where each node was dragged. Every change to the graph goes through `apply`, so
 * it is an edit-pipeline JSON Patch and one undo step.
 */
function Canvas({ layoutToken }: { layoutToken: number }): ReactElement {
  const g = useGraph();
  const [nodes, setNodes] = useState<Node[]>([]);
  const handled = useRef(layoutToken);
  const edges = useMemo(() => edgesOf(g.steps, g.catalogue), [g.steps, g.catalogue]);
  const flowEdges = useMemo<Edge[]>(
    () => edges.map((e) => ({ id: e.id, source: e.from, sourceHandle: e.port, target: e.to, targetHandle: "in" })),
    [edges],
  );

  // Steps that are new get a place from dagre; steps that were already there stay where the user put them.
  // biome-ignore lint/correctness/useExhaustiveDependencies: the layout runs again for a new step set or a relayout request
  useEffect(() => {
    const rows = (s: Step) => {
      const entry = entryOf(s, g.catalogue);
      return rowsOf(entry?.fields.length ?? 0, portsOf(entry, s).length);
    };
    const placed: ReadonlyMap<string, Placed> = layoutSteps(g.steps, edges, rows);
    const fresh = handled.current !== layoutToken;
    handled.current = layoutToken;
    setNodes((prev) =>
      g.steps.map((s) => {
        const old = fresh ? undefined : prev.find((n) => n.id === s.id);
        return {
          ...old,
          id: s.id,
          type: "step",
          position: old?.position ?? placed.get(s.id) ?? { x: 0, y: 0 },
          data: {},
          initialWidth: NODE_WIDTH,
          initialHeight: estimatedHeight(rows(s)),
        };
      }),
    );
  }, [g.steps.map((s) => s.id).join("|"), edges.map((e) => e.id + e.to).join("|"), layoutToken]);

  const onNodesChange = useCallback((changes: NodeChange[]) => setNodes((ns) => applyNodeChanges(changes, ns)), []);
  const onConnect = useCallback(
    (c: Connection) => {
      const from = g.steps.find((s) => s.id === c.source);
      const port =
        from === undefined
          ? undefined
          : portsOf(entryOf(from, g.catalogue), from).find((p) => p.name === c.sourceHandle);
      if (from !== undefined && port !== undefined && c.target !== null) {
        g.apply(connectRecipe(g.location, from.id, port, c.target));
      }
    },
    [g],
  );
  const isValidConnection = useCallback((c: Connection | Edge) => canConnect(g.steps, c.source, c.target), [g.steps]);
  const onEdgesDelete = useCallback(
    (gone: Edge[]) => {
      for (const e of gone) {
        const from = g.steps.find((s) => s.id === e.source);
        const port =
          from === undefined
            ? undefined
            : portsOf(entryOf(from, g.catalogue), from).find((p) => p.name === e.sourceHandle);
        if (from !== undefined && port !== undefined) {
          g.apply(connectRecipe(g.location, from.id, port, undefined));
        }
      }
    },
    [g],
  );
  const onNodesDelete = useCallback(
    (gone: Node[]) => {
      for (const n of gone) {
        g.apply(removeStepRecipe(g.location, n.id));
      }
    },
    [g],
  );

  const aria = useMemo(
    () => ({
      "node.a11yDescription.default": g.canEdit ? g.m.nodeHint : g.m.nodeHintFixed,
      "node.a11yDescription.keyboardDisabled": g.canEdit ? g.m.nodeHint : g.m.nodeHintFixed,
      "node.a11yDescription.ariaLiveMessage": (p: { direction: string; x: number; y: number }) =>
        g.m.moved(p.direction, String(p.x), String(p.y)),
      "edge.a11yDescription.default": g.m.edgeHint,
      "controls.ariaLabel": g.m.controls,
      "controls.zoomIn.ariaLabel": g.m.zoomIn,
      "controls.zoomOut.ariaLabel": g.m.zoomOut,
      "controls.fitView.ariaLabel": g.m.fitView,
      "controls.interactive.ariaLabel": g.m.lockView,
      "minimap.ariaLabel": g.m.miniMap,
      "handle.ariaLabel": g.m.handle,
    }),
    [g.m, g.canEdit],
  );

  return (
    <div className="flex h-full min-h-0 flex-col gap-2">
      <ReactFlow
        aria-label={g.m.graphLabel}
        nodes={nodes}
        edges={flowEdges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onConnect={onConnect}
        onEdgesDelete={onEdgesDelete}
        onNodesDelete={onNodesDelete}
        isValidConnection={isValidConnection}
        nodesConnectable={g.canEdit}
        deleteKeyCode={g.canEdit ? ["Backspace", "Delete"] : null}
        nodesFocusable
        edgesFocusable
        fitView
        ariaLabelConfig={aria}
        proOptions={{ hideAttribution: false }}
      >
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

export interface GraphCanvasProps {
  /** Changes when the user asks for the steps to be arranged again. */
  readonly layoutToken: number;
}

export default function GraphCanvas({ layoutToken }: GraphCanvasProps): ReactElement {
  return (
    <ReactFlowProvider>
      <Canvas layoutToken={layoutToken} />
    </ReactFlowProvider>
  );
}
