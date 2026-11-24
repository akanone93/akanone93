// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { Problem } from "@plux/validator";
import { type ReactElement, useCallback, useMemo, useState } from "react";
import { type Command, useScopedCommands } from "../../../lib/commands/index.ts";
import { uuidv7 } from "../screens.ts";
import type { EditFn } from "../select/edits.ts";
import { findNode } from "../select/edits.ts";
import { type Board, readEnvelopeText, writeEnvelope } from "./board.ts";
import { clipboardMessages } from "./clipboard.messages.ts";
import { type CopyOverIO, copyItems, copyOver, type Dropped, resolveCopies } from "./copy-over.ts";
import {
  applyResolutions,
  buildEnvelope,
  type ClipboardSource,
  type Envelope,
  freshNodes,
  parseClipboard,
  planReport,
  type RefusalReason,
  type ReportItem,
  type Resolution,
  type TargetHas,
} from "./envelope.ts";
import { ClipboardReportDialog } from "./report-dialog.tsx";

type Node = Record<string, unknown>;
const isNode = (v: unknown): v is Node => typeof v === "object" && v !== null && !Array.isArray(v);

/** The lists below `node` that hold widgets — its `children`, then each list slot — in document order, depth first. */
function firstList(node: unknown): unknown[] | undefined {
  if (!isNode(node)) {
    return undefined;
  }
  if (Array.isArray(node["children"])) {
    return node["children"];
  }
  const slots = isNode(node["slots"]) ? Object.values(node["slots"]) : [];
  for (const slot of slots) {
    const found = Array.isArray(slot) ? slot : firstList(slot);
    if (found !== undefined) {
      return found;
    }
  }
  return undefined;
}

/** The chain from `node` down to the node `id`: each step's list and index, or null for a single-child slot. */
function chainTo(node: unknown, id: string): { list: unknown[] | null; index: number }[] | undefined {
  if (!isNode(node)) {
    return undefined;
  }
  if (node["id"] === id) {
    return [];
  }
  const below = (child: unknown, list: unknown[] | null, index: number) => {
    const rest = chainTo(child, id);
    return rest === undefined ? undefined : [{ list, index }, ...rest];
  };
  const kids = Array.isArray(node["children"]) ? node["children"] : [];
  for (const [i, child] of kids.entries()) {
    const found = below(child, kids, i);
    if (found !== undefined) {
      return found;
    }
  }
  for (const slot of isNode(node["slots"]) ? Object.values(node["slots"]) : []) {
    const found = Array.isArray(slot)
      ? slot.map((child, i) => below(child, slot, i)).find((f) => f !== undefined)
      : below(slot, null, -1);
    if (found !== undefined) {
      return found;
    }
  }
  return undefined;
}

/**
 * Where pasted nodes go in a screen (#75): right after the anchor when it sits in a list, else after its nearest
 * ancestor that does; with no anchor, or none in a list, at the end of the screen's first list. Undefined when the
 * screen has no list at all: a widget is never put where its parent takes none.
 */
export function pastePlace(root: unknown, anchor: string | undefined): { list: unknown[]; index: number } | undefined {
  const chain = anchor === undefined ? undefined : chainTo(root, anchor);
  const inList = chain?.filter((step) => step.list !== null).at(-1);
  if (inList?.list != null) {
    return { list: inList.list, index: inList.index + 1 };
  }
  const list = firstList(root);
  return list === undefined ? undefined : { list, index: list.length };
}

/** Pastes `nodes` into the screen whose root the draft is, where `pastePlace` puts them; nothing where it puts none. */
export function pasteRecipe(anchor: string | undefined, nodes: readonly unknown[]): (draft: unknown) => void {
  return (draft) => {
    const place = pastePlace(draft, anchor);
    place?.list.splice(place.index, 0, ...nodes);
  };
}

/** Removes the node `id` from its children list or slot. */
export function removeRecipe(id: string): (draft: unknown) => void {
  return (draft) => {
    const place = findNode(draft, id);
    if (place === null) {
      return;
    }
    if (place.list !== null) {
      place.list.splice(place.index, 1);
    } else if (place.slot !== null && place.parent !== null) {
      delete (place.parent.slots as Record<string, unknown>)[place.slot];
    }
  };
}

/** What the target documents define, as IDs and keys; the report lists references outside this set. */
export function targetHas(documents: readonly { readonly content: Uint8Array }[]): TargetHas {
  const ids = new Set<string>();
  for (const d of documents) {
    try {
      const doc = JSON.parse(new TextDecoder().decode(d.content)) as Record<string, unknown>;
      for (const k of ["id", "key"]) {
        if (typeof doc[k] === "string") {
          ids.add(doc[k]);
        }
      }
      const messages = doc["messages"];
      if (typeof messages === "object" && messages !== null) {
        for (const key of Object.keys(messages)) {
          ids.add(key);
        }
      }
    } catch {
      // Not JSON: defines nothing.
    }
  }
  return { functions: ids, translations: ids, assets: ids, dataSources: ids };
}

interface Pending {
  readonly envelope: Envelope;
  readonly items: readonly ReportItem[];
}

const pageAround = (nodes: readonly unknown[]) =>
  JSON.stringify({
    schemaVersion: "1.0.0",
    kind: "page",
    id: uuidv7(),
    key: "clipboard",
    route: "clipboard",
    pageKind: "screen",
    title: "Clipboard",
    root: { id: uuidv7(), type: "Column", children: nodes },
  });

export interface ClipboardInput {
  readonly screens: readonly { readonly id: string; readonly root: unknown }[];
  readonly screenOf: ReadonlyMap<string, string>;
  readonly selected: () => readonly string[];
  readonly edit: EditFn;
  readonly canEdit: boolean;
  readonly source: ClipboardSource;
  readonly has: TargetHas;
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
  /** The page state a screen declares, for "save as template" to turn references to it into parameters (STU-082). */
  readonly pageState?: (screenId: string) => readonly { name: string; type: string; default?: unknown }[];
  readonly clipboard?: Board;
  /** Where "copy over" reads its sources and writes its copies; without it a copy keeps the reference only. */
  readonly copyOver?:
    | {
        readonly io: (source: ClipboardSource) => CopyOverIO;
        readonly prefix: string;
        /** How many documents the change set may still take. */
        readonly room: () => number;
      }
    | undefined;
}

/** Registers edit.copy, edit.cut and edit.paste on the plugin canvas; returns the dialog to render. */
export function useClipboard(input: ClipboardInput): ReactElement | null {
  const { i18n } = useLingui();
  const m = clipboardMessages(i18n);
  const [pending, setPending] = useState<Pending | undefined>();
  const [refusal, setRefusal] = useState<RefusalReason | undefined>();
  const [dropped, setDropped] = useState<readonly Dropped[] | undefined>();
  const { screens, screenOf, selected, edit, source, has, validatePage } = input;
  const board = input.clipboard ?? navigator.clipboard;

  const selectedNodes = useCallback(() => {
    const ids = selected();
    return ids.flatMap((id) => {
      const root = screens.find((s) => s.id === screenOf.get(id))?.root;
      const place = root === undefined ? null : findNode(root, id);
      return place === null ? [] : [place.node];
    });
  }, [selected, screens, screenOf]);

  const copy = useCallback(async (): Promise<boolean> => {
    const nodes = selectedNodes();
    if (nodes.length === 0) {
      return false;
    }
    await writeEnvelope(board, JSON.stringify(buildEnvelope(source, nodes)));
    return true;
  }, [selectedNodes, board, source]);

  const cut = useCallback(async () => {
    const ids = selected();
    if (!(await copy())) {
      return;
    }
    const group = `cut-${uuidv7()}`;
    for (const id of ids) {
      const doc = screenOf.get(id);
      if (doc !== undefined) {
        edit(doc, removeRecipe(id), { group });
      }
    }
  }, [copy, selected, screenOf, edit]);

  const apply = useCallback(
    async (envelope: Envelope, items: readonly ReportItem[], choices: ReadonlyMap<string, Resolution> = new Map()) => {
      const anchor = selected()[0];
      const doc = (anchor === undefined ? undefined : screenOf.get(anchor)) ?? screens[0]?.id;
      if (doc === undefined) {
        return;
      }
      if (pastePlace(screens.find((s) => s.id === doc)?.root, anchor) === undefined) {
        // The screen holds no list a widget can join; nothing is copied over for a paste that cannot land.
        setRefusal("no-place");
        return;
      }
      let resolved = choices;
      const wanted = copyItems(items, choices);
      if (input.copyOver !== undefined && wanted.length > 0) {
        // The sources are created first; the paste that refers to them is the one undo step (F7).
        const { io, prefix, room } = input.copyOver;
        const result = await copyOver(wanted, io(envelope.source), { prefix, room: room() });
        resolved = resolveCopies(choices, items, result);
        if (result.dropped.length > 0) {
          setDropped(result.dropped);
        }
      }
      edit(doc, pasteRecipe(anchor, applyResolutions(freshNodes(envelope), resolved)), { step: "paste" });
    },
    [selected, screenOf, screens, edit, input.copyOver],
  );

  const validate = useCallback(
    async (nodes: readonly unknown[]): Promise<readonly string[]> => {
      if (validatePage === undefined) {
        return ["validator unavailable"];
      }
      const found = await validatePage("clipboard.json", pageAround(nodes));
      return found.filter((p) => p.severity === "error" && p.path.startsWith("/root/children")).map((p) => p.message);
    },
    [validatePage],
  );

  const paste = useCallback(async () => {
    const result = await parseClipboard(await readEnvelopeText(board), { validate });
    if (result.kind === "refused") {
      setRefusal(result.reason);
    } else if (result.kind === "text") {
      // A paste that does nothing says why (#75).
      setRefusal("no-widgets");
    } else if (result.kind === "nodes") {
      // Widget JSON from outside an envelope: pasted as if copied here, its references checked by the validator.
      await apply(buildEnvelope(source, result.nodes), []);
    } else if (result.kind === "envelope") {
      const items = planReport(result.envelope, source, has);
      if (items.length > 0) {
        setPending({ envelope: result.envelope, items });
      } else {
        await apply(result.envelope, items);
      }
    }
  }, [board, validate, source, has, apply]);

  const commands = useMemo<readonly Command[]>(() => {
    const reason = input.canEdit ? undefined : "read-only";
    const make = (id: string, title: string, shortcut: string, run: () => Promise<void>, edits: boolean): Command => ({
      id,
      title,
      group: "edit",
      shortcut,
      permission: null,
      ...(edits && reason !== undefined ? { disabledReason: reason } : {}),
      run,
    });
    return [
      make("edit.copy", m.copy, "mod+c", async () => void (await copy()), false),
      make("edit.cut", m.cut, "mod+x", cut, true),
      make("edit.paste", m.paste, "mod+v", paste, true),
    ];
  }, [input.canEdit, m.copy, m.cut, m.paste, copy, cut, paste]);
  useScopedCommands("plugin-clipboard", commands);

  if (pending === undefined && refusal === undefined && dropped === undefined) {
    return null;
  }
  const close = () => {
    setPending(undefined);
    setRefusal(undefined);
    setDropped(undefined);
  };
  return (
    <ClipboardReportDialog
      items={pending?.items}
      candidates={has}
      refusal={refusal}
      {...(dropped === undefined ? {} : { dropped })}
      onConfirm={(choices) => {
        if (pending !== undefined) {
          const { envelope, items } = pending;
          setPending(undefined);
          void apply(envelope, items, choices);
        } else {
          close();
        }
      }}
      onClose={close}
    />
  );
}
