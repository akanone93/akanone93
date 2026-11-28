// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { type PaintTheme, resolveTextStyle } from "@plux/layout";
import { type CSSProperties, type ReactNode, useContext, useEffect, useRef, useState } from "react";
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
import { PaintThemeContext } from "../panels/style-editors.tsx";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import type { Node } from "./plugin-model.ts";
import { uuidv7 } from "./screens.ts";
import { type EditFn, findNode } from "./select/edits.ts";
import type { Box } from "./select/geometry.ts";
import { type OverlayStore, useOverlayState } from "./select/overlay-store.ts";
import { childNodes } from "./select/tree.ts";
import { CANVAS_FONT_FACES, type CanvasSurface } from "./surface.ts";
import { VARIANT_TYPE } from "./variants.ts";

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

/** Whether `value` (a node's props, at any depth) names translation key `key` (`{"$t": key}`). */
function namesKey(value: unknown, key: string): boolean {
  if (Array.isArray(value)) {
    return value.some((v) => namesKey(v, key));
  }
  if (!isObj(value)) {
    return false;
  }
  return value["$t"] === key || Object.values(value).some((v) => namesKey(v, key));
}

/**
 * Whether an in-place edit of node `id`'s text keyed `key` must take a key of its own (build log row 324): the node is
 * in an arm of a Variant — its control or a variant — and another node of the page names the same key. A variant arm
 * starts as a copy of the control, sharing its keys, so changing the shared message would change every arm alike.
 */
export function armSharesKey(root: Node | undefined, id: string, key: string): boolean {
  let inArm = false;
  let users = 0;
  const visit = (n: Node, armed: boolean) => {
    if (n.id === id) {
      inArm = armed;
    }
    if (namesKey(n.props, key)) {
      users += 1;
    }
    for (const c of childNodes(n)) {
      visit(c, armed || n.type === VARIANT_TYPE);
    }
  };
  if (root !== undefined) {
    visit(root, false);
  }
  return inArm && users > 1;
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

/** `0xRRGGBBAA` as a CSS colour. */
const cssColor = (rgba: number): string =>
  `rgb(${(rgba >>> 24) & 255} ${(rgba >>> 16) & 255} ${(rgba >>> 8) & 255} / ${((rgba & 255) / 255).toFixed(3)})`;

/** A text's `textAlign` as CSS: `start` and `end` follow the writing direction, as Flutter's do. */
const ALIGN: Readonly<Record<string, string>> = {
  left: "left",
  right: "right",
  center: "center",
  justify: "justify",
  start: "start",
  end: "end",
};

/**
 * How the field shows the text (#242): the style the canvas draws it in — face, size at the view's zoom, weight,
 * slant, tracking, line height, colour and alignment — so the person edits the text as the screen shows it, in place.
 */
export function inlineTextStyle(
  node: Node | undefined,
  theme: PaintTheme | null,
  box: Box,
  view: { readonly x: number; readonly y: number; readonly zoom: number },
): CSSProperties {
  const props = (node?.props ?? {}) as Obj;
  const t = resolveTextStyle(props["style"], theme ?? undefined);
  const colour = t.color ?? theme?.scheme.onSurface;
  const align = typeof props["textAlign"] === "string" ? ALIGN[props["textAlign"]] : undefined;
  // The page's text engine can measure a hair wider than the canvas's: a little room on the side the text grows
  // towards keeps a line from wrapping in the field when it did not on the canvas.
  const w = Math.max(24, (box.r - box.l) * view.zoom);
  const room = Math.max(8, w * 0.15);
  const before = align === "center" ? room / 2 : align === "right" || align === "end" ? room : 0;
  return {
    "--x": (box.l - view.x) * view.zoom - before,
    "--y": (box.t - view.y) * view.zoom,
    "--w": w + room,
    "--h": Math.max(8, (box.b - box.t) * view.zoom),
    fontFamily: `"${t.family ?? "Roboto"}", "Noto Sans", "Noto Sans Arabic", sans-serif`,
    fontSize: `${t.size * view.zoom}px`,
    fontWeight: t.weight,
    fontStyle: t.italic ? "italic" : "normal",
    letterSpacing: `${t.letterSpacing * view.zoom}px`,
    lineHeight: t.height,
    ...(colour === undefined ? {} : { color: cssColor(colour), caretColor: cssColor(colour) }),
    ...(align === undefined ? {} : { textAlign: align as CSSProperties["textAlign"] }),
  } as CSSProperties;
}

/** Loads the faces the canvas draws texts in into the page, once each, so the field shows the same face (#242). */
function useCanvasFaces(): void {
  useEffect(() => {
    if (typeof document === "undefined" || document.fonts === undefined || typeof FontFace === "undefined") {
      return;
    }
    const have = new Set([...document.fonts].map((f) => `${f.family.replaceAll('"', "")}/${f.weight}`));
    for (const face of CANVAS_FONT_FACES) {
      if (!have.has(`${face.family}/${face.weight}`)) {
        const font = new FontFace(face.family, `url(${face.url})`, { weight: String(face.weight) });
        document.fonts.add(font);
        void font.load().catch(() => undefined);
      }
    }
  }, []);
}

/**
 * The editor of a text in place on the canvas (PC-17, #242): a field exactly where the text is, in the style the canvas
 * draws it in, on a transparent background, while the canvas leaves that text out — the person edits the text itself,
 * not a copy over it. It follows the view. Enter (or leaving it) saves, Shift+Enter breaks the line, Escape cancels.
 */
export function InlineText(props: {
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly box: Box | undefined;
  readonly node: Node | undefined;
  readonly initial: string;
  readonly onCommit: (text: string) => void;
  readonly onCancel: () => void;
}): ReactNode {
  useOverlayState(props.store);
  useCanvasFaces();
  const theme = useContext(PaintThemeContext);
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
  const lines = Math.max(1, text.split("\n").length);
  const style = inlineTextStyle(props.node, theme, props.box, surface.viewport);
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
  const shown = textValue(node, sessions);
  // A text in a Variant's arm whose key another node names is saved as its own text, then extracted to a new key.
  const current =
    shown.key !== undefined && armSharesKey(screen?.root, editing.id, shown.key) ? { text: shown.text } : shown;
  return (
    <InlineText
      key={editing.id}
      store={props.store}
      surface={props.surface}
      box={props.boxOf(editing.id)}
      node={node}
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
