// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  buildEnvelope,
  type ClipboardSource,
  type Envelope,
  freshNodes,
} from "../../plugin-canvas/clipboard/envelope.ts";
import { uuidv7 } from "../../plugin-canvas/screens.ts";

export interface TemplateParameter {
  readonly name: string;
  readonly type: string;
  /** JSON Pointer from the template's root to the value the parameter fills. */
  readonly path: string;
}

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/** The root a template saves: the single selected node, or a column holding the selection. */
export function rootOf(nodes: readonly unknown[]): unknown {
  return nodes.length === 1 ? nodes[0] : { id: uuidv7(), type: "Column", children: [...nodes] };
}

function textProps(node: Record<string, unknown>, path: string): TemplateParameter[] {
  const type = typeof node["type"] === "string" ? node["type"] : "Node";
  const props = isRecord(node["props"]) ? node["props"] : {};
  return Object.entries(props)
    .filter(([, v]) => typeof v === "string")
    .map(([k]) => ({ name: `${type}.${k}`, type: "string", path: `${path}/props/${k}` }));
}

/** Text props of the selection that may be exposed as parameters (D28 step 2), by pointer into the root. */
export function candidateParameters(root: unknown, limit = 20): TemplateParameter[] {
  const out: TemplateParameter[] = [];
  const queue: [unknown, string][] = [[root, ""]];
  for (let next = queue.shift(); next !== undefined && out.length < limit; next = queue.shift()) {
    const [node, path] = next;
    if (isRecord(node)) {
      out.push(...textProps(node, path));
      const children = Array.isArray(node["children"]) ? node["children"] : [];
      queue.push(...children.map((c, i): [unknown, string] => [c, `${path}/children/${i}`]));
    }
  }
  return out.slice(0, limit);
}

export interface TemplateDetails {
  readonly key: string;
  readonly name: string;
  readonly description: string;
  readonly category: string;
  readonly visibility: string;
}

/** The template document D28 saves; dependencies are captured as IDs, never content (STU-080). */
export function buildTemplateDoc(
  details: TemplateDetails,
  parameters: readonly TemplateParameter[],
  nodes: readonly unknown[],
  source: ClipboardSource,
): Record<string, unknown> {
  const dependencies = buildEnvelope(source, nodes).dependencies;
  return {
    schemaVersion: "1.0.0",
    kind: "template",
    id: uuidv7(),
    key: details.key,
    name: details.name,
    description: details.description,
    category: details.category,
    visibility: details.visibility,
    parameters: parameters.map((p) => ({ name: p.name, type: p.type, path: p.path })),
    dependencies,
    root: rootOf(buildEnvelope(source, nodes).nodes),
  };
}

function setAt(root: unknown, path: string, value: unknown): void {
  const parts = path.split("/").slice(1);
  let at: unknown = root;
  for (const part of parts.slice(0, -1)) {
    at = isRecord(at) || Array.isArray(at) ? (at as Record<string, unknown>)[part] : undefined;
  }
  const last = parts.at(-1);
  if ((isRecord(at) || Array.isArray(at)) && last !== undefined) {
    (at as Record<string, unknown>)[last] = value;
  }
}

/** What inserting yields: the template's root with the parameters filled and every ID new, copied not linked (STU-083). */
export function instantiateRoot(doc: Record<string, unknown>, args: Readonly<Record<string, string>>): unknown[] {
  const root = structuredClone(doc["root"]);
  const parameters = Array.isArray(doc["parameters"]) ? (doc["parameters"] as TemplateParameter[]) : [];
  for (const p of parameters) {
    const value = args[p.name];
    if (value !== undefined) {
      setAt(root, p.path, value);
    }
  }
  const envelope = { nodes: [root] } as unknown as Envelope;
  return [...freshNodes(envelope)];
}

/** The envelope the dependency report of a template insert runs on: foreign to every plugin, so all missing show. */
export function templateEnvelope(doc: Record<string, unknown>, nodes: readonly unknown[]): Envelope {
  return buildEnvelope({ app: `template:${String(doc["id"])}`, plugin: "template" }, nodes);
}

/** A component instance node standing in for extracted nodes (D30); `id` is the component's document ID. */
export function instanceNode(componentDocId: string, version: number): Record<string, unknown> {
  return { id: uuidv7(), component: { id: componentDocId, version }, props: {} };
}

/** The component document D30 saves from the selection. */
export function buildComponentDoc(
  details: { readonly key: string; readonly name: string; readonly description: string },
  nodes: readonly unknown[],
  source: ClipboardSource,
): Record<string, unknown> {
  return {
    schemaVersion: "1.0.0",
    kind: "component",
    id: uuidv7(),
    key: details.key,
    name: details.name,
    description: details.description,
    version: 1,
    props: [],
    slots: [],
    root: rootOf(buildEnvelope(source, nodes).nodes),
  };
}

/** D33: the plain nodes an instance becomes: the component's root with the instance's prop overrides and new IDs. */
export function detachedNodes(instance: unknown, componentRoot: unknown): unknown[] {
  const root = structuredClone(componentRoot);
  const overrides = isRecord(instance) && isRecord(instance["props"]) ? instance["props"] : {};
  if (isRecord(root)) {
    root["props"] = { ...(isRecord(root["props"]) ? root["props"] : {}), ...overrides };
  }
  return [...freshNodes({ nodes: [root] } as unknown as Envelope)];
}
