// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Select as BaseSelect } from "@base-ui/react/select";
import type { ReactElement } from "react";
import { cx, focusRing } from "./cx.ts";
import { Icon } from "./icon.tsx";

export interface SelectOption {
  readonly value: string;
  readonly label: string;
}

export interface SelectProps {
  readonly options: readonly SelectOption[];
  readonly value?: string;
  readonly defaultValue?: string;
  readonly onValueChange?: (value: string) => void;
  /** Accessible name when no visible Field label wraps the select. */
  readonly label?: string;
  readonly placeholder?: string;
  readonly disabled?: boolean;
}

/** Single-choice select (U11 "Input, select"). Arrow keys, type-ahead and Escape come from Base UI. */
export function Select({
  options,
  value,
  defaultValue,
  onValueChange,
  label,
  placeholder,
  disabled,
}: SelectProps): ReactElement {
  const items = options.map((o) => ({ value: o.value, label: o.label }));
  return (
    <BaseSelect.Root
      items={items}
      {...(value !== undefined && { value })}
      {...(defaultValue !== undefined && { defaultValue })}
      {...(disabled !== undefined && { disabled })}
      onValueChange={(v) => {
        if (typeof v === "string") {
          onValueChange?.(v);
        }
      }}
    >
      <BaseSelect.Trigger
        {...(label !== undefined && { "aria-label": label })}
        className={cx(
          "flex h-(--density-field) w-full items-center justify-between gap-2 rounded-sm bg-raised px-3 text-body text-primary",
          "border border-hairline hover:border-strong disabled:opacity-50",
          focusRing,
        )}
      >
        <BaseSelect.Value {...(placeholder !== undefined && { placeholder })} />
        <BaseSelect.Icon>
          <Icon name="caret-down" />
        </BaseSelect.Icon>
      </BaseSelect.Trigger>
      <BaseSelect.Portal>
        <BaseSelect.Positioner sideOffset={4} className="z-20">
          <BaseSelect.Popup className="rounded-xl bg-glass p-1 shadow-e2">
            <BaseSelect.List>
              {items.map((o) => (
                <BaseSelect.Item
                  key={o.value}
                  value={o.value}
                  className={cx(
                    "flex h-(--density-field) cursor-default items-center gap-2 rounded-md px-3 text-body text-primary",
                    "data-highlighted:bg-hover",
                    focusRing,
                  )}
                >
                  <BaseSelect.ItemText>{o.label}</BaseSelect.ItemText>
                  <BaseSelect.ItemIndicator className="ms-auto">
                    <Icon name="check" />
                  </BaseSelect.ItemIndicator>
                </BaseSelect.Item>
              ))}
            </BaseSelect.List>
          </BaseSelect.Popup>
        </BaseSelect.Positioner>
      </BaseSelect.Portal>
    </BaseSelect.Root>
  );
}
