// biome-ignore-all lint/a11y/useSemanticElements: virtualised rows are absolutely positioned, so native table elements cannot lay them out; ARIA table roles carry the semantics
// biome-ignore-all lint/a11y/noNoninteractiveTabindex: the scrolling body must be keyboard-reachable (axe scrollable-region-focusable)
// biome-ignore-all lint/a11y/useFocusableInteractive: rows are read-only here; row actions arrive with the screens that use the table
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type ColumnDef, type RowData, tableFeatures, useTable } from "@tanstack/react-table";
import { useVirtualizer } from "@tanstack/react-virtual";
import { type ReactElement, useRef } from "react";

const features = tableFeatures({});

export interface VirtualTableProps<TData extends RowData> {
  readonly data: readonly TData[];
  readonly columns: ColumnDef<typeof features, TData>[];
  /** Accessible name of the table, from the message catalogue. */
  readonly label: string;
  /** Row height in px; must equal the `density.tableRow` token in effect (U11). */
  readonly rowHeight: number;
  /** How many rows the scrolling viewport shows. */
  readonly visibleRows: number;
  /** Rows rendered beyond the viewport on each side. */
  readonly overscan?: number;
}

/** Table of any size: only the visible rows are in the DOM (STU-120). Mono data, hairline rows, 30 px compact (U11). */
export function VirtualTable<TData extends RowData>({
  data,
  columns,
  label,
  rowHeight,
  visibleRows,
  overscan = 4,
}: VirtualTableProps<TData>): ReactElement {
  const table = useTable({ features, columns, data: data as TData[] });
  const scrollRef = useRef<HTMLDivElement>(null);
  const rows = table.getRowModel().rows;
  const viewport = rowHeight * visibleRows;
  const virtualizer = useVirtualizer({
    // The viewport height is fixed by props, so it is reported once instead of measured.
    observeElementRect: (_instance, report) => {
      report({ width: 0, height: viewport });
      return () => undefined;
    },
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan,
    initialRect: { width: 0, height: viewport },
  });
  const template = { gridTemplateColumns: `repeat(${columns.length}, minmax(0, 1fr))` };

  return (
    <div role="table" aria-label={label} aria-rowcount={rows.length + 1} className="rounded-lg bg-card text-body">
      <div role="rowgroup">
        {table.getHeaderGroups().map((group) => (
          <div
            key={group.id}
            role="row"
            aria-rowindex={1}
            className="grid border-b border-hairline"
            style={{ ...template, height: rowHeight }}
          >
            {group.headers.map((header) => (
              <div key={header.id} role="columnheader" className="flex items-center px-2 text-label text-secondary">
                {header.isPlaceholder ? null : <table.FlexRender header={header} />}
              </div>
            ))}
          </div>
        ))}
      </div>
      <div
        ref={scrollRef}
        role="rowgroup"
        tabIndex={0}
        className="overflow-y-auto focus-visible:outline-2 focus-visible:outline-focus"
        style={{ height: viewport }}
      >
        <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
          {virtualizer.getVirtualItems().map((item) => {
            const row = rows[item.index];
            if (row === undefined) {
              return null;
            }
            return (
              <div
                key={row.id}
                role="row"
                aria-rowindex={item.index + 2}
                className="absolute inset-x-0 grid border-b border-hairline hover:bg-hover"
                style={{ ...template, height: rowHeight, insetBlockStart: item.start }}
              >
                {row.getAllCells().map((cell) => (
                  <div key={cell.id} role="cell" className="flex items-center truncate px-2 font-mono text-data">
                    <table.FlexRender cell={cell} />
                  </div>
                ))}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
