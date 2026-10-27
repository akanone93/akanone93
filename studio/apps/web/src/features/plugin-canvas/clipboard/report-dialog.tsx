// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import type { ReactElement } from "react";
import { clipboardMessages } from "./clipboard.messages.ts";
import type { DependencyKind, DependencyReport, RefusalReason } from "./envelope.ts";

const KINDS: readonly DependencyKind[] = ["functions", "translations", "assets", "dataSources"];

/**
 * The dependency report of a paste across plugins or apps (D34): per kind what the target lacks and what
 * Studio will do about it, or the reason a paste was refused. IDs render as text only.
 */
export function ClipboardReportDialog({
  report,
  refusal,
  onConfirm,
  onClose,
}: {
  readonly report: DependencyReport | undefined;
  readonly refusal: RefusalReason | undefined;
  readonly onConfirm: () => void;
  readonly onClose: () => void;
}): ReactElement {
  const m = clipboardMessages(useLingui().i18n);
  const heading = {
    functions: m.functions,
    translations: m.translations,
    assets: m.assets,
    dataSources: m.dataSources,
  };
  const action = {
    functions: m.functionsAction,
    translations: m.translationsAction,
    assets: m.assetsAction,
    dataSources: m.dataSourcesAction,
  };
  const why = {
    "too-large": m.refusedTooLarge,
    "unknown-version": m.refusedVersion,
    malformed: m.refusedMalformed,
    invalid: m.refusedInvalid,
  };
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={refusal === undefined ? m.reportTitle : m.refusedTitle}
      labels={{ close: m.close }}
    >
      {refusal !== undefined && <p className="text-body text-secondary">{why[refusal]}</p>}
      {refusal === undefined && report !== undefined && (
        <>
          <p className="text-body text-secondary">{m.reportIntro}</p>
          {KINDS.filter((k) => report[k].length > 0).map((k) => (
            <section key={k} className="mt-3">
              <h3 className="text-body font-medium">{heading[k]}</h3>
              <ul className="text-body">
                {report[k].map((id) => (
                  <li key={id}>
                    <code>{id}</code>
                  </li>
                ))}
              </ul>
              <p className="text-body text-secondary">{action[k]}</p>
            </section>
          ))}
        </>
      )}
      <div className="mt-4 flex flex-wrap justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          {refusal === undefined ? m.cancel : m.close}
        </Button>
        {refusal === undefined && <Button onClick={onConfirm}>{m.pasteAnyway}</Button>}
      </div>
    </Dialog>
  );
}
