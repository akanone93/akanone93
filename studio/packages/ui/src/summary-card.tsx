// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";

export interface SummaryCell {
  readonly value: string;
  readonly caption: string;
}

export interface SummaryCardProps {
  readonly cells: readonly [SummaryCell, SummaryCell, SummaryCell];
}

/** Three equal cells, mono value over a sans caption, for panel footers (U10). */
export function SummaryCard({ cells }: SummaryCardProps): ReactElement {
  return (
    <dl className="m-0 grid grid-cols-3 rounded-lg bg-card px-(--density-card-padding-x) py-(--density-card-padding-y) text-center">
      {cells.map((c) => (
        <div key={c.caption} className="flex flex-col-reverse">
          <dt className="text-caption text-secondary">{c.caption}</dt>
          <dd className="m-0 font-mono text-data text-primary">{c.value}</dd>
        </div>
      ))}
    </dl>
  );
}
