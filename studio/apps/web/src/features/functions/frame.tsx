// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement, ReactNode } from "react";
import { PageFrame } from "../../components/page.tsx";
import { functionsMessages } from "./functions.messages.ts";

/** Heading and the loading and error states shared by the pages of the area. */
export function Frame({
  id,
  title,
  loading,
  failed,
  error,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly loading: boolean;
  readonly failed: boolean;
  readonly error?: unknown;
  readonly children: ReactNode;
}): ReactElement {
  const m = functionsMessages(useLingui().i18n);
  return (
    <PageFrame id={id} title={title} loading={loading} failed={failed} error={error} labels={m}>
      {children}
    </PageFrame>
  );
}
