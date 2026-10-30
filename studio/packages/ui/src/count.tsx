// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { Badge } from "./components/badge";

export interface CountProps {
  readonly value: number;
  /** Warning counts stand out from neutral ones. */
  readonly tone?: "neutral" | "warning";
  /** Accessible name, e.g. "3 warnings", from the message catalogue. */
  readonly label?: string;
}

/** A count as shadcn's Badge, for tabs, layer rows and notifications (ADR-0103). */
export function Count({ value, tone = "neutral", label }: CountProps): ReactElement {
  return (
    <Badge
      variant={tone === "warning" ? "destructive" : "secondary"}
      className="h-5 min-w-5 rounded-full px-1 tabular-nums"
      {...(label === undefined ? {} : { role: "img", "aria-label": label })}
    >
      {value}
    </Badge>
  );
}
