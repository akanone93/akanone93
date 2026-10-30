// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { Badge } from "./components/badge";
import { cx } from "./cx.ts";
import { Icon } from "./icon.tsx";
import type { IconName } from "./icons.ts";

export type StatusKind = "success" | "warning" | "danger" | "neutral";

/** shadcn's Badge variant for each status; success and warning colour only their icon. */
const kindVariant = { success: "outline", warning: "outline", danger: "destructive", neutral: "secondary" } as const;

const kindIconClass: Readonly<Record<StatusKind, string>> = {
  success: "text-success",
  warning: "text-warning",
  danger: "",
  neutral: "text-muted-foreground",
};

const kindIcon: Readonly<Record<StatusKind, IconName>> = {
  success: "check-circle",
  warning: "warning",
  danger: "x-circle",
  neutral: "info",
};

export interface StatusPillProps {
  readonly status: StatusKind;
  /** The status word from the message catalogue; every status has an icon and a word. */
  readonly label: string;
  /** Optional number or value with its unit, shown after the word. */
  readonly value?: string;
}

/** A status as shadcn's Badge: an icon, the word and an optional value (ADR-0103). */
export function StatusPill({ status, label, value }: StatusPillProps): ReactElement {
  return (
    <Badge variant={kindVariant[status]} data-status={status}>
      <Icon name={kindIcon[status]} size="pill" className={cx(kindIconClass[status])} />
      <span>{label}</span>
      {value === undefined ? null : <span className="tabular-nums text-muted-foreground">{value}</span>}
    </Badge>
  );
}
