// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { cx } from "./cx.ts";

export interface CountProps {
  readonly value: number;
  /** Warning counts sit on the warning surface. */
  readonly tone?: "neutral" | "warning";
  /** Accessible name, e.g. "3 warnings", from the message catalogue. */
  readonly label?: string;
}

/** Round mono badge for tabs, layer rows and notifications (U10). */
export function Count({ value, tone = "neutral", label }: CountProps): ReactElement {
  return (
    <span
      {...(label === undefined ? {} : { role: "img", "aria-label": label })}
      className={cx(
        "inline-flex min-w-4 items-center justify-center rounded-full px-1 font-mono text-tag",
        tone === "warning" ? "bg-warning-surface text-warning" : "bg-hover text-secondary",
      )}
    >
      {value}
    </span>
  );
}
