// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { PublicColumn } from "./layouts.tsx";
import { notifyMessages } from "./notify.messages.ts";

/** P8: shown when WebGL2 is unavailable (A5.8). */
export function UnsupportedPage() {
  const m = notifyMessages(useLingui().i18n);
  return (
    <PublicColumn>
      <section className="flex flex-col gap-3">
        <h1 className="text-screen">{m.unsupportedTitle}</h1>
        <p className="text-body text-muted-foreground">{m.unsupportedBody}</p>
      </section>
    </PublicColumn>
  );
}
