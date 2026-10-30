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

/** Three equal cells, value over caption, for panel footers; the surface is shadcn's card. */
export function SummaryCard({ cells }: SummaryCardProps): ReactElement {
  return (
    <dl className="m-0 grid grid-cols-3 rounded-xl bg-card py-3 text-center text-card-foreground ring-1 ring-foreground/10">
      {cells.map((c) => (
        <div key={c.caption} className="flex flex-col-reverse gap-0.5">
          <dt className="text-xs text-muted-foreground">{c.caption}</dt>
          <dd className="m-0 text-sm font-medium tabular-nums">{c.value}</dd>
        </div>
      ))}
    </dl>
  );
}
