// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName, Select } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { Switch } from "@plux/ui/components/switch";
import type { ReactElement, ReactNode } from "react";
import {
  type Align3,
  type Axis,
  type LayoutBox,
  type LayoutItem,
  layoutContextOf,
  overridesOf,
  PINS,
  type Pin,
  padOf,
  type Sizing,
  sizingOf,
} from "./layout-model.ts";
import { type LayoutMessages, layoutMessages } from "./layout-panel.messages.ts";
import { type Breakpoint, valueAt } from "./properties-model.ts";
import { TokenNumber } from "./state-editors.tsx";
import "./layout-panel.css";

/**
 * The layout editors at the top of the inspector's properties (PC-22, WGT-010): a row, column, flex or wrap as an auto
 * layout (direction, alignment pad, distribution, gap, size, stretch); a widget in a row or column sized to hug, fill
 * or a fixed size; a widget in a stack pinned to edges; and the size classes the widget overrides. Props edit at the
 * size class picked above; wrapping in `Expanded`, `SizedBox` or `Positioned` is the same on every size class.
 */

type Obj = Record<string, unknown>;
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

export interface LayoutPanelProps {
  readonly node: Obj;
  readonly root: unknown;
  readonly id: string;
  readonly bp: Breakpoint;
  readonly canEdit: boolean;
  /** Sets props of the widget at the size class being edited, as one undo step. */
  readonly applyProps: (values: Readonly<Obj>) => void;
  /** Runs a structural recipe over the page (wrap, unwrap, convert), as one undo step. */
  readonly structure: (recipe: (root: unknown) => void) => void;
  /** Recipes made by the model, given fresh ids for wrappers. */
  readonly recipes: LayoutRecipes;
  readonly layoutNames: readonly string[];
  readonly onBp: (bp: Breakpoint) => void;
  readonly onClearOverrides: (bp: "medium" | "expanded") => void;
}

export interface LayoutRecipes {
  readonly flip: (box: LayoutBox) => (root: unknown) => void;
  readonly sizing: (axis: Axis, sizing: Sizing, size: number) => (root: unknown) => void;
  readonly pin: (pin: Pin, value: number | undefined) => (root: unknown) => void;
  readonly flex: (wrapperId: string, flex: number) => (root: unknown) => void;
}

function Segments<T extends string>(p: {
  readonly label: string;
  readonly value: T | undefined;
  readonly options: readonly { readonly value: T; readonly label: string; readonly icon?: IconName }[];
  readonly disabled: boolean;
  readonly onChange: (v: T) => void;
}) {
  return (
    <div className="lay-segments" role="radiogroup" aria-label={p.label}>
      {p.options.map((o) => (
        // biome-ignore lint/a11y/useSemanticElements: a segmented control's option, styled as a pill
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={p.value === o.value}
          className="lay-segment"
          disabled={p.disabled}
          onClick={() => p.onChange(o.value)}
        >
          {o.icon !== undefined && <Icon name={o.icon} size="pill" />}
          <span>{o.label}</span>
        </button>
      ))}
    </div>
  );
}

function Row(p: { readonly label: string; readonly children: ReactNode }) {
  return (
    <div className="lay-row">
      <span className="lay-label">{p.label}</span>
      <div className="lay-control">{p.children}</div>
    </div>
  );
}

function NumberField(p: {
  readonly name: string;
  readonly label: string;
  readonly value: unknown;
  readonly disabled: boolean;
  readonly min?: number;
  readonly onChange: (v: unknown) => void;
}) {
  return (
    <TokenNumber name={p.name} value={p.value} readOnly={p.disabled} onChange={p.onChange} onEnd={() => undefined}>
      <Input
        type="number"
        aria-label={p.label}
        min={p.min ?? 0}
        className="lay-number"
        value={num(p.value) ?? ""}
        readOnly={p.disabled}
        onChange={(e) => p.onChange(e.target.value === "" ? undefined : Math.max(p.min ?? 0, Number(e.target.value)))}
      />
    </TokenNumber>
  );
}

const ALIGNS: readonly Align3[] = ["start", "center", "end"];
const DISTRIBUTIONS = ["packed", "spaceBetween", "spaceAround", "spaceEvenly"] as const;

/** The 3×3 pad: each point a main and cross alignment, laid out as the box's axis runs. */
function AlignPad(p: {
  readonly axis: Axis;
  readonly props: Readonly<Obj>;
  readonly m: LayoutMessages;
  readonly disabled: boolean;
  readonly onPick: (main: Align3, cross: Align3) => void;
}) {
  const { main, cross } = padOf(p.props);
  const distributed = !["start", "center", "end", undefined].includes(p.props["mainAxisAlignment"] as string);
  return (
    <fieldset className="lay-pad" aria-label={p.m.alignPad} data-axis={p.axis}>
      {ALIGNS.flatMap((v) =>
        ALIGNS.map((h) => {
          const [m, c] = p.axis === "horizontal" ? [h, v] : [v, h];
          const on = (distributed || main === m) && cross === c;
          return (
            <button
              key={`${v}-${h}`}
              type="button"
              className="lay-pad-cell"
              aria-label={p.m.alignAt(v, h)}
              aria-pressed={on}
              disabled={p.disabled}
              onClick={() => p.onPick(m, c)}
            >
              <span className="lay-pad-dot" aria-hidden="true" />
            </button>
          );
        }),
      )}
    </fieldset>
  );
}

function distributionOf(props: Readonly<Obj>): (typeof DISTRIBUTIONS)[number] {
  const v = props["mainAxisAlignment"];
  return v === "spaceBetween" || v === "spaceAround" || v === "spaceEvenly" ? v : "packed";
}

function AutoLayout(p: {
  readonly box: LayoutBox;
  readonly props: Readonly<Obj>;
  readonly m: LayoutMessages;
  readonly disabled: boolean;
  readonly apply: (values: Readonly<Obj>) => void;
  readonly flip: () => void;
}) {
  const { m, box, props } = p;
  const flex = box.kind === "flex";
  return (
    <section className="lay-card" aria-label={m.autoLayout}>
      <h3 className="lay-title">
        <Icon name={box.axis === "horizontal" ? "columns" : "rows"} size="pill" />
        {m.autoLayout}
        <span className="lay-type">{box.type}</span>
      </h3>
      <Row label={m.direction}>
        <Segments
          label={m.direction}
          value={box.axis}
          disabled={p.disabled}
          options={[
            { value: "horizontal", label: m.row, icon: "columns" },
            { value: "vertical", label: m.column, icon: "rows" },
          ]}
          onChange={(axis) => axis !== box.axis && p.flip()}
        />
      </Row>
      {flex && (
        <div className="lay-pad-row">
          <AlignPad
            axis={box.axis}
            props={props}
            m={m}
            disabled={p.disabled}
            onPick={(main, cross) =>
              p.apply({
                mainAxisAlignment:
                  distributionOf(props) === "packed"
                    ? main === "start"
                      ? undefined
                      : main
                    : props["mainAxisAlignment"],
                crossAxisAlignment: cross === "center" ? undefined : cross,
              })
            }
          />
          <div className="lay-pad-side">
            <div className="lay-stack-label">
              <span>{m.distribution}</span>
              <Select
                label={m.distribution}
                options={DISTRIBUTIONS.map((d) => ({ value: d, label: m.distributionOf(d) }))}
                value={distributionOf(props)}
                disabled={p.disabled}
                onValueChange={(v) => p.apply({ mainAxisAlignment: v === "packed" ? undefined : v })}
              />
            </div>
            <div className="lay-switch">
              <span>{m.stretch}</span>
              <Switch
                aria-label={m.stretch}
                checked={props["crossAxisAlignment"] === "stretch"}
                disabled={p.disabled}
                onCheckedChange={(on) => p.apply({ crossAxisAlignment: on ? "stretch" : undefined })}
              />
            </div>
          </div>
        </div>
      )}
      <Row label={m.gap}>
        <NumberField
          name="spacing"
          label={m.gap}
          value={props["spacing"]}
          disabled={p.disabled}
          onChange={(v) => p.apply({ spacing: v })}
        />
      </Row>
      {box.kind === "wrap" && (
        <Row label={m.runGap}>
          <NumberField
            name="runSpacing"
            label={m.runGap}
            value={props["runSpacing"]}
            disabled={p.disabled}
            onChange={(v) => p.apply({ runSpacing: v })}
          />
        </Row>
      )}
      {flex && (
        <Row label={m.size}>
          <Segments
            label={m.size}
            value={props["mainAxisSize"] === "min" ? "min" : "max"}
            disabled={p.disabled}
            options={[
              { value: "min", label: m.hug },
              { value: "max", label: m.fill },
            ]}
            onChange={(v) => p.apply({ mainAxisSize: v === "min" ? "min" : undefined })}
          />
        </Row>
      )}
    </section>
  );
}

function AxisSizing(p: {
  readonly axis: Axis;
  readonly node: Obj;
  readonly item: LayoutItem;
  readonly m: LayoutMessages;
  readonly disabled: boolean;
  readonly recipes: LayoutRecipes;
  readonly structure: (recipe: (root: unknown) => void) => void;
  readonly apply: (values: Readonly<Obj>) => void;
}) {
  const { m, axis, item } = p;
  const sizing = sizingOf(p.node, item, axis);
  const prop = axis === "horizontal" ? "width" : "height";
  const along = item.kind === "flex" && item.axis === axis;
  const props = (p.node["props"] ?? {}) as Obj;
  const options = [
    { value: "hug" as const, label: m.hug },
    ...(along ? [{ value: "fill" as const, label: m.fill }] : []),
    { value: "fixed" as const, label: m.fixed },
  ];
  return (
    <div className="lay-sizing">
      <span className="lay-label">{axis === "horizontal" ? m.width : m.height}</span>
      <Segments
        label={axis === "horizontal" ? m.width : m.height}
        value={sizing}
        disabled={p.disabled}
        options={options}
        onChange={(s) => p.structure(p.recipes.sizing(axis, s, num(props[prop]) ?? 120))}
      />
      {sizing === "fixed" && (
        <NumberField
          name={prop}
          label={axis === "horizontal" ? m.width : m.height}
          value={props[prop]}
          disabled={p.disabled}
          onChange={(v) => p.apply({ [prop]: v })}
        />
      )}
      {sizing === "fill" && item.kind === "flex" && item.wrapper !== undefined && (
        <div className="lay-inline">
          <span>{m.flex}</span>
          <Input
            type="number"
            min={1}
            aria-label={m.flex}
            className="lay-number lay-flex"
            value={item.wrapper.flex}
            readOnly={p.disabled}
            onChange={(e) => {
              const wrapper = item.wrapper;
              if (wrapper !== undefined && e.target.value !== "") {
                p.structure(p.recipes.flex(wrapper.id, Math.max(1, Math.round(Number(e.target.value)))));
              }
            }}
          />
        </div>
      )}
    </div>
  );
}

function StackPins(p: {
  readonly item: Extract<LayoutItem, { kind: "stack" }>;
  readonly m: LayoutMessages;
  readonly disabled: boolean;
  readonly recipes: LayoutRecipes;
  readonly structure: (recipe: (root: unknown) => void) => void;
}) {
  const { m } = p;
  const pins = (p.item.positioned?.props ?? {}) as Obj;
  const toggle = (pin: Pin) => p.structure(p.recipes.pin(pin, pins[pin] === undefined ? 0 : undefined));
  const set = PINS.filter((pin) => pins[pin] !== undefined);
  return (
    <section className="lay-card" aria-label={m.inStack}>
      <h3 className="lay-title">
        <Icon name="stack" size="pill" />
        {m.inStack}
      </h3>
      <div className="lay-pins">
        <div className="lay-pin-box">
          {(["top", "left", "right", "bottom"] as const).map((edge) => (
            <button
              key={edge}
              type="button"
              className="lay-pin"
              data-edge={edge}
              aria-label={m.pin(edge)}
              aria-pressed={pins[edge] !== undefined}
              disabled={p.disabled}
              onClick={() => toggle(edge)}
            />
          ))}
          <span className="lay-pin-core" aria-hidden="true" />
        </div>
        <div className="lay-pin-values">
          {set.length === 0 && <p className="lay-note">{m.unpinned}</p>}
          {set.map((pin) => (
            <div key={pin} className="lay-inline">
              <span>{m.pinValue(pin)}</span>
              <Input
                type="number"
                aria-label={m.pinValue(pin)}
                className="lay-number"
                value={num(pins[pin]) ?? ""}
                readOnly={p.disabled}
                onChange={(e) =>
                  p.structure(p.recipes.pin(pin, e.target.value === "" ? undefined : Number(e.target.value)))
                }
              />
            </div>
          ))}
          <div className="lay-pin-sizes">
            {(["width", "height"] as const).map((pin) => (
              <button
                key={pin}
                type="button"
                className="lay-chip"
                aria-pressed={pins[pin] !== undefined}
                disabled={p.disabled}
                onClick={() => toggle(pin)}
              >
                {m.pin(pin)}
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

function Overrides(p: {
  readonly node: Obj;
  readonly bp: Breakpoint;
  readonly m: LayoutMessages;
  readonly disabled: boolean;
  readonly onBp: (bp: Breakpoint) => void;
  readonly onClear: (bp: "medium" | "expanded") => void;
}) {
  const { m } = p;
  const o = overridesOf(p.node);
  const classes = (["medium", "expanded"] as const).filter((bp) => o[bp].length > 0);
  if (classes.length === 0 && p.bp === "compact") {
    return null;
  }
  return (
    <section className="lay-overrides" aria-label={m.overrides}>
      {p.bp !== "compact" && <p className="lay-note">{m.editingClass(p.bp)}</p>}
      {classes.length > 0 && (
        <div className="lay-override-list">
          <span className="lay-label">{m.overrides}</span>
          {classes.map((bp) => (
            <span key={bp} className="lay-override" data-current={p.bp === bp || undefined} title={o[bp].join(", ")}>
              <button
                type="button"
                className="lay-override-name"
                aria-label={m.editOverrides(bp)}
                onClick={() => p.onBp(bp)}
              >
                {m.overrideCount(bp, o[bp].length)}
              </button>
              {!p.disabled && (
                <button
                  type="button"
                  className="lay-override-clear"
                  aria-label={m.clearOverrides(bp)}
                  onClick={() => p.onClear(bp)}
                >
                  <Icon name="x" size="pill" />
                </button>
              )}
            </span>
          ))}
        </div>
      )}
    </section>
  );
}

/** The layout editors of one selected widget; nothing when it neither lays out children nor sits in a layout. */
export function LayoutPanel(p: LayoutPanelProps): ReactElement | null {
  const m = layoutMessages(useLingui().i18n);
  const ctx = layoutContextOf(p.root, p.id);
  const disabled = !p.canEdit;
  const props: Obj = {};
  for (const name of ["mainAxisAlignment", "crossAxisAlignment", "mainAxisSize", "spacing", "runSpacing"]) {
    const v = valueAt(p.node, name, p.bp).value;
    if (v !== undefined) {
      props[name] = v;
    }
  }
  const box = ctx.box;
  const item = ctx.item;
  const set = p.layoutNames.filter((n) => valueAt(p.node, n, p.bp).value !== undefined);
  return (
    <div className="lay-panel">
      <Overrides node={p.node} bp={p.bp} m={m} disabled={disabled} onBp={p.onBp} onClear={p.onClearOverrides} />
      {box !== undefined && (
        <AutoLayout
          box={box}
          props={props}
          m={m}
          disabled={disabled}
          apply={p.applyProps}
          flip={() => p.structure(p.recipes.flip(box))}
        />
      )}
      {item?.kind === "flex" && (
        <section className="lay-card" aria-label={m.inRowOrColumn(item.axis)}>
          <h3 className="lay-title">
            <Icon name={item.axis === "horizontal" ? "columns" : "rows"} size="pill" />
            {m.inRowOrColumn(item.axis)}
          </h3>
          {(["horizontal", "vertical"] as const).map((axis) => (
            <AxisSizing
              key={axis}
              axis={axis}
              node={p.node}
              item={item}
              m={m}
              disabled={disabled}
              recipes={p.recipes}
              structure={p.structure}
              apply={p.applyProps}
            />
          ))}
        </section>
      )}
      {item?.kind === "stack" && (
        <StackPins item={item} m={m} disabled={disabled} recipes={p.recipes} structure={p.structure} />
      )}
      {set.length > 0 && !disabled && (
        <button
          type="button"
          className="lay-reset"
          onClick={() => p.applyProps(Object.fromEntries(set.map((n) => [n, undefined])))}
        >
          <Icon name="arrow-counter-clockwise" size="pill" />
          {m.resetLayout}
        </button>
      )}
    </div>
  );
}

/** A `BoxConstraints`: the least and most width and height, side by side; an empty most is unbounded. */
export function ConstraintsEditor(p: {
  readonly name: string;
  readonly value: unknown;
  readonly readOnly: boolean;
  readonly onChange: (value: unknown) => void;
  readonly onEnd: () => void;
}): ReactElement {
  const m = layoutMessages(useLingui().i18n);
  const o = (typeof p.value === "object" && p.value !== null ? p.value : {}) as Obj;
  const put = (k: string, v: number | undefined) => {
    const next: Obj = { ...o, [k]: v };
    if (v === undefined) {
      delete next[k];
    }
    p.onChange(Object.keys(next).length === 0 ? undefined : next);
  };
  const field = (k: string, label: string, placeholder: string) => (
    <Input
      type="number"
      min={0}
      aria-label={label}
      placeholder={placeholder}
      className="lay-number"
      value={num(o[k]) ?? ""}
      readOnly={p.readOnly}
      onChange={(e) => put(k, e.target.value === "" ? undefined : Math.max(0, Number(e.target.value)))}
      onBlur={p.onEnd}
    />
  );
  return (
    <div className="lay-constraints">
      <span />
      <span className="lay-label">{m.least}</span>
      <span className="lay-label">{m.most}</span>
      <span className="lay-label">{m.width}</span>
      {field("minWidth", m.minOf(m.width), "0")}
      {field("maxWidth", m.maxOf(m.width), "∞")}
      <span className="lay-label">{m.height}</span>
      {field("minHeight", m.minOf(m.height), "0")}
      {field("maxHeight", m.maxOf(m.height), "∞")}
    </div>
  );
}
