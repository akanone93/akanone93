// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";

/** The series colours: Porcelain tokens, distinguishable in both themes and never the only cue (each line carries its label). */
const palette = ["var(--color-accent)", "var(--color-brand-mint)", "var(--color-brand-amber)"] as const;

export interface Series {
  readonly name: string;
  readonly values: readonly number[];
}

const W = 480;
const H = 160;
const PAD = 8;

const scale = (v: number, max: number, size: number) => (max <= 0 ? size : size - (v / max) * size);

/** Points of one polyline inside the plot area. */
export function linePoints(values: readonly number[], max: number): string {
  const step = values.length > 1 ? (W - 2 * PAD) / (values.length - 1) : 0;
  return values
    .map((v, i) => `${(PAD + i * step).toFixed(1)},${(PAD + scale(v, max, H - 2 * PAD)).toFixed(1)}`)
    .join(" ");
}

/** A line chart of daily values; `label` names it for assistive technology, and the page repeats the numbers in a table. */
export function LineChart({
  label,
  series,
}: {
  readonly label: string;
  readonly series: readonly Series[];
}): ReactElement {
  const max = Math.max(0, ...series.flatMap((s) => s.values));
  return (
    <figure className="m-0 flex flex-col gap-2">
      <svg role="img" aria-label={label} viewBox={`0 0 ${W} ${H}`} className="w-full max-w-2xl">
        <title>{label}</title>
        <line x1={PAD} x2={W - PAD} y1={H - PAD} y2={H - PAD} stroke="var(--color-hairline)" />
        {series.map((s, i) => (
          <polyline
            key={s.name}
            fill="none"
            stroke={palette[i % palette.length]}
            strokeWidth={2}
            points={linePoints(s.values, max)}
          />
        ))}
      </svg>
      <figcaption className="flex gap-4 text-caption text-secondary">
        {series.map((s, i) => (
          <span key={s.name} className="flex items-center gap-1">
            <svg width="12" height="4" aria-hidden="true">
              <rect width="12" height="4" fill={palette[i % palette.length]} />
            </svg>
            {s.name}
          </span>
        ))}
      </figcaption>
    </figure>
  );
}

export interface Bar {
  readonly key: string;
  readonly label: string;
  readonly value: number;
}

/** Horizontal bars scaled to the largest value; each row shows its number as text. */
export function BarList({
  label,
  bars,
  render,
}: {
  readonly label: string;
  readonly bars: readonly Bar[];
  readonly render?: (bar: Bar) => ReactElement;
}): ReactElement {
  const max = Math.max(1, ...bars.map((b) => b.value));
  return (
    <ul aria-label={label} className="m-0 flex list-none flex-col gap-2 p-0">
      {bars.map((b) => (
        <li key={b.key} className="flex flex-col gap-1">
          <span className="flex justify-between text-body">
            {render === undefined ? b.label : render(b)}
            <span className="font-mono text-data">{b.value}</span>
          </span>
          <svg viewBox="0 0 100 4" preserveAspectRatio="none" className="h-1 w-full" aria-hidden="true">
            <rect width="100" height="4" fill="var(--color-hairline)" />
            <rect width={(b.value / max) * 100} height="4" fill="var(--color-accent)" />
          </svg>
        </li>
      ))}
    </ul>
  );
}

/** A tiny trend line without axes. */
export function Sparkline({ values, label }: { readonly values: readonly number[]; readonly label: string }) {
  const max = Math.max(0, ...values);
  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${W} ${H}`} className="h-6 w-24">
      <title>{label}</title>
      <polyline fill="none" stroke="var(--color-accent)" strokeWidth={8} points={linePoints(values, max)} />
    </svg>
  );
}
