// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import {
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Select as ShadcnSelect,
} from "./components/select";

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

/**
 * shadcn's Select (ADR-0103). Arrow keys, type-ahead and Escape come from Base UI. It is not modal, so the page keeps
 * its scroll bar and scroll position while the list is open.
 */
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
    <ShadcnSelect
      items={items}
      modal={false}
      {...(value !== undefined && { value })}
      {...(defaultValue !== undefined && { defaultValue })}
      {...(disabled !== undefined && { disabled })}
      onValueChange={(v) => {
        if (typeof v === "string") {
          onValueChange?.(v);
        }
      }}
    >
      <SelectTrigger {...(label !== undefined && { "aria-label": label })} className="w-full">
        <SelectValue {...(placeholder !== undefined && { placeholder })} />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {items.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </ShadcnSelect>
  );
}
