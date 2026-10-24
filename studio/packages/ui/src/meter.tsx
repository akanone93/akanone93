// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";

export interface MeterProps {
  readonly value: number;
  readonly max: number;
  /** What is measured, e.g. "Bundle size", from the message catalogue. */
  readonly label: string;
  /** Human readable reading with its unit, e.g. "412 of 512 KB". */
  readonly valueText: string;
}

/** Budget meter with a success fill that grows in on load (U10); the number is always in text. */
export function Meter({ value, max, label, valueText }: MeterProps): ReactElement {
  const ratio = max <= 0 ? 0 : Math.min(1, Math.max(0, value / max));
  return (
    <div className="inline-flex items-center gap-2">
      {/* biome-ignore lint/a11y/useSemanticElements: the track needs a styled fill, which <meter> does not allow */}
      <div
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={max}
        aria-valuenow={value}
        aria-valuetext={valueText}
        className="h-1.5 w-16 overflow-hidden rounded-full bg-hover"
      >
        <div
          className="h-full origin-left rounded-full bg-success transition-transform duration-enter ease-standard rtl:origin-right"
          style={{ transform: `scaleX(${ratio})` }}
        />
      </div>
      <span className="font-mono text-tag text-secondary">{valueText}</span>
    </div>
  );
}
