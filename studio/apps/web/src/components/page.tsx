// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName } from "@plux/ui";
import { Alert, AlertTitle } from "@plux/ui/components/alert";
import { Spinner } from "@plux/ui/components/spinner";
import { Table, TableCaption, TableCell, TableHead, TableHeader, TableRow } from "@plux/ui/components/table";
import { Link } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { usePageArea } from "../features/shell/page-area.ts";
import { AnimatedRows } from "./animated-list.tsx";
import { Refusal } from "./refusal.tsx";
import "./page.css";

/**
 * The top of a page, as the canvases' docks head theirs: the area's mark on the primary colour, the area's name over
 * the page's heading and what it is for, the page's own actions on the end side, and the area's pages as a tab strip
 * along its foot. The mark and the area come from the sidebar's entry for the page unless the page names its own.
 */
export function PageHeader({
  id,
  title,
  caption,
  actions,
  icon,
  kicker,
  nav,
}: {
  readonly id: string;
  readonly title: ReactNode;
  readonly caption?: ReactNode;
  readonly actions?: ReactNode;
  /** The page's own mark, instead of its area's. */
  readonly icon?: IconName;
  /** The line above the heading, instead of the area's name. */
  readonly kicker?: ReactNode;
  /** The area's pages, as links with `data-status="active"` on the open one. */
  readonly nav?: ReactNode;
}): ReactElement {
  const area = usePageArea();
  const mark = icon ?? area?.icon ?? "squares-four";
  const over = kicker ?? area?.label;
  return (
    <header className="page-hero" data-nav={nav === undefined ? undefined : ""}>
      <div className="page-hero-main">
        <span className="page-hero-mark" aria-hidden="true">
          <Icon name={mark} />
        </span>
        <div className="page-hero-text">
          {over !== undefined && <span className="page-hero-kicker">{over}</span>}
          <h1 id={id} className="page-hero-title">
            {title}
          </h1>
          {caption !== undefined && <p className="page-hero-caption">{caption}</p>}
        </div>
        {actions !== undefined && <div className="page-hero-actions">{actions}</div>}
      </div>
      {nav !== undefined && <div className="page-hero-nav">{nav}</div>}
    </header>
  );
}

/** One page of an area in its tab strip: where it lives, its name and mark; the landing page matches exactly. */
export interface AreaTab {
  readonly href: string;
  readonly label: string;
  readonly icon?: IconName;
  readonly exact?: boolean;
}

/** The pages of an area as the header's tab strip (`nav` of PageHeader); a tab keeps the page's search, such as the environment. */
export function AreaTabs({ label, tabs }: { readonly label: string; readonly tabs: readonly AreaTab[] }): ReactElement {
  return (
    <nav aria-label={label} className="sub-nav">
      {tabs.map((t) => (
        <Link key={t.href} to={t.href} search={true} activeOptions={{ exact: t.exact === true, includeSearch: false }}>
          {t.icon !== undefined && <Icon name={t.icon} size="pill" />}
          {t.label}
        </Link>
      ))}
    </nav>
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
  icon,
  kicker,
  nav,
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
  readonly icon?: IconName;
  readonly kicker?: ReactNode;
  /** The area's pages, shown along the foot of the header. */
  readonly nav?: ReactNode;
  readonly loading?: boolean;
  readonly failed?: boolean;
  /** The failed read's error, so the page can say why it was refused. */
  readonly error?: unknown;
  readonly saveFailed?: boolean;
  readonly labels: { readonly loading: string; readonly loadFailed: string; readonly saveFailed?: string };
  readonly children: ReactNode;
}): ReactElement {
  return (
    <section className="page-frame" aria-labelledby={id}>
      <PageHeader
        id={id}
        title={title}
        caption={caption}
        actions={actions}
        {...(icon === undefined ? {} : { icon })}
        {...(kicker === undefined ? {} : { kicker })}
        {...(nav === undefined ? {} : { nav })}
      />
      <div className="page-body">
        <PageStatus loading={loading} failed={failed} error={error} saveFailed={saveFailed} labels={labels} />
        {!loading && !failed && children}
      </div>
    </section>
  );
}

/**
 * An empty list or section: a mark over the canvas's dot grid and what is missing, with the action that fills it when
 * there is one.
 */
export function PageEmpty({
  text,
  icon = "inbox",
  action,
}: {
  readonly text: ReactNode;
  readonly icon?: IconName;
  readonly action?: ReactNode;
}): ReactElement {
  return (
    <div className="page-empty">
      <span className="page-empty-mark" aria-hidden="true">
        <Icon name={icon} />
      </span>
      <p>{text}</p>
      {action}
    </div>
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
    return <PageEmpty text={empty} />;
  }
  return (
    <div className="data-table">
      <Table>
        <TableCaption className="sr-only">{caption}</TableCaption>
        <TableHeader>
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
