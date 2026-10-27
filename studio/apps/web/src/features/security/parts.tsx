// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement, ReactNode } from "react";
import { securityMessages } from "./security.messages.ts";

/** Heading, caption and the loading, error and empty states shared by the pages of the area. */
export function Frame({
  id,
  title,
  caption,
  loading,
  failed,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly caption?: string;
  readonly loading: boolean;
  readonly failed: boolean;
  readonly children: ReactNode;
}): ReactElement {
  const m = securityMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-4" aria-labelledby={id}>
      <h1 id={id} className="text-screen">
        {title}
      </h1>
      {caption !== undefined && <p className="text-body text-secondary">{caption}</p>}
      {loading && !failed && (
        <p role="status" className="text-body text-secondary">
          {m.loading}
        </p>
      )}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.loadFailed}
        </p>
      )}
      {!loading && !failed && children}
    </section>
  );
}

export function Table({
  caption,
  heads,
  rows,
}: {
  readonly caption: string;
  readonly heads: readonly string[];
  readonly rows: readonly { readonly key: string; readonly cells: readonly ReactNode[] }[];
}): ReactElement {
  return (
    <table className="w-full text-left text-body">
      <caption className="sr-only">{caption}</caption>
      <thead>
        <tr>
          {heads.map((h) => (
            <th key={h} scope="col" className="p-2 text-caption text-secondary">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.key}>
            {r.cells.map((c, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: cells have a fixed order
              <td key={i} className="p-2">
                {c}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function fmt(date: Date | undefined, never: string): string {
  return date === undefined ? never : date.toLocaleString();
}
