// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { type CSSProperties, type ReactNode, useEffect, useRef, useState } from "react";
import type { JsonValue } from "../editor/json-patch.ts";
import { extractKey, type L10nSessions, useL10nSessions } from "../localisation/index.ts";
import {
  KEYS_PATH,
  localePath,
  proposeKey,
  readKeys,
  readMessages,
  setMessageRecipe,
} from "../localisation/l10n-model.ts";
import { setPropRecipe } from "../panels/properties-model.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import type { Node } from "./plugin-model.ts";
import { uuidv7 } from "./screens.ts";
import { type EditFn, findNode } from "./select/edits.ts";
import type { Box } from "./select/geometry.ts";
import { type OverlayStore, useOverlayState } from "./select/overlay-store.ts";
import type { CanvasSurface } from "./surface.ts";

/** A text being edited in place: the `Text` node, its screen, and where it stands. */
export interface InlineEdit {
  readonly id: string;
  readonly screen: string;
}

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** What a text shows: a literal, or a translation key's message in the source locale. */
export function textValue(
  node: Node | undefined,
  sessions: L10nSessions | null,
): { readonly text: string; readonly key?: string } {
  const data = (node?.props as Obj | undefined)?.["data"];
  if (typeof data === "string") {
    return { text: data };
  }
  if (isObj(data) && typeof data["$t"] === "string") {
    const doc = sessions?.messages.store.getState().docs[localePath(sessions.defaultLocale)]?.content;
    return { text: readMessages(doc)[data["$t"]] ?? "", key: data["$t"] };
  }
  return { text: "" };
}

/** Whether the source locale's messages and the keys can be written now. */
const sourceWritable = (s: L10nSessions | null): s is L10nSessions =>
  s !== null && s.locale === s.defaultLocale && s.canEditMessages;

/**
 * Commits a text edited in place (PC-17, I18N-007): a keyed text's source-locale message changes; a literal is written,
 * then extracted to a new translation key named after the page and its words, when the keys and the source messages
 * can be written. Returns the key it was saved under, if any.
 */
export async function commitInlineText(args: {
  readonly edit: EditFn;
  readonly screen: string;
  readonly id: string;
  readonly text: string;
  readonly current: { readonly text: string; readonly key?: string };
  readonly sessions: L10nSessions | null;
  readonly prefix: string;
  readonly schemaVersion: JsonValue | undefined;
}): Promise<string | undefined> {
  const { edit, screen, id, text, current, sessions } = args;
  if (text === current.text) {
    return current.key;
  }
  if (current.key !== undefined) {
    if (sourceWritable(sessions)) {
      sessions.messages.edit(localePath(sessions.defaultLocale), setMessageRecipe(current.key, text), {
        group: `inline:${id}`,
      });
    }
    return current.key;
  }
  edit(screen, setPropRecipe(id, "compact", "data", text), { group: `inline:${id}` });
  if (!sourceWritable(sessions) || !sessions.canEditKeys || text.trim() === "") {
    return undefined;
  }
  const taken = new Set(readKeys(sessions.keys.store.getState().docs[KEYS_PATH]?.content).map((k) => k.key));
  const key = proposeKey(text, args.prefix, taken);
  const result = await extractKey({
    keys: sessions.keys,
    source: sessions.messages,
    sourceLocale: sessions.defaultLocale,
    schemaVersion: args.schemaVersion ?? 1,
    newId: uuidv7,
    key,
    text,
    replaceProp: (reference, step) => edit(screen, setPropRecipe(id, "compact", "data", reference), { step }),
  });
  return result.ok ? key : undefined;
}

/**
 * The editor of a text in place on the canvas (PC-17): a field over the text, at its size on screen, that follows the
 * view. Enter (or leaving it) saves, Shift+Enter breaks the line, Escape cancels.
 */
export function InlineText(props: {
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly box: Box | undefined;
  readonly initial: string;
  readonly onCommit: (text: string) => void;
  readonly onCancel: () => void;
}): ReactNode {
  useOverlayState(props.store);
  const m = pluginCanvasMessages(useLingui().i18n);
  const [text, setText] = useState(props.initial);
  const ref = useRef<HTMLTextAreaElement>(null);
  const done = useRef(false);
  const focused = useRef(false);
  useEffect(() => {
    // The canvas takes the focus as the double-click ends; the field takes it once that is over.
    const t = setTimeout(() => {
      ref.current?.focus();
      ref.current?.select();
    }, 60);
    return () => clearTimeout(t);
  }, []);
  const surface = props.surface();
  if (surface === null || props.box === undefined) {
    return null;
  }
  const v = surface.viewport;
  const b = props.box;
  const height = Math.max(16, (b.b - b.t) * v.zoom);
  const lines = Math.max(1, text.split("\n").length);
  const style = {
    "--x": (b.l - v.x) * v.zoom,
    "--y": (b.t - v.y) * v.zoom,
    "--w": Math.max(96, (b.r - b.l) * v.zoom + 24),
    "--fs": Math.min(48, Math.max(11, (height / lines) * 0.7)),
  } as CSSProperties;
  const finish = (save: boolean) => {
    if (done.current) {
      return;
    }
    done.current = true;
    if (save) {
      props.onCommit(text);
    } else {
      props.onCancel();
    }
  };
  return (
    <textarea
      ref={ref}
      className="pc-inline-text"
      aria-label={m.inlineText}
      value={text}
      rows={lines}
      style={style}
      onChange={(e) => setText(e.target.value)}
      onFocus={() => {
        focused.current = true;
      }}
      onBlur={() => {
        // Leaving the field saves it; a blur before it ever had the focus is the double-click's, not the user's.
        if (focused.current) {
          finish(true);
        }
      }}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === "Escape") {
          finish(false);
        } else if (e.key === "Enter" && !e.shiftKey) {
          e.preventDefault();
          finish(true);
        }
      }}
    />
  );
}

/**
 * The text being edited in place, if any, with what saving it does (PC-17): the editor over the text, and the commit
 * that writes the text or its source message and announces the key it was saved under.
 */
export function InlineTextHost(props: {
  readonly editing: InlineEdit | null;
  readonly onDone: () => void;
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly boxOf: (id: string) => Box | undefined;
  readonly screens: readonly { readonly id: string; readonly route?: string; readonly root: Node | undefined }[];
  readonly edit: EditFn;
  readonly schemaVersion: JsonValue | undefined;
  readonly announce: (text: string) => void;
}): ReactNode {
  const m = pluginCanvasMessages(useLingui().i18n);
  const sessions = useL10nSessions();
  const { editing } = props;
  if (editing === null) {
    return null;
  }
  const screen = props.screens.find((s) => s.id === editing.screen);
  const found = screen?.root === undefined ? null : findNode(screen.root, editing.id);
  const node = found === null ? undefined : (found.node as Node);
  const current = textValue(node, sessions);
  return (
    <InlineText
      key={editing.id}
      store={props.store}
      surface={props.surface}
      box={props.boxOf(editing.id)}
      initial={current.text}
      onCancel={props.onDone}
      onCommit={(text) => {
        props.onDone();
        void commitInlineText({
          edit: props.edit,
          screen: editing.screen,
          id: editing.id,
          text,
          current,
          sessions,
          prefix: screen?.route ?? "text",
          schemaVersion: props.schemaVersion,
        }).then((key) => {
          if (key !== undefined && key !== current.key) {
            props.announce(m.inlineSaved(key));
          }
        });
      }}
    />
  );
}
