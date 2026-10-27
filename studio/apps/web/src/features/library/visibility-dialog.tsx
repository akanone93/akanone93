// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { libraryMessages } from "./library.messages.ts";
import { type Finding, isScanFailure, scanFindings, VISIBILITIES, type Visibility } from "./library-model.ts";

type Outcome =
  | { readonly kind: "idle" }
  | { readonly kind: "failed" }
  | { readonly kind: "scan"; readonly findings: Finding[] };

/**
 * D14: change the visibility of a library item. Making an item public runs the server's scan first; a refusal
 * (PLX-8063) lists every finding by JSON pointer and category, never the value (STU-081).
 */
export function VisibilityDialog({
  current,
  canPublish,
  apply,
  onClose,
}: {
  readonly current: string;
  readonly canPublish: boolean;
  readonly apply: (visibility: Visibility) => Promise<void>;
  readonly onClose: () => void;
}): ReactElement {
  const m = libraryMessages(useLingui().i18n);
  const [choice, setChoice] = useState<Visibility>(VISIBILITIES.find((v) => v === current) ?? "private");
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<Outcome>({ kind: "idle" });
  const names: Record<Visibility, string> = {
    private: m.visPrivate,
    organization: m.visOrganization,
    public: m.visPublic,
  };
  const submit = async () => {
    setBusy(true);
    try {
      await apply(choice);
      onClose();
    } catch (err) {
      setOutcome(isScanFailure(err) ? { kind: "scan", findings: scanFindings(err) } : { kind: "failed" });
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={m.visibilityTitle} labels={{ close: m.close }}>
      <p className="text-body text-secondary">{m.visibilityNote}</p>
      <fieldset className="mt-3 flex flex-col gap-2">
        <legend className="text-body font-medium">{m.visibilityLabel}</legend>
        {VISIBILITIES.map((v) => (
          <label key={v} className="flex items-center gap-2 text-body">
            <input
              type="radio"
              name="visibility"
              value={v}
              checked={choice === v}
              disabled={v === "public" && !canPublish}
              onChange={() => setChoice(v)}
            />
            {names[v]}
          </label>
        ))}
      </fieldset>
      {outcome.kind === "failed" && (
        <p role="alert" className="mt-3 text-body text-danger">
          {m.visibilityFailed}
        </p>
      )}
      {outcome.kind === "scan" && (
        <section role="alert" className="mt-3">
          <h3 className="text-body font-medium text-danger">{m.scanTitle}</h3>
          <ul className="text-body">
            {outcome.findings.map((f) => (
              <li key={`${f.pointer}:${f.category}`}>
                <code>{m.scanFinding(f.pointer, f.category)}</code>
              </li>
            ))}
          </ul>
          <p className="text-caption text-secondary">{m.scanHelp}</p>
        </section>
      )}
      <div className="mt-4 flex justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          {m.cancel}
        </Button>
        <Button disabled={busy || choice === current} onClick={() => void submit()}>
          {m.visibilityApply}
        </Button>
      </div>
    </Dialog>
  );
}
