// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon } from "@plux/ui";
import { Alert, AlertTitle } from "@plux/ui/components/alert";
import { Spinner } from "@plux/ui/components/spinner";
import { Table, TableCaption, TableCell, TableHead, TableHeader, TableRow } from "@plux/ui/components/table";
import type { ReactElement, ReactNode } from "react";
import { AnimatedRows } from "./animated-list.tsx";
import { Refusal } from "./refusal.tsx";
import "./page.css";

/** The top of a page: its heading, what it is for, and the page's own actions on the end side. */
export function PageHeader({
  id,
  title,
  caption,
  actions,
}: {
  readonly id: string;
  readonly title: ReactNode;
  readonly caption?: ReactNode;
  readonly actions?: ReactNode;
}): ReactElement {
  return (
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div className="flex min-w-0 flex-col gap-1">
        <h1 id={id} className="text-2xl font-semibold tracking-tight">
          {title}
        </h1>
        {caption !== undefined && <p className="max-w-prose text-sm text-muted-foreground">{caption}</p>}
      </div>
      {actions !== undefined && <div className="flex items-center gap-2">{actions}</div>}
    </header>
  );
}

/**
 * A page's loading, failure and save-failure states, in the same words and roles on every page. A failed read says why
 * the server refused it, from `error`, and how to get past it (step-up, licence, permission, another try).
 */
export function PageStatus({
  loading,
  failed,
  error,
  saveFailed = false,
  labels,
}: {
  readonly loading: boolean;
  readonly failed: boolean;
  /** The failed read's error, when there is one. */
  readonly error?: unknown;
  readonly saveFailed?: boolean;
  readonly labels: { readonly loading: string; readonly loadFailed: string; readonly saveFailed?: string };
}): ReactElement {
  return (
    <>
      {loading && !failed && (
        <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner />
          {labels.loading}
        </p>
      )}
      {failed && <Refusal error={error} failedTitle={labels.loadFailed} />}
      {saveFailed && labels.saveFailed !== undefined && (
        <Alert variant="destructive" role="alert">
          <Icon name="warning-circle" />
          <AlertTitle>{labels.saveFailed}</AlertTitle>
        </Alert>
      )}
    </>
  );
}

/**
 * A page of an area: header, states, then the content once it has loaded. Every area's frame is this one with the
 * area's own words.
 */
export function PageFrame({
  id,
  title,
  caption,
  actions,
  loading = false,
  failed = false,
  error,
  saveFailed = false,
  labels,
  children,
}: {
  readonly id: string;
  readonly title: ReactNode;
  readonly caption?: ReactNode;
  readonly actions?: ReactNode;
  readonly loading?: boolean;
  readonly failed?: boolean;
  /** The failed read's error, so the page can say why it was refused. */
  readonly error?: unknown;
  readonly saveFailed?: boolean;
  readonly labels: { readonly loading: string; readonly loadFailed: string; readonly saveFailed?: string };
  readonly children: ReactNode;
}): ReactElement {
  return (
    <section className="page-frame flex flex-col gap-6" aria-labelledby={id}>
      <PageHeader id={id} title={title} caption={caption} actions={actions} />
      <PageStatus loading={loading} failed={failed} error={error} saveFailed={saveFailed} labels={labels} />
      {!loading && !failed && children}
    </section>
  );
}

/** One row of a data table: a stable key and its cells in column order. */
export interface DataRow {
  readonly key: string;
  readonly cells: readonly ReactNode[];
}

/**
 * shadcn's table in a bordered frame, with a muted header row; an empty list says so in the same frame. Rows animate as
 * a filter changes them.
 */
export function DataTable({
  caption,
  heads,
  rows,
  empty,
}: {
  readonly caption: string;
  readonly heads: readonly string[];
  readonly rows: readonly DataRow[];
  readonly empty?: string;
}): ReactElement {
  if (rows.length === 0 && empty !== undefined) {
    return (
      <p className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">{empty}</p>
    );
  }
  return (
    <div className="data-table overflow-hidden rounded-lg border">
      <Table>
        <TableCaption className="sr-only">{caption}</TableCaption>
        <TableHeader className="bg-muted/50">
          <TableRow>
            {heads.map((h) => (
              <TableHead key={h} scope="col" className="px-4">
                {h}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <AnimatedRows
          rows={rows}
          keyOf={(r) => r.key}
          render={(r) =>
            r.cells.map((c, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: cells have a fixed order
              <TableCell key={i} className="px-4">
                {c}
              </TableCell>
            ))
          }
        />
      </Table>
    </div>
  );
}
