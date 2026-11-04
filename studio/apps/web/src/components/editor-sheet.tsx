// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName } from "@plux/ui";
import type { ReactElement, ReactNode } from "react";
import "./editor-sheet.css";

/**
 * The full-screen editors' one structure (STU-063): a list column at the start where the editor has a list, the work
 * area in the middle, an optional column of details at the end, and a foot bar for what the validator found. Each
 * column scrolls on its own; the sheet itself never does.
 */
export function SheetLayout({
  nav,
  aside,
  foot,
  children,
}: {
  readonly nav?: ReactNode;
  readonly aside?: ReactNode;
  readonly foot?: ReactNode;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <div
      className="sheet-layout"
      data-nav={nav !== undefined || undefined}
      data-aside={aside !== undefined || undefined}
    >
      {nav !== undefined && <div className="sheet-nav">{nav}</div>}
      <div className="sheet-main">{children}</div>
      {aside !== undefined && <div className="sheet-aside">{aside}</div>}
      {foot !== undefined && <div className="sheet-foot">{foot}</div>}
    </div>
  );
}

/** The list column's header: what it lists, how many, and the action that adds one. */
export function SheetNavHead({
  title,
  count,
  action,
}: {
  readonly title: string;
  readonly count?: number;
  readonly action?: ReactNode;
}): ReactElement {
  return (
    <div className="sheet-nav-head">
      <h3>{title}</h3>
      {count !== undefined && <span className="sheet-count">{count}</span>}
      {action !== undefined && <span className="ms-auto">{action}</span>}
    </div>
  );
}

/** A titled group of fields or content in the work area, as a card. */
export function SheetSection({
  title,
  description,
  icon,
  actions,
  children,
}: {
  readonly title: string;
  readonly description?: string;
  readonly icon?: IconName;
  readonly actions?: ReactNode;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <section className="sheet-section" aria-label={title}>
      <header className="sheet-section-head">
        {icon !== undefined && (
          <span className="sheet-section-icon" aria-hidden="true">
            <Icon name={icon} size="pill" />
          </span>
        )}
        <div className="flex min-w-0 flex-1 flex-col">
          <h3>{title}</h3>
          {description !== undefined && <p>{description}</p>}
        </div>
        {actions !== undefined && <div className="flex items-center gap-2">{actions}</div>}
      </header>
      <div className="sheet-section-body">{children}</div>
    </section>
  );
}

/** Fields side by side on a wide sheet, one above the other on a narrow one; `wide` children take the whole row. */
export function FieldGrid({ children }: { readonly children: ReactNode }): ReactElement {
  return <div className="sheet-fields">{children}</div>;
}

/** An empty list or work area: an icon, what is missing, and the action that fills it. */
export function SheetEmpty({
  icon,
  title,
  text,
  action,
}: {
  readonly icon: IconName;
  readonly title: string;
  readonly text?: string;
  readonly action?: ReactNode;
}): ReactElement {
  return (
    <div className="sheet-empty">
      <span className="sheet-empty-icon" aria-hidden="true">
        <Icon name={icon} />
      </span>
      <p className="sheet-empty-title">{title}</p>
      {text !== undefined && <p className="sheet-empty-text">{text}</p>}
      {action}
    </div>
  );
}
