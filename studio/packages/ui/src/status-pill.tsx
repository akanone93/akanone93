// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { cx } from "./cx.ts";
import { Icon } from "./icon.tsx";
import type { IconName } from "./icons.gen.ts";

export type StatusKind = "success" | "warning" | "danger" | "neutral";

const kindClass: Readonly<Record<StatusKind, string>> = {
  success: "bg-success-surface text-success",
  warning: "bg-warning-surface text-warning",
  danger: "bg-danger-surface text-danger",
  neutral: "bg-raised text-secondary",
};

const kindIcon: Readonly<Record<StatusKind, IconName>> = {
  success: "check-circle",
  warning: "warning",
  danger: "x-circle",
  neutral: "info",
};

export interface StatusPillProps {
  readonly status: StatusKind;
  /** The status word from the message catalogue; every status has an icon and a word (U10). */
  readonly label: string;
  /** Optional number or value with its unit, shown after the word. */
  readonly value?: string;
}

/** Health under a screen, release health, test results (U10). */
export function StatusPill({ status, label, value }: StatusPillProps): ReactElement {
  return (
    <span
      className={cx(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-mono text-tag shadow-e1",
        kindClass[status],
      )}
    >
      <Icon name={kindIcon[status]} size="pill" />
      <span>{label}</span>
      {value === undefined ? null : <span>{value}</span>}
    </span>
  );
}
