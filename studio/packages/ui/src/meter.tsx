// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { cx } from "./cx.ts";

export interface MeterProps {
  readonly value: number;
  readonly max: number;
  /** What is measured, e.g. "Bundle size", from the message catalogue. */
  readonly label: string;
  /** Human readable reading with its unit, e.g. "412 of 512 KB". */
  readonly valueText: string;
  /** `bar` fills its container and leaves the reading to the caller, who shows it beside the label. */
  readonly layout?: "inline" | "bar";
}

/** A budget meter in shadcn's progress style; the reading is always available as text. */
export function Meter({ value, max, label, valueText, layout = "inline" }: MeterProps): ReactElement {
  const ratio = max <= 0 ? 0 : Math.min(1, Math.max(0, value / max));
  const track = (
    // biome-ignore lint/a11y/useSemanticElements: the track needs a styled fill, which <meter> does not allow
    <div
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={value}
      aria-valuetext={valueText}
      className={cx("h-1 overflow-hidden rounded-full bg-muted", layout === "bar" ? "w-full" : "w-16")}
    >
      <div
        className={cx(
          "h-full origin-left rounded-full transition-transform duration-enter ease-standard rtl:origin-right",
          ratio >= 0.8 ? "bg-destructive" : "bg-primary",
        )}
        style={{ transform: `scaleX(${ratio})` }}
      />
    </div>
  );
  if (layout === "bar") {
    return track;
  }
  return (
    <div className="inline-flex items-center gap-2">
      {track}
      <span className="text-xs tabular-nums text-muted-foreground">{valueText}</span>
    </div>
  );
}
