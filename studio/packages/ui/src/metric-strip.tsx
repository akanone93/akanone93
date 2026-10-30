// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";

export interface Metric {
  readonly label: string;
  /** Value with its unit, e.g. "41 ms". */
  readonly value: string;
}

export interface MetricStripProps {
  readonly metrics: readonly Metric[];
}

/** One row at the foot of a panel: the label muted, the value in tabular figures. */
export function MetricStrip({ metrics }: MetricStripProps): ReactElement {
  return (
    <dl className="m-0 flex flex-wrap items-baseline gap-x-4 gap-y-1 border-t pt-2 text-xs">
      {metrics.map((m) => (
        <div key={m.label} className="flex items-baseline gap-1">
          <dt className="text-muted-foreground">{m.label}</dt>
          <dd className="m-0 font-medium tabular-nums text-foreground">{m.value}</dd>
        </div>
      ))}
    </dl>
  );
}
