// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { createContext, type ReactElement, useContext, useState } from "react";
import {
  hexOf,
  matchingToken,
  newTokenName,
  rgbaOf,
  type StyleValue,
  styleValuesOf,
} from "../plugin-canvas/theme-round-trip.ts";
import { PaintThemeContext } from "./style-editors.tsx";
import { type ThemeCardMessages, themeCardMessages } from "./theme-card.messages.ts";
import "./theme-card.css";

/**
 * The selection's Theme card (PC-35, THM-001): each colour and size the widget sets — a token with its value, the
 * widgets that use it and an editor that changes it for all of them, or a raw value with the theme's matching token
 * or a new token to make of it.
 */

/** What the canvas offers the card: where a token is used, and writing the theme. */
export interface ThemeRoundTrip {
  readonly users: (token: string) => readonly string[];
  readonly select: (ids: readonly string[]) => void;
  readonly canEditTheme: boolean;
  /** Sets a token's value in the theme. */
  readonly setToken: (token: string, kind: StyleValue["kind"], value: number) => void;
  /** Adds a token of a raw value to the theme. */
  readonly addToken: (name: string, value: StyleValue) => void;
}
export const ThemeRoundTripContext = createContext<ThemeRoundTrip | null>(null);

type Obj = Record<string, unknown>;

const label = (v: StyleValue) => v.path.join(".");

/** A token's current value: a colour field or a number of pixels. */
function TokenValue(p: {
  readonly v: StyleValue & { readonly token: string };
  readonly current: number | undefined;
  readonly rt: ThemeRoundTrip;
  readonly m: ThemeCardMessages;
}) {
  const [text, setText] = useState<string | undefined>();
  if (p.current === undefined) {
    return null;
  }
  if (p.v.kind === "color") {
    const rgb = hexOf(p.current).slice(0, 7).toLowerCase();
    const alpha = hexOf(p.current).slice(7);
    return (
      <input
        type="color"
        className="theme-color"
        aria-label={p.m.value(p.v.token)}
        value={rgb}
        disabled={!p.rt.canEditTheme}
        onChange={(e) => {
          const c = rgbaOf(`${e.target.value}${alpha}`);
          if (c !== undefined) {
            p.rt.setToken(p.v.token, "color", c);
          }
        }}
      />
    );
  }
  return (
    <Input
      type="number"
      className="theme-px"
      aria-label={p.m.value(p.v.token)}
      value={text ?? String(p.current)}
      readOnly={!p.rt.canEditTheme}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => {
        const n = Number(text);
        if (text !== undefined && text.trim() !== "" && Number.isFinite(n)) {
          p.rt.setToken(p.v.token, "dimension", n);
        }
        setText(undefined);
      }}
    />
  );
}

function Swatch({ v, colour }: { readonly v: StyleValue; readonly colour: number | undefined }) {
  return v.kind === "color" && colour !== undefined ? (
    <span className="theme-swatch" style={{ backgroundColor: hexOf(colour) }} aria-hidden="true" />
  ) : (
    <Icon name="sliders" size="pill" />
  );
}

function BoundRow(p: { readonly v: StyleValue & { readonly token: string }; readonly rt: ThemeRoundTrip }) {
  const m = themeCardMessages(useLingui().i18n);
  const theme = useContext(PaintThemeContext);
  const current = p.v.kind === "color" ? theme?.tokens?.[p.v.token] : theme?.dimensions?.[p.v.token];
  const users = p.rt.users(p.v.token);
  return (
    <div className="theme-row">
      <span className="theme-prop">{label(p.v)}</span>
      <span className="theme-token">
        <Swatch v={p.v} colour={current} />
        <span className="truncate">{p.v.token}</span>
        <TokenValue v={p.v} current={current} rt={p.rt} m={m} />
      </span>
      <button
        type="button"
        className="theme-users"
        aria-label={m.select(p.v.token)}
        title={m.select(p.v.token)}
        onClick={() => p.rt.select(users)}
      >
        <Icon name="crosshair" size="pill" />
        {m.usedBy(users.length)}
      </button>
    </div>
  );
}

function RawRow(p: {
  readonly v: StyleValue;
  readonly rt: ThemeRoundTrip;
  readonly canEdit: boolean;
  readonly bind: (v: StyleValue, token: string) => void;
}) {
  const m = themeCardMessages(useLingui().i18n);
  const theme = useContext(PaintThemeContext);
  const match = matchingToken(p.v, theme);
  const taken = new Set([...Object.keys(theme?.tokens ?? {}), ...Object.keys(theme?.dimensions ?? {})]);
  const [name, setName] = useState<string | undefined>();
  const shown = p.v.kind === "color" && typeof p.v.raw === "number" ? hexOf(p.v.raw) : String(p.v.raw);
  return (
    <div className="theme-row">
      <span className="theme-prop">{label(p.v)}</span>
      <span className="theme-token" data-raw="">
        <Swatch v={p.v} colour={typeof p.v.raw === "number" && p.v.kind === "color" ? p.v.raw : undefined} />
        <span className="truncate">{shown}</span>
        <span className="theme-raw">{m.raw}</span>
      </span>
      {p.canEdit && match !== undefined && (
        <button type="button" className="theme-action" onClick={() => p.bind(p.v, match)}>
          <Icon name="link" size="pill" />
          {m.bindTo(match)}
        </button>
      )}
      {p.canEdit && match === undefined && p.rt.canEditTheme && name === undefined && (
        <button type="button" className="theme-action" onClick={() => setName(newTokenName(p.v, taken))}>
          <Icon name="plus" size="pill" />
          {m.makeToken}
        </button>
      )}
      {name !== undefined && (
        <form
          className="theme-make"
          onSubmit={(e) => {
            e.preventDefault();
            const n = name.trim();
            if (/^[a-zA-Z][\w-]*(\.[\w-]+)+$/.test(n) && !taken.has(n)) {
              p.rt.addToken(n, p.v);
              p.bind(p.v, n);
              setName(undefined);
            }
          }}
        >
          <Input
            aria-label={m.tokenName}
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="theme-name"
          />
          <button type="submit" className="theme-action">
            <Icon name="check" size="pill" />
            {m.makeToken}
          </button>
        </form>
      )}
    </div>
  );
}

export function ThemeCard(p: {
  readonly node: Obj;
  readonly canEdit: boolean;
  readonly bind: (v: StyleValue, token: string) => void;
}): ReactElement | null {
  const m = themeCardMessages(useLingui().i18n);
  const rt = useContext(ThemeRoundTripContext);
  if (rt === null) {
    return null;
  }
  const values = styleValuesOf(p.node);
  return (
    <section className="lay-card" aria-label={m.title}>
      <h3 className="lay-title">
        <Icon name="palette" size="pill" />
        {m.title}
      </h3>
      {values.length === 0 ? (
        <p className="theme-hint">{m.none}</p>
      ) : (
        <>
          <p className="theme-hint">{rt.canEditTheme ? m.live : m.themeLocked}</p>
          {values.map((v) =>
            v.token === undefined ? (
              <RawRow key={label(v)} v={v} rt={rt} canEdit={p.canEdit} bind={p.bind} />
            ) : (
              <BoundRow key={label(v)} v={v as StyleValue & { token: string }} rt={rt} />
            ),
          )}
        </>
      )}
    </section>
  );
}
