// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Input } from "@plux/ui";
import type { ReactElement } from "react";
import "./colour-field.css";

/** `#RRGGBB` or `#RRGGBBAA`, the colour forms a theme token and a colour prop take. */
const HEX = /^#([0-9a-fA-F]{6})([0-9a-fA-F]{2})?$/;

/** The picker's value for a colour: its `#rrggbb` part, or black for an alias or text that is not a colour yet. */
export const pickerValue = (value: string): string => {
  const m = HEX.exec(value.trim());
  return m === null ? "#000000" : `#${(m[1] ?? "000000").toLowerCase()}`;
};

/** A picked `#rrggbb` applied to the value: the alpha the value had is kept, and the digits are upper case. */
export const withPicked = (value: string, picked: string): string => {
  const alpha = HEX.exec(value.trim())?.[2] ?? "";
  return `${picked.toUpperCase()}${alpha.toUpperCase()}`;
};

/**
 * A colour value: the browser's colour picker as a swatch beside a text field. The text takes a hex code, with alpha
 * (`#00000066`), or an alias (`{color.primary}`), which a picker cannot; picking keeps the alpha.
 */
export function ColourField(props: {
  readonly value: string;
  readonly onChange: (value: string) => void;
  /** The accessible name of the swatch that opens the picker. */
  readonly pickLabel: string;
  /** The accessible name of the text field, when no visible label names it. */
  readonly textLabel?: string;
  readonly id?: string;
  readonly disabled?: boolean;
  readonly onBlur?: () => void;
}): ReactElement {
  return (
    <span className="colour-field">
      <input
        type="color"
        className="colour-swatch"
        aria-label={props.pickLabel}
        disabled={props.disabled}
        value={pickerValue(props.value)}
        onChange={(e) => props.onChange(withPicked(props.value, e.target.value))}
        onBlur={props.onBlur}
      />
      <Input
        id={props.id}
        aria-label={props.textLabel}
        value={props.value}
        disabled={props.disabled}
        autoComplete="off"
        spellCheck={false}
        dir="ltr"
        onChange={(e) => props.onChange(e.target.value)}
        onBlur={props.onBlur}
      />
    </span>
  );
}

/** A colour shown in a table or list: a swatch and its code; an alias or other text shows as text alone. */
export function ColourChip({ value }: { readonly value: string }): ReactElement {
  const colour = HEX.test(value.trim());
  return (
    <span className="colour-chip">
      {colour && <span aria-hidden="true" className="colour-chip-swatch" style={{ backgroundColor: value.trim() }} />}
      <span className="font-mono">{value}</span>
    </span>
  );
}
