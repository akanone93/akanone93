// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Style copy and paste (PC-21, STU-062): a widget's style is its appearance props — colours, decoration, borders,
 * radius, elevation, text style, shape — copied from one widget and pasted onto others; each target takes the props
 * its own descriptor declares, so a button's style pastes onto another button, and a card's colour onto a container.
 */

import { widget } from "@plux/schema";
import type { Command } from "../../lib/commands/index.ts";
import { isStyleProp } from "../panels/prop-groups.ts";
import { type EditFn, findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** A copied style: the type it came from and its appearance props. */
export interface CopiedStyle {
  readonly from: string;
  readonly props: Readonly<Obj>;
}

/** The style of `node`: its appearance props, deep-copied; undefined when it has none. */
export function copyStyle(
  node: { readonly type?: string; readonly props?: unknown } | undefined,
): CopiedStyle | undefined {
  if (node?.type === undefined || !isObj(node.props)) {
    return undefined;
  }
  const props = Object.fromEntries(
    Object.entries(node.props)
      .filter(([k]) => isStyleProp(k))
      .map(([k, v]) => [k, JSON.parse(JSON.stringify(v)) as unknown]),
  );
  return Object.keys(props).length === 0 ? undefined : { from: node.type, props };
}

const propTypes = (type: string | undefined): Map<string, string> =>
  new Map((widget(type ?? "")?.props ?? []).map((p) => [p.name, p.type.replace(/\?$/, "")]));

/**
 * The props of `style` a widget of `type` takes: those its descriptor declares with the type they have where they were
 * copied — a button's `style` (a `ButtonStyle`) never lands on a text's `style` (a `TextStyle`).
 */
export function styleFor(type: string | undefined, style: CopiedStyle): Obj {
  const target = propTypes(type);
  const source = propTypes(style.from);
  return Object.fromEntries(
    Object.entries(style.props).filter(([k]) => target.has(k) && (!source.has(k) || source.get(k) === target.get(k))),
  );
}

/** The recipe that pastes `style` onto node `id`: its appearance props replaced by those of the style it takes. */
export function pasteStyleRecipe(id: string, style: CopiedStyle): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, id);
    if (at === null) {
      return;
    }
    const node = at.node as Obj;
    const type = typeof node["type"] === "string" ? node["type"] : undefined;
    const taken = styleFor(type, style);
    if (Object.keys(taken).length === 0) {
      return;
    }
    const props = isObj(node["props"]) ? (node["props"] as Obj) : {};
    const kept = Object.fromEntries(Object.entries(props).filter(([k]) => !isStyleProp(k)));
    node["props"] = { ...kept, ...taken };
  };
}

/** What the style commands work on: the selection, its nodes and documents, the held style and where to note. */
export interface StyleCommandInput {
  readonly selected: () => readonly string[];
  readonly nodeOf: (id: string) => { readonly type?: string; readonly props?: unknown } | undefined;
  readonly docOf: (id: string) => string | undefined;
  readonly edit: EditFn;
  readonly held: () => CopiedStyle | undefined;
  readonly hold: (style: CopiedStyle) => void;
  readonly note: (text: string) => void;
  readonly text: {
    readonly copy: string;
    readonly paste: string;
    readonly copied: (type: string) => string;
    readonly pasted: (n: number) => string;
    readonly none: string;
  };
}

/**
 * Copy style and Paste style (PC-21), on Ctrl/Cmd+Alt+C and V as in design tools: the first selected widget's style is
 * held; pasting puts it on every selected widget that takes some of it, all in one undo step.
 */
export function styleCommands(input: StyleCommandInput): readonly Command[] {
  const copy = () => {
    const [id] = input.selected();
    const style = id === undefined ? undefined : copyStyle(input.nodeOf(id));
    if (style !== undefined) {
      input.hold(style);
      input.note(input.text.copied(style.from));
    }
  };
  const paste = () => {
    const style = input.held();
    if (style === undefined) {
      return;
    }
    const group = `paste-style:${Date.now()}`;
    const taking = input.selected().filter((id) => Object.keys(styleFor(input.nodeOf(id)?.type, style)).length > 0);
    for (const id of taking) {
      const doc = input.docOf(id);
      if (doc !== undefined) {
        input.edit(doc, pasteStyleRecipe(id, style), { group });
      }
    }
    input.note(taking.length === 0 ? input.text.none : input.text.pasted(taking.length));
  };
  return [
    {
      id: "canvas.copyStyle",
      title: input.text.copy,
      group: "edit",
      shortcut: "mod+alt+c",
      permission: null,
      run: copy,
    },
    {
      id: "canvas.pasteStyle",
      title: input.text.paste,
      group: "edit",
      shortcut: "mod+alt+v",
      permission: null,
      run: paste,
    },
  ];
}
