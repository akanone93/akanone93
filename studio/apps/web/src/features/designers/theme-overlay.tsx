// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Icon, Input, TypeTag } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { useEffect, useMemo, useRef, useState } from "react";
import { useCanEdit, useEditSession } from "../editor/editor-provider.tsx";
import type { JsonValue } from "../editor/json-patch.ts";
import { type DesignerMessages, designerMessages } from "./designers.messages.ts";
import { ProblemList } from "./parts.tsx";
import {
  type DesignerDocument,
  findDocument,
  isObject,
  type Json,
  useCheck,
  useDocumentValue,
} from "./use-document.ts";

/** The key under which a token carries its dark-mode value (THM-001). */
export const DARK_KEY = "dev.plux.dark";

export interface ThemeOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The app's documents: the theme is `theme.json`. */
  readonly documents: readonly DesignerDocument[] | undefined;
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
}

export interface TokenRow {
  readonly path: readonly string[];
  readonly type: string;
  readonly light: unknown;
  readonly dark: unknown;
}

/** Every token of the theme with its `$type` (own or inherited from its group), light value and dark value. */
export function flattenTokens(tokens: unknown): TokenRow[] {
  const rows: TokenRow[] = [];
  const walk = (node: unknown, path: string[], type: string) => {
    if (!isObject(node)) {
      return;
    }
    const own = typeof node["$type"] === "string" ? node["$type"] : type;
    if ("$value" in node) {
      const ext = isObject(node["$extensions"]) ? node["$extensions"][DARK_KEY] : undefined;
      rows.push({ path, type: own, light: node["$value"], dark: ext });
    }
    for (const [key, child] of Object.entries(node)) {
      if (!key.startsWith("$")) {
        walk(child, [...path, key], own);
      }
    }
  };
  walk(tokens, [], "");
  return rows;
}

const show = (v: unknown): string => (v === undefined ? "" : typeof v === "string" ? v : JSON.stringify(v));

/** What a text field holds as a token value: text stays text, and a value that was structured stays structured when it still parses. */
export function tokenValue(text: string, before: unknown): unknown {
  if (typeof before === "string" || before === undefined) {
    return text;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return before;
  }
}

function leafAt(tokens: Json, path: readonly string[]): Json | undefined {
  let node: unknown = tokens;
  for (const key of path) {
    node = isObject(node) ? node[key] : undefined;
  }
  return isObject(node) ? node : undefined;
}

function Swatches({ rows, m }: { rows: readonly TokenRow[]; m: DesignerMessages }) {
  const colours = rows.filter((r) => r.type === "color" && typeof r.light === "string");
  return (
    <section aria-label={m.themePreview} className="flex flex-col gap-2">
      <h3 className="text-label font-medium">{m.themePreview}</h3>
      <div className="grid grid-cols-2 gap-3">
        {[m.themeLight, m.themeDark].map((mode, i) => (
          <div key={mode} className="flex flex-col gap-1">
            <span className="text-caption text-secondary">{mode}</span>
            <ul className="flex flex-wrap gap-2">
              {colours.map((r) => {
                const value = i === 0 ? r.light : (r.dark ?? r.light);
                return (
                  <li key={r.path.join(".")} className="flex items-center gap-1">
                    <span
                      aria-hidden="true"
                      className="inline-block size-4 rounded-sm border border-hairline"
                      style={{ backgroundColor: String(value) }}
                    />
                    <span className="font-mono text-caption">{r.path.join(".")}</span>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </div>
    </section>
  );
}

/** The theme editor V8 (STU-063): every change goes through the edit pipeline at once, so the canvas shows it as you type. */
export function ThemeOverlay(props: ThemeOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => designerMessages(i18n), [i18n]);
  const session = useEditSession();
  const { path, value } = useDocumentValue(props.documents, "theme.json");
  const canEdit = useCanEdit();
  // The theme is an app document, so the plugin canvas's session does not track it yet: the overlay opens it.
  const stored = findDocument(props.documents, "theme.json");
  useEffect(() => {
    if (props.open && stored?.revision !== undefined) {
      try {
        session.open({
          path: stored.path,
          revision: stored.revision,
          content: JSON.parse(new TextDecoder().decode(stored.content)) as JsonValue,
        });
      } catch {
        // A theme that is not JSON stays unopened; the validator reports it.
      }
    }
  }, [props.open, stored, session]);
  const check = useCheck(props.open, path, value, props.validatePage);
  const rows = useMemo(() => flattenTokens(value?.["tokens"]), [value]);
  const [newPath, setNewPath] = useState("");
  const [newValue, setNewValue] = useState("");
  const adds = useRef(0);

  const write = (group: string, recipe: (tokens: Json) => void) => {
    if (path === undefined) {
      return;
    }
    session.edit(
      path,
      (draft) => {
        if (isObject(draft["tokens"])) {
          recipe(draft["tokens"]);
        }
      },
      { group: `theme:${group}` },
    );
  };
  const setLight = (row: TokenRow, text: string) =>
    write(`${row.path.join(".")}:light`, (tokens) => {
      const leaf = leafAt(tokens, row.path);
      if (leaf !== undefined) {
        leaf["$value"] = tokenValue(text, row.light);
      }
    });
  const setDark = (row: TokenRow, text: string) =>
    write(`${row.path.join(".")}:dark`, (tokens) => {
      const leaf = leafAt(tokens, row.path);
      if (leaf === undefined) {
        return;
      }
      const ext: Json = isObject(leaf["$extensions"]) ? leaf["$extensions"] : {};
      if (text === "") {
        delete ext[DARK_KEY];
      } else {
        ext[DARK_KEY] = tokenValue(text, row.dark);
      }
      if (Object.keys(ext).length === 0) {
        delete leaf["$extensions"];
      } else {
        leaf["$extensions"] = ext;
      }
    });
  const remove = (row: TokenRow) =>
    write(`${row.path.join(".")}:remove`, (tokens) => {
      const parent = leafAt(tokens, row.path.slice(0, -1));
      const last = row.path.at(-1);
      if (parent !== undefined && last !== undefined) {
        delete parent[last];
      }
    });
  const segments = newPath.split(".").filter((s) => /^[a-zA-Z0-9][a-zA-Z0-9-]*$/.test(s));
  const addable = canEdit && segments.length > 0 && segments.length === newPath.split(".").length && newValue !== "";
  const add = () => {
    adds.current += 1;
    write(`add:${adds.current}`, (tokens) => {
      let node: Json = tokens;
      for (const key of segments.slice(0, -1)) {
        const child = node[key];
        if (isObject(child)) {
          node = child;
        } else {
          const made: Json = {};
          node[key] = made;
          node = made;
        }
      }
      const last = segments.at(-1);
      if (last !== undefined) {
        node[last] = { $value: newValue };
      }
    });
    setNewPath("");
    setNewValue("");
  };
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      size="full"
      title={m.themeTitle}
      labels={{ close: m.themeClose }}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-3">
        <p className="text-caption text-secondary">{value === undefined ? m.themeNone : m.themeLive}</p>
        {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
        <div className="min-h-0 flex-1 overflow-y-auto">
          <table aria-label={m.themeTokens} className="w-full text-body">
            <thead>
              <tr className="text-start text-caption text-secondary">
                <th scope="col">{m.themeToken}</th>
                <th scope="col">{m.themeLight}</th>
                <th scope="col">{m.themeDark}</th>
                <td />
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const name = row.path.join(".");
                return (
                  <tr key={name}>
                    <th scope="row" className="text-start font-mono font-normal">
                      {name} {row.type !== "" && <TypeTag>{row.type}</TypeTag>}
                    </th>
                    <td>
                      <Input
                        aria-label={m.themeLightValue(name)}
                        value={show(row.light)}
                        disabled={!canEdit}
                        onChange={(e) => setLight(row, e.target.value)}
                      />
                    </td>
                    <td>
                      <Input
                        aria-label={m.themeDarkValue(name)}
                        value={show(row.dark)}
                        disabled={!canEdit}
                        onChange={(e) => setDark(row, e.target.value)}
                      />
                    </td>
                    <td>
                      <Button
                        variant="secondary"
                        aria-label={m.themeRemoveToken(name)}
                        disabled={!canEdit}
                        onClick={() => remove(row)}
                      >
                        <Icon name="trash" />
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <div className="mt-3 flex flex-wrap items-end gap-3">
            <Field label={m.themeNewPath}>
              <Input value={newPath} disabled={!canEdit} onChange={(e) => setNewPath(e.target.value)} />
            </Field>
            <Field label={m.themeNewValue}>
              <Input value={newValue} disabled={!canEdit} onChange={(e) => setNewValue(e.target.value)} />
            </Field>
            <Button variant="secondary" disabled={!addable} onClick={add}>
              <Icon name="plus" />
              {m.themeAddToken}
            </Button>
          </div>
        </div>
        <Swatches rows={rows} m={m} />
        <ProblemList problems={check.state === "done" ? check.problems : []} m={m} />
      </div>
    </Dialog>
  );
}
