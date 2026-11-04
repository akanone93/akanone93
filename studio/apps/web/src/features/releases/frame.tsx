// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement, ReactNode } from "react";
import { PageFrame } from "../../components/page.tsx";
import { releasesMessages } from "./releases.messages.ts";

/** Heading, caption and the loading and error states shared by the pages of the area. */
export function Frame({
  id,
  title,
  caption,
  loading,
  failed,
  error,
  nav,
  actions,
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
  readonly actions?: ReactNode;
  readonly children: ReactNode;
}): ReactElement {
  const m = releasesMessages(useLingui().i18n);
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
      {...(actions === undefined ? {} : { actions })}
    >
      {children}
    </PageFrame>
  );
}
