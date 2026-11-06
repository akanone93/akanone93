// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { resolveColor } from "@plux/layout";
import { valueTypes } from "@plux/schema";
import { Icon, Menu } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { type CSSProperties, createContext, type ReactElement, useContext, useState } from "react";
import { type StyleMessages, styleMessages } from "./style.messages.ts";
import { ColorField, PaintThemeContext, swatch } from "./style-editors.tsx";
import "./state-editors.css";

/**
 * The state and shape editors of the inspector (PC-21, STU-062): a `WidgetState…` value — a button's background,
 * outline, shape or padding per interaction state — edits one state at a time behind a row of state chips, and the
 * canvas shows the widget in the state being edited; a `ShapeBorder` edits its kind, corners, outline and, for a
 * circle, its eccentricity. Numbers bind to the theme's dimension tokens first (token-first values, THM-001).
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** The states a `WidgetState…` value can set, as the editor offers them: the default first, then by how often used. */
export const EDITED_STATES = [
  "value",
  "pressed",
  "hovered",
  "focused",
  "disabled",
  "selected",
  "error",
  "dragged",
] as const;
export type EditedState = (typeof EDITED_STATES)[number];

/** The type a `WidgetState…` value type holds per state (`color` for `WidgetStateColor`), else undefined. */
export function stateInnerOf(type: string): string | undefined {
  const bare = type.replace(/\?$/, "");
  if (!bare.startsWith("WidgetState")) {
    return undefined;
  }
  const fields = valueTypes.find((v) => v.name === bare)?.fields ?? [];
  const value = fields.find((f) => f.name === "value");
  return value !== undefined && fields.some((f) => f.name === "pressed") ? value.type : undefined;
}

/**
 * The state the inspector edits and the canvas shows on the selected widget (PC-21): one for every state editor of the
 * selection, so picking Pressed on the background shows the pressed foreground too. Null outside a canvas.
 */
export interface StatePreview {
  readonly state: EditedState;
  readonly show: (state: EditedState) => void;
}
export const StatePreviewContext = createContext<StatePreview | null>(null);

function useEditedState(): StatePreview {
  const shared = useContext(StatePreviewContext);
  const [state, show] = useState<EditedState>("value");
  return shared ?? { state, show };
}

/** A chip's mark: the state's colour for a colour, a dot when the state is set, nothing when it is not. */
function ChipMark({ value, colour }: { readonly value: unknown; readonly colour: boolean }) {
  const theme = useContext(PaintThemeContext);
  if (value === undefined) {
    return <span className="state-chip-mark" data-empty="" aria-hidden="true" />;
  }
  if (!colour || theme === null) {
    return <span className="state-chip-mark" aria-hidden="true" />;
  }
  return (
    <span className="state-chip-mark state-chip-swatch" aria-hidden="true" style={swatch(resolveColor(value, theme))} />
  );
}

/**
 * A `WidgetState…` value: a chip per state — marked when it is set, in its colour for a colour — and the editor of the
 * state picked; a state left unset falls back to the default, as Flutter resolves it.
 */
export function StateValueEditor(p: {
  readonly name: string;
  readonly inner: string;
  readonly value: unknown;
  readonly readOnly: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
  readonly children: (state: EditedState, value: unknown, set: (value: unknown) => void) => ReactElement;
}): ReactElement {
  const m = styleMessages(useLingui().i18n);
  const { state, show } = useEditedState();
  const o = isObj(p.value) ? p.value : {};
  const set = (nv: unknown) => {
    const next = { ...o };
    if (nv === undefined) {
      delete next[state];
    } else {
      next[state] = nv;
    }
    p.onChange(Object.keys(next).length === 0 ? undefined : next);
  };
  const colour = p.inner === "color";
  return (
    <div className="state-editor">
      <div className="state-chips" role="tablist" aria-label={m.statesOf(p.name)}>
        {EDITED_STATES.map((s) => (
          <button
            key={s}
            type="button"
            role="tab"
            className="state-chip"
            aria-selected={s === state}
            data-set={o[s] !== undefined || undefined}
            title={o[s] === undefined && s !== "value" ? m.stateUnset(m.stateName(s)) : m.stateName(s)}
            onClick={() => show(s)}
          >
            <ChipMark value={o[s]} colour={colour} />
            {m.stateName(s)}
          </button>
        ))}
      </div>
      <div className="state-body" role="tabpanel" aria-label={m.stateName(state)}>
        {p.children(state, o[state], set)}
        {state !== "value" && (
          <p className="state-note">
            <Icon name="eye" size="pill" />
            <span>{o[state] === undefined ? m.stateFallsBack : m.stateShown}</span>
          </p>
        )}
      </div>
    </div>
  );
}

/** The shape kinds of `ShapeKind`, rounded first. */
const SHAPE_KINDS = ["roundedRectangle", "stadium", "circle", "beveledRectangle", "continuousRectangle"] as const;
const CORNERED: ReadonlySet<string> = new Set(["roundedRectangle", "beveledRectangle", "continuousRectangle"]);

function SmallNumber(p: {
  readonly label: string;
  readonly value: number | undefined;
  readonly min?: number;
  readonly max?: number;
  readonly step?: number;
  readonly readOnly: boolean;
  readonly onChange: (v: number | undefined) => void;
  readonly onEnd: () => void;
}) {
  return (
    <Input
      type="number"
      min={p.min ?? 0}
      max={p.max}
      step={p.step ?? 1}
      aria-label={p.label}
      className="shape-number"
      value={p.value ?? ""}
      readOnly={p.readOnly}
      onChange={(e) => p.onChange(e.target.value === "" ? undefined : Math.max(p.min ?? 0, Number(e.target.value)))}
      onBlur={p.onEnd}
    />
  );
}

/** The corner radius of a `BorderRadius` when all four are the same: its `all`, or the one circular radius. */
function radiusOf(r: unknown): number | undefined {
  if (!isObj(r)) {
    return undefined;
  }
  const all = num(r["all"]);
  if (all !== undefined) {
    return all;
  }
  const corners = ["topLeft", "topRight", "bottomLeft", "bottomRight"].map((c) => {
    const v = r[c];
    return num(v) ?? (isObj(v) ? num(v["circular"]) : undefined);
  });
  return corners.every((c) => c === corners[0]) ? corners[0] : undefined;
}

function ShapePreview({ kind, radius }: { readonly kind: string; readonly radius: number | undefined }) {
  const style = { "--shape-radius": String(radius ?? 0) } as CSSProperties;
  return <span className="shape-preview" data-kind={kind} style={style} aria-hidden="true" />;
}

function shapeKindEntries(m: StyleMessages, pick: (k: string) => void) {
  return SHAPE_KINDS.map((k) => ({ kind: "item" as const, id: k, label: m.shapeKind(k), onSelect: () => pick(k) }));
}

/** A `ShapeBorder`: its kind, the corner radius of a rectangle, the outline and a circle's eccentricity. */
export function ShapeEditor(p: {
  readonly name: string;
  readonly value: unknown;
  readonly readOnly: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
}): ReactElement {
  const m = styleMessages(useLingui().i18n);
  const o = isObj(p.value) ? p.value : {};
  const kind = typeof o["kind"] === "string" ? o["kind"] : "roundedRectangle";
  const side = isObj(o["side"]) ? o["side"] : {};
  const radius = radiusOf(o["borderRadius"]);
  const corners = o["borderRadius"] !== undefined && radius === undefined;
  const put = (k: string, v: unknown, end = false) => {
    const next: Obj = { kind, ...o, [k]: v };
    if (v === undefined) {
      delete next[k];
    }
    p.onChange(next);
    if (end) {
      p.onEnd();
    }
  };
  const putSide = (k: string, v: unknown) => {
    const next: Obj = { ...side, [k]: v };
    if (v === undefined) {
      delete next[k];
    }
    put("side", Object.keys(next).length === 0 ? undefined : next);
  };
  return (
    <div className="shape-editor">
      <div className="shape-row">
        <ShapePreview kind={kind} radius={radius} />
        <Menu
          trigger={
            <button type="button" className="shape-kind" disabled={p.readOnly} aria-label={m.shapeOf(p.name)}>
              <span className="truncate">{m.shapeKind(kind)}</span>
              <Icon name="caret-down" size="pill" />
            </button>
          }
          entries={shapeKindEntries(m, (k) => put("kind", k, true))}
        />
      </div>
      {CORNERED.has(kind) && (
        <div className="shape-field">
          <span>{m.radius}</span>
          {corners ? (
            <span className="shape-mixed">{m.eachCornerSet}</span>
          ) : (
            <SmallNumber
              label={m.radius}
              value={radius}
              readOnly={p.readOnly}
              onChange={(v) => put("borderRadius", v === undefined ? undefined : { all: v })}
              onEnd={p.onEnd}
            />
          )}
        </div>
      )}
      {kind === "circle" && (
        <div className="shape-field">
          <span>{m.eccentricity}</span>
          <SmallNumber
            label={m.eccentricity}
            value={num(o["eccentricity"])}
            max={1}
            step={0.1}
            readOnly={p.readOnly}
            onChange={(v) => put("eccentricity", v === undefined ? undefined : Math.min(1, v))}
            onEnd={p.onEnd}
          />
        </div>
      )}
      <div className="shape-field">
        <span>{m.outline}</span>
        <div className="shape-outline">
          <ColorField
            name={`${p.name}.side.color`}
            value={side["color"]}
            readOnly={p.readOnly}
            onChange={(v) => putSide("color", v)}
            onEnd={p.onEnd}
          />
          <SmallNumber
            label={m.outlineWidth}
            value={num(side["width"])}
            step={0.5}
            readOnly={p.readOnly}
            onChange={(v) => putSide("width", v)}
            onEnd={p.onEnd}
          />
        </div>
      </div>
    </div>
  );
}

/** The dimension tokens a number named `prop` binds to first: radii for a radius, spacing for gaps and padding. */
export function tokenFamilyOf(prop: string): string | undefined {
  if (/radius/i.test(prop)) {
    return "radius";
  }
  if (/spacing|gap|padding|margin|indent|inset/i.test(prop)) {
    return "space";
  }
  if (/elevation/i.test(prop)) {
    return "elevation";
  }
  return /width|height|size|extent/i.test(prop) ? "size" : undefined;
}

const groupOf = (path: string): string => path.split(".")[0] ?? path;

/** The theme's dimension tokens for a number named `prop`: its family's first, each family by size. */
export function dimensionChoices(
  prop: string,
  dimensions: Readonly<Record<string, number>>,
): { readonly path: string; readonly px: number }[] {
  const family = tokenFamilyOf(prop);
  const rank = (path: string) => (family !== undefined && path.startsWith(`${family}.`) ? 0 : 1);
  return Object.entries(dimensions)
    .map(([path, px]) => ({ path, px }))
    .sort((a, b) => rank(a.path) - rank(b.path) || groupOf(a.path).localeCompare(groupOf(b.path), "en") || a.px - b.px);
}

type NumberProps = {
  readonly name: string;
  readonly value: unknown;
  readonly readOnly: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
};

/** A number bound to a token: a chip with the token's path and pixels, and the button that unbinds it to them. */
function BoundToken(p: NumberProps & { readonly token: string; readonly px: number | undefined }) {
  const m = styleMessages(useLingui().i18n);
  return (
    <div className="token-number">
      <span className="token-chip" title={p.token}>
        <Icon name="link" size="pill" />
        <span className="truncate">{p.token}</span>
        {p.px !== undefined && <span className="token-px">{p.px}</span>}
      </span>
      {!p.readOnly && (
        <button
          type="button"
          className="token-unbind"
          aria-label={m.unbindToken(p.token)}
          onClick={() => {
            p.onChange(p.px);
            p.onEnd();
          }}
        >
          <Icon name="x" size="pill" />
        </button>
      )}
    </div>
  );
}

/** An unbound number: its field and a menu of the tokens it can bind to. */
function TokenPicker(
  p: NumberProps & {
    readonly choices: readonly { readonly path: string; readonly px: number }[];
    readonly children: ReactElement;
  },
) {
  const m = styleMessages(useLingui().i18n);
  return (
    <div className="token-number">
      {p.children}
      <Menu
        trigger={
          <button type="button" className="token-pick" aria-label={m.bindToken(p.name)} title={m.bindToken(p.name)}>
            <Icon name="link" size="pill" />
          </button>
        }
        entries={p.choices.map((c) => ({
          kind: "item" as const,
          id: c.path,
          label: `${c.path} · ${c.px}`,
          onSelect: () => {
            p.onChange({ $token: c.path });
            p.onEnd();
          },
        }))}
      />
    </div>
  );
}

/**
 * A number with the theme's dimension tokens first (PC-21, THM-001): a bound token shows as a chip with its pixels and
 * unbinds to them; an unbound number offers the tokens of its family. Outside a canvas it is the plain field.
 */
export function TokenNumber(p: NumberProps & { readonly children: ReactElement }): ReactElement {
  const theme = useContext(PaintThemeContext);
  const dims = theme?.dimensions ?? {};
  const token = isObj(p.value) && typeof p.value["$token"] === "string" ? p.value["$token"] : undefined;
  if (token !== undefined) {
    return <BoundToken {...p} token={token} px={dims[token]} />;
  }
  const choices = dimensionChoices(p.name, dims);
  return choices.length === 0 || p.readOnly ? p.children : <TokenPicker {...p} choices={choices} />;
}
