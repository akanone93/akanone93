// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { Adoption, AdoptionConflict, LibraryVersion } from "@plux/api";
import { Button, Dialog, Select } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { libraryMessages } from "./library.messages.ts";
import { interfaceDiff } from "./library-model.ts";

/**
 * D32: adopt a component version in one app, explicitly. The diff compares the props and slots of the adopted
 * version with the chosen one; after adoption the instances whose overrides name removed props are flagged
 * (STU-085). The server keeps overrides where the prop still exists.
 */
export function AdoptDialog({
  appId,
  appName,
  versions,
  adopted,
  adopt,
  onClose,
}: {
  readonly appId: string;
  readonly appName: string;
  readonly versions: readonly LibraryVersion[];
  readonly adopted: Adoption | undefined;
  readonly adopt: (appId: string, version: number) => Promise<readonly AdoptionConflict[]>;
  readonly onClose: () => void;
}): ReactElement {
  const m = libraryMessages(useLingui().i18n);
  const newest = versions.reduce((a, v) => Math.max(a, v.version), 0);
  const [version, setVersion] = useState(newest);
  const [conflicts, setConflicts] = useState<readonly AdoptionConflict[] | undefined>();
  const [failed, setFailed] = useState(false);
  const diff = interfaceDiff(
    versions.find((v) => v.version === adopted?.version)?.content,
    versions.find((v) => v.version === version)?.content,
  );
  const lists: [string, string[]][] = [
    [m.adoptAddedProps, diff.addedProps],
    [m.adoptRemovedProps, diff.removedProps],
    [m.adoptAddedSlots, diff.addedSlots],
    [m.adoptRemovedSlots, diff.removedSlots],
  ];
  const submit = async () => {
    try {
      setConflicts(await adopt(appId, version));
    } catch {
      setFailed(true);
    }
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.adoptTitle(appName)} labels={{ close: m.close }}>
      {conflicts === undefined && (
        <>
          <Select
            label={m.versionsHeading}
            value={String(version)}
            options={versions.map((v) => ({ value: String(v.version), label: m.headVersion(v.version) }))}
            onValueChange={(v) => setVersion(Number(v))}
          />
          <h3 className="mt-3 text-body font-medium">{m.adoptDiff}</h3>
          {lists.every(([, names]) => names.length === 0) && (
            <p className="text-body text-secondary">{m.adoptNoDiff}</p>
          )}
          {lists
            .filter(([, names]) => names.length > 0)
            .map(([label, names]) => (
              <p key={label} className="text-body">
                {label}: <code>{names.join(", ")}</code>
              </p>
            ))}
          {failed && (
            <p role="alert" className="text-body text-danger">
              {m.adoptFailed}
            </p>
          )}
          <div className="mt-4 flex justify-end gap-2">
            <Button variant="secondary" onClick={onClose}>
              {m.cancel}
            </Button>
            <Button disabled={adopted?.version === version} onClick={() => void submit()}>
              {m.adoptConfirm}
            </Button>
          </div>
        </>
      )}
      {conflicts !== undefined && (
        <>
          <p className="text-body">{m.adoptDone(version)}</p>
          {conflicts.length > 0 && <h3 className="text-body font-medium text-danger">{m.adoptFlagged}</h3>}
          <ul className="text-body">
            {conflicts.map((c) => (
              <li key={`${c.file}${c.path}`}>
                <code>{c.file + c.path}</code>: {[...c.removedProps, ...c.removedSlots].join(", ")}
              </li>
            ))}
          </ul>
          <div className="mt-4 flex justify-end">
            <Button onClick={onClose}>{m.close}</Button>
          </div>
        </>
      )}
    </Dialog>
  );
}
