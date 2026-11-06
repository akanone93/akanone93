// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { createContext, type ReactElement, useContext } from "react";
import { type ListState, sourceOfItems } from "../plugin-canvas/list-preview.ts";
import { findNode } from "../plugin-canvas/select/edits.ts";
import { type ListMessages, listMessages } from "./list-panel.messages.ts";
import "./list-panel.css";

/**
 * The list panel of the inspector (PC-24, WGT-012, DAT-011): what a list's item template is repeated for, the state
 * the canvas draws it in — following the Preview's data state, or picked here — and its empty, loading and error
 * views, each added in one step from a template and then designed like any widget.
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The state picked for each list on the canvas (PC-24); null outside a canvas. */
export interface ListPreview {
  readonly stateOf: (id: string) => ListState | undefined;
  readonly show: (id: string, state: ListState | undefined) => void;
}
export const ListPreviewContext = createContext<ListPreview | null>(null);

const SHOWN = ["auto", "items", "empty", "loading", "error", "more"] as const;
const STATE_SLOTS = ["empty", "loading", "error"] as const;

/**
 * A starting view for a list's `state` slot, with fresh ids; an error view's **Try again** reloads `source` (the data
 * source the items come from) with `refreshData`, and has no action without one.
 */
export function stateTemplate(
  state: (typeof STATE_SLOTS)[number],
  m: ListMessages,
  newId: () => string,
  source?: string,
): Obj {
  if (state === "loading") {
    return {
      id: newId(),
      type: "Padding",
      props: { padding: { all: 24 } },
      slots: {
        child: { id: newId(), type: "Center", slots: { child: { id: newId(), type: "CircularProgressIndicator" } } },
      },
    };
  }
  const icon = {
    id: newId(),
    type: "Icon",
    props: { icon: { name: state === "empty" ? "inbox" : "error_outline" }, size: 48 },
  };
  const text = { id: newId(), type: "Text", props: { data: state === "empty" ? m.defaultEmpty : m.defaultError } };
  const retry = {
    id: newId(),
    type: "TextButton",
    ...(source === undefined
      ? {}
      : { events: { onPressed: { steps: [{ id: "retry", action: "refreshData", input: { source } }] } } }),
    slots: { child: { id: newId(), type: "Text", props: { data: m.retry } } },
  };
  return {
    id: newId(),
    type: "Padding",
    props: { padding: { all: 32 } },
    slots: {
      child: {
        id: newId(),
        type: "Column",
        props: { mainAxisSize: "min", spacing: 12 },
        children: state === "error" ? [icon, text, retry] : [icon, text],
      },
    },
  };
}

/** The recipe that gives list `id` a view for `state`. */
export function addStateRecipe(id: string, state: string, view: Obj): (root: unknown) => void {
  return (root) => {
    const node = findNode(root, id)?.node as Obj | undefined;
    if (node !== undefined) {
      node["slots"] = { ...(isObj(node["slots"]) ? node["slots"] : {}), [state]: view };
    }
  };
}

function itemsLine(m: ListMessages, items: unknown, drawn: string | undefined): string {
  if (Array.isArray(items)) {
    return m.itemsLiteral(items.length);
  }
  const expr = isObj(items) && typeof items["$expr"] === "string" ? items["$expr"] : undefined;
  return expr === undefined ? m.itemsNone : m.itemsFrom(expr, drawn ?? "—");
}

export function ListPanel(p: {
  readonly id: string;
  readonly node: Obj;
  readonly canEdit: boolean;
  /** What the canvas draws for the items' expression. */
  readonly drawn: (expr: string) => string | undefined;
  readonly addState: (state: string, view: Obj) => void;
  readonly newId: () => string;
}): ReactElement {
  const m = listMessages(useLingui().i18n);
  const preview = useContext(ListPreviewContext);
  const props = isObj(p.node["props"]) ? p.node["props"] : {};
  const slots = isObj(p.node["slots"]) ? p.node["slots"] : {};
  const items = props["items"];
  const expr = isObj(items) && typeof items["$expr"] === "string" ? items["$expr"] : undefined;
  const shown = preview?.stateOf(p.id) ?? "auto";
  return (
    <section className="lay-card list-panel" aria-label={m.list}>
      <h3 className="lay-title">
        <Icon name="list" size="pill" />
        {m.list}
      </h3>
      <p className="list-items" title={expr}>
        <span className="lay-label">{m.items}</span>
        <span className="list-items-value">{itemsLine(m, items, expr === undefined ? undefined : p.drawn(expr))}</span>
      </p>
      {preview !== null && (
        <div className="list-show">
          <span className="lay-label">{m.show}</span>
          <div className="state-chips" role="radiogroup" aria-label={m.show}>
            {SHOWN.map((s) => (
              // biome-ignore lint/a11y/useSemanticElements: a chip of a segmented choice
              <button
                key={s}
                type="button"
                role="radio"
                aria-checked={shown === s}
                className="state-chip"
                onClick={() => preview.show(p.id, s === "auto" ? undefined : s)}
              >
                {m.stateName(s)}
              </button>
            ))}
          </div>
        </div>
      )}
      <div className="list-states">
        <span className="lay-label">{m.states}</span>
        {STATE_SLOTS.map((s) =>
          slots[s] !== undefined ? (
            <button
              key={s}
              type="button"
              className="list-state"
              data-designed=""
              onClick={() => preview?.show(p.id, s)}
            >
              <Icon name="check" size="pill" />
              {m.stateName(s)} · {m.designed}
            </button>
          ) : (
            <button
              key={s}
              type="button"
              className="list-state"
              disabled={!p.canEdit}
              onClick={() => {
                p.addState(s, stateTemplate(s, m, p.newId, sourceOfItems(items)));
                preview?.show(p.id, s);
              }}
            >
              <Icon name="plus" size="pill" />
              {m.add(s)}
            </button>
          ),
        )}
      </div>
    </section>
  );
}
