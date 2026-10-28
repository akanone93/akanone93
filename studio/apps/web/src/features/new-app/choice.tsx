// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { cx } from "@plux/ui";
import { type ReactElement, type ReactNode, useId } from "react";

/**
 * One choice of a group: a radio button or a checkbox on a card (selected = tinted card with the accent border, U11),
 * named by its label and described by its hint. A native input keeps the keyboard and the accessibility tree.
 */
export function Choice({
  type,
  name,
  checked,
  disabled = false,
  onChange,
  label,
  hint,
  leading,
}: {
  readonly type: "radio" | "checkbox";
  readonly name?: string;
  readonly checked: boolean;
  readonly disabled?: boolean;
  readonly onChange: (checked: boolean) => void;
  readonly label: string;
  readonly hint?: string;
  /** Decoration before the text, such as a colour swatch. */
  readonly leading?: ReactNode;
}): ReactElement {
  const id = useId();
  return (
    <label
      className={cx(
        "flex items-start gap-3 rounded-lg border p-3 text-body",
        checked ? "border-accent bg-selection-surface" : "border-hairline bg-raised",
        disabled && "opacity-60",
      )}
    >
      <input
        type={type}
        {...(name !== undefined && { name })}
        checked={checked}
        disabled={disabled}
        aria-labelledby={`${id}-label`}
        {...(hint !== undefined && { "aria-describedby": `${id}-hint` })}
        onChange={(e) => onChange(e.target.checked)}
        className="mt-1"
      />
      {leading}
      <span className="flex flex-col gap-1">
        <span id={`${id}-label`} className="text-body text-primary">
          {label}
        </span>
        {hint !== undefined && (
          <span id={`${id}-hint`} className="text-caption text-secondary">
            {hint}
          </span>
        )}
      </span>
    </label>
  );
}
