// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useParams } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { AreaTabs, type DataRow, DataTable, PageFrame } from "../../components/page.tsx";
import { securityMessages } from "./security.messages.ts";

/** Heading, caption and the loading, error and empty states shared by the pages of the area. */
export function Frame({
  id,
  title,
  caption,
  loading,
  failed,
  error,
  nav,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly caption?: string;
  readonly loading: boolean;
  readonly failed: boolean;
  readonly error?: unknown;
  /** The area's pages, along the foot of the header. */
  readonly nav?: ReactNode;
  readonly children: ReactNode;
}): ReactElement {
  const m = securityMessages(useLingui().i18n);
  return (
    <PageFrame
      id={id}
      title={title}
      caption={caption}
      loading={loading}
      failed={failed}
      error={error}
      labels={m}
      {...(nav === undefined ? {} : { nav })}
    >
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

/** The pages of the security area as the header's tab strip: the profile, the capability grants, the keys and the pins. */
export function SecurityTabs(): ReactElement {
  const m = securityMessages(useLingui().i18n);
  const { org, app } = useParams({ strict: false }) as { org: string; app: string };
  const base = `/o/${org}/apps/${app}/security`;
  return (
    <AreaTabs
      label={m.areaNav}
      tabs={[
        { href: base, label: m.profileTitle, icon: "shield", exact: true },
        { href: `${base}/capabilities`, label: m.capsTitle, icon: "plug" },
        { href: `${base}/keys`, label: m.keysTitle, icon: "key" },
        { href: `${base}/pins`, label: m.pinsTitle, icon: "push-pin" },
      ]}
    />
  );
}
