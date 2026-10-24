// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";

export interface SparklineProps {
  readonly values: readonly number[];
  /** Label and latest value in text, e.g. "Frame time, 7 ms" (U10: no chart without a text value). */
  readonly label: string;
}

const width = 84;
const height = 18;

function points(values: readonly number[]): string {
  const min = Math.min(...values);
  const range = Math.max(...values) - min || 1;
  const step = values.length > 1 ? width / (values.length - 1) : 0;
  return values
    .map((v, i) => `${(i * step).toFixed(2)},${(height - ((v - min) / range) * height).toFixed(2)}`)
    .join(" ");
}

/** Small violet line of recent values; SVG elements only, no markup injection (U10). */
export function Sparkline({ values, label }: SparklineProps): ReactElement {
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${width} ${height}`}
      className="h-5 w-16 overflow-visible rtl:-scale-x-100"
    >
      <title>{label}</title>
      <polyline
        points={values.length === 0 ? "" : points(values)}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
        className="text-accent"
      />
    </svg>
  );
}
