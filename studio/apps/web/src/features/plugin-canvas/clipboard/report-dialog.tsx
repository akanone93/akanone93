// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { clipboardMessages } from "./clipboard.messages.ts";
import type { Dropped } from "./copy-over.ts";
import {
  choiceKey,
  type DependencyKind,
  presetChoices,
  type RefusalReason,
  type ReportItem,
  type Resolution,
  type ResolutionAction,
  type TargetHas,
} from "./envelope.ts";

type Messages = ReturnType<typeof clipboardMessages>;
type Choices = ReadonlyMap<string, Resolution>;

const KINDS: readonly DependencyKind[] = ["functions", "translations", "assets", "dataSources"];

const SELECT = "h-8 rounded-sm border border-border bg-surface px-2 text-body";

function ItemRow({
  item,
  candidates,
  choice,
  m,
  onChange,
}: {
  readonly item: ReportItem;
  readonly candidates: readonly string[];
  readonly choice: Resolution;
  readonly m: Messages;
  readonly onChange: (next: Resolution) => void;
}): ReactElement {
  const label: Record<ResolutionAction, string> = { map: m.actionMap, copy: m.actionCopy, drop: m.actionDrop };
  return (
    <li className="flex flex-wrap items-center gap-2 py-1">
      <code>{item.id}</code>
      <select
        className={SELECT}
        aria-label={m.actionFor(item.id)}
        value={choice.action}
        onChange={(e) => {
          const action = e.target.value as ResolutionAction;
          onChange(action === "map" ? { action, to: candidates[0] ?? "" } : { action });
        }}
      >
        {item.options.map((o) => (
          <option key={o} value={o}>
            {label[o]}
          </option>
        ))}
      </select>
      {choice.action === "map" && (
        <select
          className={SELECT}
          aria-label={m.mapTarget(item.id)}
          value={choice.to ?? ""}
          onChange={(e) => onChange({ action: "map", to: e.target.value })}
        >
          {candidates.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
      )}
    </li>
  );
}

/**
 * The dependency report of a paste (D34, F7): per missing item the user maps it to an existing one, copies it
 * over or drops it. Nothing is applied until the user confirms with the choices; cancelling applies nothing.
 * IDs render as text only. Also shows the reason a paste was refused.
 */
export function ClipboardReportDialog({
  items,
  candidates,
  refusal,
  dropped,
  onConfirm,
  onClose,
}: {
  readonly items: readonly ReportItem[] | undefined;
  readonly candidates: TargetHas;
  readonly refusal: RefusalReason | undefined;
  /** Items the copy over could not bring along; shown after the paste, never silently. */
  readonly dropped?: readonly Dropped[];
  readonly onConfirm: (choices: Choices) => void;
  readonly onClose: () => void;
}): ReactElement {
  const m = clipboardMessages(useLingui().i18n);
  const [choices, setChoices] = useState<Choices>(() => presetChoices(items ?? []));
  const heading = {
    functions: m.functions,
    translations: m.translations,
    assets: m.assets,
    dataSources: m.dataSources,
  };
  const why = {
    "too-large": m.refusedTooLarge,
    "unknown-version": m.refusedVersion,
    malformed: m.refusedMalformed,
    invalid: m.refusedInvalid,
  };
  const reason = { forbidden: m.dropForbidden, limit: m.dropLimit, rejected: m.dropRejected };
  const change = (item: ReportItem, next: Resolution) =>
    setChoices((c) => new Map(c).set(choiceKey(item.kind, item.id), next));
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={dropped !== undefined ? m.droppedTitle : refusal === undefined ? m.reportTitle : m.refusedTitle}
      labels={{ close: m.close }}
    >
      {dropped !== undefined && (
        <>
          <p className="text-body text-secondary">{m.droppedIntro}</p>
          <ul className="text-body">
            {dropped.map((x) => (
              <li key={`${x.kind}:${x.id}`} className="py-1">
                <code>{x.id}</code> {reason[x.reason]}
              </li>
            ))}
          </ul>
        </>
      )}
      {refusal !== undefined && <p className="text-body text-secondary">{why[refusal]}</p>}
      {dropped === undefined && refusal === undefined && items !== undefined && (
        <>
          <p className="text-body text-secondary">{m.reportIntro}</p>
          {KINDS.filter((k) => items.some((i) => i.kind === k)).map((k) => (
            <section key={k} className="mt-3">
              <h3 className="text-body font-medium">{heading[k]}</h3>
              <ul className="text-body">
                {items
                  .filter((i) => i.kind === k)
                  .map((item) => (
                    <ItemRow
                      key={item.id}
                      item={item}
                      candidates={[...candidates[k]].sort()}
                      choice={choices.get(choiceKey(k, item.id)) ?? item.preset}
                      m={m}
                      onChange={(next) => change(item, next)}
                    />
                  ))}
              </ul>
            </section>
          ))}
        </>
      )}
      <div className="mt-4 flex flex-wrap justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          {refusal === undefined && dropped === undefined ? m.cancel : m.close}
        </Button>
        {refusal === undefined && dropped === undefined && (
          <Button onClick={() => onConfirm(choices)}>{m.pasteWithChoices}</Button>
        )}
      </div>
    </Dialog>
  );
}
