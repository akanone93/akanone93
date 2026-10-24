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

/** One row at the foot of a panel: label in sans, value in mono (U10). */
export function MetricStrip({ metrics }: MetricStripProps): ReactElement {
  return (
    <dl className="m-0 flex flex-wrap items-baseline gap-x-4 gap-y-1 border-t border-hairline pt-2 text-caption">
      {metrics.map((m) => (
        <div key={m.label} className="flex items-baseline gap-1">
          <dt className="text-secondary">{m.label}</dt>
          <dd className="m-0 font-mono text-data text-primary">{m.value}</dd>
        </div>
      ))}
    </dl>
  );
}
