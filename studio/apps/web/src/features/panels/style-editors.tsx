// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import {
  type ColorRole,
  type PaintTheme,
  type ResolvedText,
  resolveColor,
  resolveTextStyle,
  roleColor,
  typeRoleOf,
  typeRoles,
} from "@plux/layout";
import { Select } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { Popover, PopoverContent, PopoverTrigger } from "@plux/ui/components/popover";
import { Switch } from "@plux/ui/components/switch";
import { type CSSProperties, createContext, type ReactElement, useContext } from "react";
import { type StyleMessages, styleMessages } from "./style.messages.ts";
import "./style-editors.css";

/**
 * The style editors of the inspector (STU-062, THM-001): a colour field that offers the theme's colour roles and tokens
 * as swatches and a custom colour with its opacity, and a decoration editor for fill, gradient, corners, border and
 * shadow. Swatches show the colours of the screens on the canvas, for the previewed mode and brand.
 */

/** The colours the canvas paints with, for the swatches; null outside a canvas. */
export const PaintThemeContext = createContext<PaintTheme | null>(null);

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** The roles offered as swatches: the accents and their containers, the surfaces, the texts and the outlines. */
const SWATCH_ROLES: readonly ColorRole[] = [
  "primary",
  "onPrimary",
  "primaryContainer",
  "onPrimaryContainer",
  "secondary",
  "secondaryContainer",
  "tertiary",
  "tertiaryContainer",
  "error",
  "errorContainer",
  "surface",
  "surfaceContainerLow",
  "surfaceContainer",
  "surfaceContainerHigh",
  "surfaceContainerHighest",
  "onSurface",
  "onSurfaceVariant",
  "outline",
  "outlineVariant",
  "inverseSurface",
];

const hex2 = (n: number) => n.toString(16).padStart(2, "0");
/** RGBA as a CSS colour, built from its digits (no literal in the source). */
const cssOf = (rgba: number) => `#${(rgba >>> 0).toString(16).padStart(8, "0")}`;
export const swatch = (rgba: number | undefined): CSSProperties =>
  ({ "--swatch": rgba === undefined ? "transparent" : cssOf(rgba) }) as CSSProperties;

/** The RRGGBB digits and the opacity in percent of a literal colour value. */
function partsOf(value: unknown, theme: PaintTheme | null): { digits: string; alpha: number } {
  const rgba = typeof value === "string" || theme === null ? undefined : resolveColor(value, theme);
  const literal = typeof value === "string" && /^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$/.test(value) ? value : undefined;
  if (literal !== undefined) {
    return {
      digits: literal.slice(1, 7).toUpperCase(),
      alpha: literal.length === 9 ? Math.round((Number.parseInt(literal.slice(7), 16) / 255) * 100) : 100,
    };
  }
  if (rgba !== undefined) {
    return {
      digits: (rgba >>> 8).toString(16).padStart(6, "0").toUpperCase(),
      alpha: Math.round(((rgba & 0xff) / 255) * 100),
    };
  }
  return { digits: "000000", alpha: 100 };
}

/** A literal colour from its digits and opacity, without an alpha at full opacity, as the runtime reads it. */
const literalOf = (digits: string, alpha: number): string =>
  alpha >= 100 ? `#${digits}` : `#${digits}${hex2(Math.round((Math.max(0, alpha) / 100) * 255))}`;

function Swatches({
  title,
  entries,
  current,
  onPick,
}: {
  readonly title: string;
  readonly entries: readonly { readonly path: string; readonly label: string; readonly rgba: number }[];
  readonly current: string | undefined;
  readonly onPick: (path: string) => void;
}): ReactElement {
  return (
    <section aria-label={title} className="flex flex-col gap-1.5">
      <p className="color-popover-title">{title}</p>
      <div className="color-grid">
        {entries.map((e) => (
          <button
            key={e.path}
            type="button"
            className="color-chip"
            title={e.label}
            aria-label={e.label}
            aria-pressed={current === e.path}
            style={swatch(e.rgba)}
            onClick={() => onPick(e.path)}
          />
        ))}
      </div>
    </section>
  );
}

/** A colour property: a swatch that opens the theme's roles and tokens and a custom colour with its opacity. */
export function ColorField({
  name,
  value,
  mixed = false,
  readOnly = false,
  onChange,
  onEnd,
}: {
  readonly name: string;
  readonly value: unknown;
  readonly mixed?: boolean;
  readonly readOnly?: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
}): ReactElement {
  const m = styleMessages(useLingui().i18n);
  const theme = useContext(PaintThemeContext);
  const token = isObj(value) && typeof value["$token"] === "string" ? value["$token"] : undefined;
  const resolved = theme === null ? undefined : resolveColor(value, theme);
  const text = token?.replace(/^color\./, "") ?? (typeof value === "string" ? value : mixed ? "—" : m.none);
  const parts = partsOf(value, theme);
  const set = (next: unknown) => {
    onChange(next);
    onEnd();
  };
  const roles =
    theme === null ? [] : SWATCH_ROLES.map((r) => ({ path: `color.${r}`, label: r, rgba: roleColor(theme, r) }));
  const tokens = Object.entries(theme?.tokens ?? {})
    .filter(([path]) => !SWATCH_ROLES.some((r) => path === `color.${r}`))
    .map(([path, rgba]) => ({ path, label: path, rgba }));
  return (
    <Popover>
      <PopoverTrigger
        disabled={readOnly}
        render={
          <button type="button" className="color-field" aria-label={`${m.pick(name)}: ${text}`}>
            <span className="color-swatch" style={swatch(resolved)} aria-hidden="true" />
            <span className="min-w-0 flex-1 truncate text-start">{text}</span>
          </button>
        }
      />
      <PopoverContent align="start" className="color-popover">
        {roles.length > 0 && (
          <Swatches title={m.themeColors} entries={roles} current={token} onPick={(p) => set({ $token: p })} />
        )}
        {tokens.length > 0 && (
          <Swatches title={m.tokenColors} entries={tokens} current={token} onPick={(p) => set({ $token: p })} />
        )}
        <section aria-label={m.custom} className="flex flex-col gap-1.5">
          <p className="color-popover-title">{m.custom}</p>
          <div className="flex items-center gap-2">
            <input
              type="color"
              aria-label={m.custom}
              value={`#${parts.digits.toLowerCase()}`}
              onChange={(e) => onChange(literalOf(e.target.value.slice(1).toUpperCase(), parts.alpha))}
              onBlur={onEnd}
            />
            <Input
              aria-label={m.hex}
              className="h-8 font-mono"
              defaultValue={parts.digits}
              key={parts.digits}
              maxLength={6}
              onBlur={(e) => {
                const d = e.target.value.replace(/^#/, "").toUpperCase();
                if (/^[0-9A-F]{6}$/.test(d)) {
                  set(literalOf(d, parts.alpha));
                }
              }}
            />
          </div>
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <span className="w-14">{m.opacity}</span>
            <input
              type="range"
              min={0}
              max={100}
              value={parts.alpha}
              className="flex-1"
              onChange={(e) => onChange(literalOf(parts.digits, Number(e.target.value)))}
              onPointerUp={onEnd}
            />
            <span className="w-9 text-end tabular-nums">{parts.alpha}%</span>
          </label>
        </section>
        <button type="button" className="color-clear" onClick={() => set(undefined)}>
          {m.clear}
        </button>
      </PopoverContent>
    </Popover>
  );
}

const DIRECTIONS: Readonly<Record<string, readonly [Obj, Obj]>> = {
  down: [
    { x: 0, y: -1 },
    { x: 0, y: 1 },
  ],
  across: [
    { start: -1, y: 0 },
    { start: 1, y: 0 },
  ],
  diagonal: [
    { start: -1, y: -1 },
    { start: 1, y: 1 },
  ],
  rising: [
    { start: -1, y: 1 },
    { start: 1, y: -1 },
  ],
};

/** The direction preset a gradient's `begin` and `end` match, else none. */
function directionOf(g: Obj): string {
  const same = (a: unknown, b: Obj) => JSON.stringify(a) === JSON.stringify(b);
  return Object.entries(DIRECTIONS).find(([, [b, e]]) => same(g["begin"], b) && same(g["end"], e))?.[0] ?? "down";
}

/** Material elevations offered as shadow presets: the umbra, penumbra and ambient of `kElevationToShadow`. */
const ELEVATIONS: Readonly<Record<string, readonly (readonly [number, number, number, number])[]>> = {
  "1": [
    [2, 1, -1, 0x33],
    [1, 1, 0, 0x24],
    [1, 3, 0, 0x1f],
  ],
  "2": [
    [3, 1, -2, 0x33],
    [2, 2, 0, 0x24],
    [1, 5, 0, 0x1f],
  ],
  "3": [
    [3, 3, -2, 0x33],
    [3, 4, 0, 0x24],
    [1, 8, 0, 0x1f],
  ],
  "4": [
    [2, 4, -1, 0x33],
    [4, 5, 0, 0x24],
    [1, 10, 0, 0x1f],
  ],
  "6": [
    [3, 5, -1, 0x33],
    [6, 10, 0, 0x24],
    [1, 18, 0, 0x1f],
  ],
  "8": [
    [5, 5, -3, 0x33],
    [8, 10, 1, 0x24],
    [3, 14, 2, 0x1f],
  ],
  "12": [
    [7, 8, -4, 0x33],
    [12, 17, 2, 0x24],
    [5, 22, 4, 0x1f],
  ],
};

const shadowsFor = (level: string): Obj[] =>
  (ELEVATIONS[level] ?? []).map(([dy, blur, spread, alpha]) => ({
    color: literalOf("000000", Math.round((alpha / 255) * 100)),
    offset: { dx: 0, dy },
    blurRadius: blur,
    spreadRadius: spread,
  }));

function elevationOf(shadows: unknown): string {
  if (!Array.isArray(shadows) || shadows.length === 0) {
    return "none";
  }
  const json = JSON.stringify(shadows);
  return Object.keys(ELEVATIONS).find((k) => JSON.stringify(shadowsFor(k)) === json) ?? "custom";
}

function Row({ label, children }: { readonly label: string; readonly children: ReactElement | ReactElement[] }) {
  return (
    <div className="deco-row">
      <span className="deco-label">{label}</span>
      <div className="deco-value">{children}</div>
    </div>
  );
}

function NumberBox({
  label,
  value,
  onChange,
  onEnd,
}: {
  readonly label: string;
  readonly value: number | undefined;
  readonly onChange: (v: number | undefined) => void;
  readonly onEnd: () => void;
}) {
  return (
    <Input
      type="number"
      min={0}
      aria-label={label}
      className="h-8"
      value={value ?? ""}
      onChange={(e) => onChange(e.target.value === "" ? undefined : Math.max(0, Number(e.target.value)))}
      onBlur={onEnd}
    />
  );
}

const CORNERS = ["topLeft", "topRight", "bottomRight", "bottomLeft"] as const;

function CornersEditor({
  d,
  m,
  set,
  onEnd,
}: {
  d: Obj;
  m: StyleMessages;
  set: (k: string, v: unknown) => void;
  onEnd: () => void;
}) {
  const r = isObj(d["borderRadius"]) ? d["borderRadius"] : {};
  const each = CORNERS.some((c) => r[c] !== undefined);
  const circle = d["shape"] === "circle";
  const corner = (c: (typeof CORNERS)[number]) => {
    const v = r[c];
    return num(v) ?? (isObj(v) ? num(v["circular"]) : undefined);
  };
  const setRadius = (next: Obj) => set("borderRadius", Object.keys(next).length === 0 ? undefined : next);
  return (
    <fieldset className="deco-section">
      <legend>{m.corners}</legend>
      <Row label={m.circle}>
        <Switch
          checked={circle}
          onCheckedChange={(on) => {
            set("shape", on ? "circle" : undefined);
            onEnd();
          }}
        />
      </Row>
      {!circle && (
        <>
          <Row label={m.eachCorner}>
            <Switch
              checked={each}
              onCheckedChange={(on) => {
                const all = num(r["all"]) ?? 0;
                setRadius(
                  on ? Object.fromEntries(CORNERS.map((c) => [c, { circular: all }])) : { all: corner("topLeft") ?? 0 },
                );
                onEnd();
              }}
            />
          </Row>
          {each ? (
            <div className="grid grid-cols-2 gap-2">
              {CORNERS.map((c) => (
                <div key={c} className="flex flex-col gap-1 text-xs text-muted-foreground">
                  {m.corner(c)}
                  <NumberBox
                    label={m.corner(c)}
                    value={corner(c)}
                    onChange={(v) => setRadius({ ...r, [c]: v === undefined ? undefined : { circular: v } })}
                    onEnd={onEnd}
                  />
                </div>
              ))}
            </div>
          ) : (
            <Row label={m.radius}>
              <NumberBox
                label={m.radius}
                value={num(r["all"])}
                onChange={(v) => setRadius(v === undefined ? {} : { all: v })}
                onEnd={onEnd}
              />
            </Row>
          )}
        </>
      )}
    </fieldset>
  );
}

function GradientEditor({
  d,
  m,
  set,
  onEnd,
}: {
  d: Obj;
  m: StyleMessages;
  set: (k: string, v: unknown) => void;
  onEnd: () => void;
}) {
  const g = isObj(d["gradient"]) ? d["gradient"] : undefined;
  const kind = typeof g?.["kind"] === "string" ? g["kind"] : "none";
  const colors = Array.isArray(g?.["colors"]) ? (g["colors"] as unknown[]) : [];
  const update = (patch: Obj) => set("gradient", { ...(g ?? {}), ...patch });
  return (
    <fieldset className="deco-section">
      <legend>{m.gradient}</legend>
      <Select
        label={m.gradient}
        value={kind}
        options={["none", "linear", "radial", "sweep"].map((k) => ({ value: k, label: m.gradientKind(k) }))}
        onValueChange={(k) => {
          set(
            "gradient",
            k === "none"
              ? undefined
              : {
                  kind: k,
                  colors: colors.length >= 2 ? colors : [{ $token: "color.primary" }, { $token: "color.tertiary" }],
                  ...(k === "linear" ? { begin: DIRECTIONS["down"]?.[0], end: DIRECTIONS["down"]?.[1] } : {}),
                },
          );
          onEnd();
        }}
      />
      {g !== undefined && (
        <>
          {kind === "linear" && (
            <Row label={m.direction}>
              <Select
                label={m.direction}
                value={directionOf(g)}
                options={Object.keys(DIRECTIONS).map((k) => ({ value: k, label: m.directionName(k) }))}
                onValueChange={(k) => {
                  update({ begin: DIRECTIONS[k]?.[0], end: DIRECTIONS[k]?.[1] });
                  onEnd();
                }}
              />
            </Row>
          )}
          <Row label={m.from}>
            <ColorField
              name={m.from}
              value={colors[0]}
              onChange={(c) => update({ colors: [c, colors[1]] })}
              onEnd={onEnd}
            />
          </Row>
          <Row label={m.to}>
            <ColorField
              name={m.to}
              value={colors[1]}
              onChange={(c) => update({ colors: [colors[0], c] })}
              onEnd={onEnd}
            />
          </Row>
        </>
      )}
    </fieldset>
  );
}

/** A BoxDecoration: fill, gradient, corners (or a circle), border and a shadow by Material elevation. */
export function DecorationEditor({
  name,
  value,
  readOnly = false,
  onChange,
  onEnd,
}: {
  readonly name: string;
  readonly value: unknown;
  readonly readOnly?: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
}): ReactElement {
  const m = styleMessages(useLingui().i18n);
  const d = isObj(value) ? value : {};
  const set = (key: string, v: unknown) => {
    const next = { ...d };
    if (v === undefined) {
      delete next[key];
    } else {
      next[key] = v;
    }
    onChange(Object.keys(next).length === 0 ? undefined : next);
  };
  const border = isObj(d["border"]) && isObj(d["border"]["all"]) ? d["border"]["all"] : undefined;
  const setBorder = (patch: Obj) => {
    const side = { ...(border ?? { width: 1, color: { $token: "color.outlineVariant" } }), ...patch };
    set("border", num(side["width"]) === 0 ? undefined : { all: side });
  };
  return (
    <fieldset className="deco-editor" aria-label={name} disabled={readOnly}>
      <div className="deco-section">
        <Row label={m.fill}>
          <ColorField name={`${name} ${m.fill}`} value={d["color"]} onChange={(c) => set("color", c)} onEnd={onEnd} />
        </Row>
      </div>
      <GradientEditor d={d} m={m} set={set} onEnd={onEnd} />
      <CornersEditor d={d} m={m} set={set} onEnd={onEnd} />
      <fieldset className="deco-section">
        <legend>{m.border}</legend>
        <Row label={m.width}>
          <NumberBox
            label={`${m.border} ${m.width}`}
            value={num(border?.["width"])}
            onChange={(w) => setBorder({ width: w ?? 0 })}
            onEnd={onEnd}
          />
        </Row>
        {border !== undefined && (
          <Row label={m.fill}>
            <ColorField
              name={`${m.border} ${m.fill}`}
              value={border["color"]}
              onChange={(c) => setBorder({ color: c })}
              onEnd={onEnd}
            />
          </Row>
        )}
      </fieldset>
      <fieldset className="deco-section">
        <legend>{m.shadow}</legend>
        <Select
          label={m.shadow}
          value={elevationOf(d["boxShadow"])}
          options={[
            "none",
            ...Object.keys(ELEVATIONS),
            ...(elevationOf(d["boxShadow"]) === "custom" ? ["custom"] : []),
          ].map((k) => ({ value: k, label: m.elevation(k) }))}
          onValueChange={(k) => {
            if (k !== "custom") {
              set("boxShadow", k === "none" ? undefined : shadowsFor(k));
              onEnd();
            }
          }}
        />
      </fieldset>
    </fieldset>
  );
}

/** The text style presets: a custom style, the type scale's roles, then the theme's other typography tokens. */
function textPresets(theme: PaintTheme | null, token: string | undefined): string[] {
  const own = Object.keys(theme?.textTokens ?? {}).filter((p) => typeRoleOf(p) === undefined);
  const known = ["custom", ...typeRoles.map((r) => `typography.${r}`), ...own.sort()];
  return token === undefined || known.includes(token) ? known : [...known, token];
}

function presetLabel(m: StyleMessages, path: string): string {
  if (path === "custom") {
    return m.typeRole("custom");
  }
  const role = typeRoleOf(path);
  return role === undefined ? path : m.typeRole(role);
}

/** A custom TextStyle with the measured fields of `t`, as a document writes them (`fontWeight: "w500"`). */
function customFrom(t: ResolvedText): Obj {
  return {
    ...(t.family === undefined ? {} : { fontFamily: t.family }),
    fontSize: t.size,
    fontWeight: `w${t.weight}`,
    height: t.height,
    letterSpacing: t.letterSpacing,
  };
}

/**
 * A TextStyle: a role of the theme's type scale (a `typography.<role>` token) or another typography token of the theme,
 * or a custom style — size, weight, italic, colour, tracking, line height and a decoration line. Editing a field of a
 * token-bound style starts a custom style from what the token resolves to.
 */
export function TextStyleEditor({
  name,
  value,
  readOnly = false,
  onChange,
  onEnd,
}: {
  readonly name: string;
  readonly value: unknown;
  readonly readOnly?: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
}): ReactElement {
  const m = styleMessages(useLingui().i18n);
  const theme = useContext(PaintThemeContext);
  const token = isObj(value) && typeof value["$token"] === "string" ? value["$token"] : undefined;
  const custom: Obj = isObj(value) && token === undefined ? value : {};
  const shown = resolveTextStyle(value, theme ?? undefined);
  const set = (field: string, v: unknown) => {
    const from: Obj = token === undefined ? { ...custom } : customFrom(shown);
    if (v === undefined) {
      delete from[field];
    } else {
      from[field] = v;
    }
    onChange(Object.keys(from).length === 0 ? undefined : from);
  };
  return (
    <fieldset className="deco-editor" aria-label={name} disabled={readOnly}>
      <Select
        label={m.textPreset}
        value={token ?? "custom"}
        options={textPresets(theme, token).map((path) => ({ value: path, label: presetLabel(m, path) }))}
        onValueChange={(path) => {
          onChange(path === "custom" ? customFrom(shown) : { $token: path });
          onEnd();
        }}
      />
      <div className="grid grid-cols-2 gap-2">
        <div className="flex flex-col gap-1 text-xs text-muted-foreground">
          {m.size}
          <NumberBox
            label={m.size}
            value={token === undefined ? num(custom["fontSize"]) : shown.size}
            onChange={(v) => set("fontSize", v)}
            onEnd={onEnd}
          />
        </div>
        <div className="flex flex-col gap-1 text-xs text-muted-foreground">
          {m.weight}
          <Select
            label={m.weight}
            value={String(shown.weight)}
            options={[100, 200, 300, 400, 500, 600, 700, 800, 900].map((w) => ({ value: String(w), label: String(w) }))}
            onValueChange={(w) => {
              set("fontWeight", `w${w}`);
              onEnd();
            }}
          />
        </div>
        <div className="flex flex-col gap-1 text-xs text-muted-foreground">
          {m.tracking}
          <NumberBox
            label={m.tracking}
            value={shown.letterSpacing}
            onChange={(v) => set("letterSpacing", v)}
            onEnd={onEnd}
          />
        </div>
        <div className="flex flex-col gap-1 text-xs text-muted-foreground">
          {m.lineHeight}
          <NumberBox
            label={m.lineHeight}
            value={Math.round(shown.height * 100) / 100}
            onChange={(v) => set("height", v)}
            onEnd={onEnd}
          />
        </div>
      </div>
      <Row label={m.color}>
        <ColorField
          name={`${name} ${m.color}`}
          value={custom["color"]}
          onChange={(c) => set("color", c)}
          onEnd={onEnd}
        />
      </Row>
      <Row label={m.italic}>
        <Switch
          checked={shown.italic}
          onCheckedChange={(on) => {
            set("fontStyle", on ? "italic" : undefined);
            onEnd();
          }}
        />
      </Row>
      <Select
        label={m.line}
        value={shown.decoration ?? "none"}
        options={["none", "underline", "lineThrough", "overline"].map((d) => ({ value: d, label: m.decoration(d) }))}
        onValueChange={(d) => {
          set("decoration", d === "none" ? undefined : d);
          onEnd();
        }}
      />
    </fieldset>
  );
}
