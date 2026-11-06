// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { widget } from "@plux/schema";
import { Icon, Select } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { createContext, type ReactElement, useContext, useState } from "react";
import { pluralArgs } from "../plugin-canvas/icu.ts";
import { ownOverrides } from "../plugin-canvas/locale-overrides.ts";
import { type LocaleToolsMessages, localeToolsMessages } from "./locale-tools.messages.ts";
import "./locale-tools.css";

/**
 * The previewed locale's tools in the Localisation tab (PC-32, I18N-001, I18N-006, I18N-012): whether the selected
 * texts' keys have a message in the locale, the count a plural message is shown with on the canvas, and the props the
 * widget sets differently in this locale.
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** What the canvas knows of the previewed locale; null outside a canvas. */
export interface LocalePreview {
  readonly locale: string;
  readonly defaultLocale: string;
  /** The message of a key in the previewed locale, falling back to the default's; undefined when it has none. */
  readonly messageOf: (key: string) => string | undefined;
  /** A key's message as a row shows it: in the previewed locale, formatted with no arguments (`{n} items`). */
  readonly textOf: (key: string) => string | undefined;
  readonly missing: (key: string) => boolean;
  readonly sampleOf: (node: string) => number | undefined;
  readonly setSample: (node: string, n: number | undefined) => void;
}
export const LocalePreviewContext = createContext<LocalePreview | null>(null);

const SAMPLES = [0, 1, 2, 5, 21] as const;

/** The translation keys of a node's text props. */
function keysOf(node: Obj): string[] {
  const props = isObj(node["props"]) ? node["props"] : {};
  return Object.values(props).flatMap((v) => (isObj(v) && typeof v["$t"] === "string" ? [v["$t"]] : []));
}

function Samples(p: { readonly id: string; readonly preview: LocalePreview; readonly m: LocaleToolsMessages }) {
  const current = p.preview.sampleOf(p.id);
  return (
    <div className="state-chips" role="radiogroup" aria-label={p.m.sample}>
      {[undefined, ...SAMPLES].map((n) => (
        // biome-ignore lint/a11y/useSemanticElements: a chip of a segmented choice
        <button
          key={n ?? "data"}
          type="button"
          role="radio"
          aria-checked={current === n}
          className="state-chip"
          onClick={() => p.preview.setSample(p.id, n)}
        >
          {n === undefined ? p.m.fromData : String(n)}
        </button>
      ))}
    </div>
  );
}

function Overrides(p: {
  readonly node: Obj;
  readonly locale: string;
  readonly canEdit: boolean;
  readonly m: LocaleToolsMessages;
  readonly set: (prop: string, value: unknown) => void;
}) {
  const own = ownOverrides(p.node, p.locale);
  const props = (widget(String(p.node["type"]))?.props ?? []).map((x) => x.name);
  const [prop, setProp] = useState<string | undefined>();
  const [text, setText] = useState("");
  const chosen = prop !== undefined && props.includes(prop) ? prop : props[0];
  const parse = (t: string): unknown => {
    try {
      return JSON.parse(t) as unknown;
    } catch {
      return t;
    }
  };
  return (
    <div className="locale-overrides">
      {Object.entries(own).map(([k, v]) => (
        <div key={k} className="quick-step">
          <span className="quick-step-event">{k}</span>
          <span className="quick-step-detail">{JSON.stringify(v)}</span>
          {p.canEdit && (
            <button
              type="button"
              className="form-icon"
              aria-label={p.m.removeOverride(k)}
              onClick={() => p.set(k, undefined)}
            >
              <Icon name="x" size="pill" />
            </button>
          )}
        </div>
      ))}
      {p.canEdit && chosen !== undefined && (
        <div className="locale-add">
          <Select
            label={p.m.prop}
            value={chosen}
            options={props.map((x) => ({ value: x, label: x }))}
            onValueChange={setProp}
          />
          <Input
            aria-label={p.m.value}
            placeholder={p.m.value}
            className="quick-input"
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          <button
            type="button"
            className="form-add"
            disabled={text.trim() === ""}
            onClick={() => {
              p.set(chosen, parse(text));
              setText("");
            }}
          >
            <Icon name="plus" size="pill" />
            {p.m.addOverride(p.locale)}
          </button>
        </div>
      )}
    </div>
  );
}

export function LocaleTools(p: {
  readonly id: string;
  readonly node: Obj;
  readonly canEdit: boolean;
  readonly setOverride: (locale: string, prop: string, value: unknown) => void;
}): ReactElement | null {
  const m = localeToolsMessages(useLingui().i18n);
  const preview = useContext(LocalePreviewContext);
  if (preview === null) {
    return null;
  }
  const keys = keysOf(p.node);
  const plural = keys.some((k) => pluralArgs(preview.messageOf(k) ?? "").length > 0);
  return (
    <section className="lay-card" aria-label={m.title(preview.locale)}>
      <h3 className="lay-title">
        <Icon name="translate" size="pill" />
        {m.title(preview.locale)}
      </h3>
      {keys.map((k) => (
        <div key={k} className="locale-key" data-missing={String(preview.missing(k))}>
          <span className="locale-message" dir="auto" lang={preview.locale}>
            {preview.messageOf(k) ?? k}
          </span>
          <span className="locale-status">
            <Icon name={preview.missing(k) ? "warning" : "check"} size="pill" />
            {preview.missing(k) ? m.missing(preview.locale, preview.defaultLocale) : m.translated}
          </span>
        </div>
      ))}
      {plural && (
        <div className="cond-block">
          <span className="lay-label">{m.sample}</span>
          <Samples id={p.id} preview={preview} m={m} />
        </div>
      )}
      <div className="cond-block">
        <span className="lay-label">{m.overrides(preview.locale)}</span>
        <Overrides
          node={p.node}
          locale={preview.locale}
          canEdit={p.canEdit}
          m={m}
          set={(prop, value) => p.setOverride(preview.locale, prop, value)}
        />
      </div>
    </section>
  );
}
