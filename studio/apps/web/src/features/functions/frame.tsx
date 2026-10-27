// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement, ReactNode } from "react";
import { functionsMessages } from "./functions.messages.ts";

/** Heading and the loading and error states shared by the pages of the area. */
export function Frame({
  id,
  title,
  loading,
  failed,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly loading: boolean;
  readonly failed: boolean;
  readonly children: ReactNode;
}): ReactElement {
  const m = functionsMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-4" aria-labelledby={id}>
      <h1 id={id} className="text-screen">
        {title}
      </h1>
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
