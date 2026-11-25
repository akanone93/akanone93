// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, StatusPill } from "@plux/ui";
import type { ReactElement } from "react";
import { SetupToDo } from "../../components/setup-to-do.tsx";
import { newAppMessages } from "./new-app.messages.ts";
import type { CreatedApp } from "./use-new-app.ts";

/**
 * The last step (`step=done`): the app exists; open it. Its generated Flutter project (GEN-001) is downloaded from the
 * app's Native shell page once a release gives it signing keys; this step says how (build log row 234).
 */
export function DoneStep({
  created,
  onOpen,
  onClose,
}: {
  readonly created: CreatedApp;
  readonly onOpen: () => void;
  readonly onClose: () => void;
}): ReactElement {
  const m = newAppMessages(useLingui().i18n);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <h3 className="font-serif text-title text-foreground">{m.doneTitle(created.name)}</h3>
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-data text-muted-foreground">{created.key}</span>
          <span className="text-caption text-muted-foreground">{m.doneDocuments(created.documents)}</span>
          {created.errors > 0 && <StatusPill status="warning" label={m.doneErrors(created.errors)} />}
        </div>
      </div>
      <SetupToDo steps={created.setupToDo} />
      <section aria-labelledby="new-app-project" className="flex flex-col gap-2">
        <h3 id="new-app-project" className="text-section">
          {m.projectTitle}
        </h3>
        <p className="text-body text-muted-foreground">{m.projectHowTo}</p>
      </section>
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          {m.doneClose}
        </Button>
        <Button onClick={onOpen}>{m.doneOpen}</Button>
      </div>
    </div>
  );
}
