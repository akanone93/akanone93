// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement, ReactNode } from "react";
import { type DataRow, DataTable, PageFrame } from "../../components/page.tsx";
import { securityMessages } from "./security.messages.ts";

/** Heading, caption and the loading, error and empty states shared by the pages of the area. */
export function Frame({
  id,
  title,
  caption,
  loading,
  failed,
  error,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly caption?: string;
  readonly loading: boolean;
  readonly failed: boolean;
  readonly error?: unknown;
  readonly children: ReactNode;
}): ReactElement {
  const m = securityMessages(useLingui().i18n);
  return (
    <PageFrame id={id} title={title} caption={caption} loading={loading} failed={failed} error={error} labels={m}>
      {children}
    </PageFrame>
  );
}

export function Table({
  caption,
  heads,
  rows,
}: {
  readonly caption: string;
  readonly heads: readonly string[];
  readonly rows: readonly DataRow[];
}): ReactElement {
  return <DataTable caption={caption} heads={heads} rows={rows} />;
}

export function fmt(date: Date | undefined, never: string): string {
  return date === undefined ? never : date.toLocaleString();
}
