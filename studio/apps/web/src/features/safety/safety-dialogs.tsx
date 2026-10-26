// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Input } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { safetyMessages } from "./safety.messages.ts";
import type { DeleteCheck, RenameEffect, Use } from "./where-used.ts";

/**
 * The one shared type-to-confirm dialog for destructive actions affecting production (STU-074,
 * decisions F13): disabling a plugin, rolling back, deleting a published plugin. The action stays
 * disabled until the exact name is typed; approval policies and step-up follow on the server.
 */
export function TypeToConfirmDialog({
  open,
  title,
  description,
  name,
  action,
  onConfirm,
  onOpenChange,
}: {
  readonly open: boolean;
  readonly title: string;
  readonly description: string;
  /** What must be typed: the plugin key or environment name the action affects. */
  readonly name: string;
  /** The label of the destructive button, naming the action. */
  readonly action: string;
  readonly onConfirm: () => void;
  readonly onOpenChange: (open: boolean) => void;
}): ReactElement {
  const m = safetyMessages(useLingui().i18n);
  const [typed, setTyped] = useState("");
  const matches = typed === name;
  const close = (next: boolean) => {
    if (!next) {
      setTyped("");
    }
    onOpenChange(next);
  };
  return (
    <Dialog open={open} onOpenChange={close} title={title} description={description} labels={{ close: m.close }}>
      <form
        className="mt-3 flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (matches) {
            setTyped("");
            onConfirm();
          }
        }}
      >
        <Field
          label={m.confirmInstruction(name)}
          {...(typed !== "" && !matches ? { description: m.confirmMismatch } : {})}
        >
          <Input
            value={typed}
            autoComplete="off"
            spellCheck={false}
            dir="ltr"
            onChange={(e) => setTyped(e.currentTarget.value)}
          />
        </Field>
        <div className="flex flex-wrap justify-end gap-2">
          <Button variant="ghost" type="button" onClick={() => close(false)}>
            {m.cancel}
          </Button>
          <Button variant="danger" type="submit" disabled={!matches}>
            {action}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function UseList({
  title,
  uses,
  onNavigate,
}: {
  readonly title: string;
  readonly uses: readonly Use[];
  readonly onNavigate: (use: Use) => void;
}): ReactElement | null {
  const m = safetyMessages(useLingui().i18n);
  if (uses.length === 0) {
    return null;
  }
  return (
    <section className="mt-3">
      <h3 className="text-label text-secondary">{title}</h3>
      <ul className="mt-1 flex max-h-60 flex-col gap-1 overflow-auto">
        {uses.map((u) => (
          <li key={`${u.path}#${u.pointer}`} className="flex items-center justify-between gap-2">
            <span className="text-body text-primary">{u.fromTitle}</span>
            <Button variant="ghost" size="sm" aria-label={m.goToUse(u.fromTitle)} onClick={() => onNavigate(u)}>
              <span className="font-mono text-tag" dir="ltr">
                {u.path}
              </span>
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * D19, D24, D26: a blocked delete lists every use with navigation to it, and offers the assisted
 * fixes that apply; each fix is one change set and one undo step.
 */
export function BlockedDeletePanel({
  open,
  check,
  onNavigate,
  onRedirect,
  onRemoveOutgoing,
  onOpenChange,
}: {
  readonly open: boolean;
  readonly check: DeleteCheck;
  readonly onNavigate: (use: Use) => void;
  /** Offered when links enter a screen or plugin. */
  readonly onRedirect?: () => void;
  /** Offered when links leave a plugin. */
  readonly onRemoveOutgoing?: () => void;
  readonly onOpenChange: (open: boolean) => void;
}): ReactElement {
  const m = safetyMessages(useLingui().i18n);
  const links = check.incoming.filter((u) => u.kind === "navigates");
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={m.blockedTitle} size="lg" labels={{ close: m.close }}>
      {check.entry && <p className="mt-2 text-body text-primary">{m.blockedEntry}</p>}
      <UseList title={m.blockedIncoming(check.incoming.length)} uses={check.incoming} onNavigate={onNavigate} />
      <UseList title={m.blockedOutgoing(check.outgoing.length)} uses={check.outgoing} onNavigate={onNavigate} />
      <div className="mt-4 flex flex-wrap justify-end gap-2">
        {onRemoveOutgoing !== undefined && check.outgoing.length > 0 && (
          <Button variant="secondary" onClick={onRemoveOutgoing}>
            {m.removeOutgoing}
          </Button>
        )}
        {onRedirect !== undefined && links.length > 0 && (
          <Button variant="primary" onClick={onRedirect}>
            {m.redirectIncoming}
          </Button>
        )}
      </div>
    </Dialog>
  );
}

/** D27, STU-076: the deep links and generated APIs a key or route change affects. */
export function RenameWarning({ effects }: { readonly effects: readonly RenameEffect[] }): ReactElement | null {
  const m = safetyMessages(useLingui().i18n);
  if (effects.length === 0) {
    return null;
  }
  return (
    <section role="alert" className="mt-3">
      <h3 className="text-label text-primary">{m.renameTitle}</h3>
      <ul className="mt-1 flex flex-col gap-1">
        {effects.map((e) => (
          <li key={`${e.kind}:${e.before}`} className="flex flex-wrap items-baseline gap-2">
            <span className="text-label text-secondary">
              {e.kind === "deepLink" ? m.renameDeepLink : m.renameGenerated}
            </span>
            <span className="font-mono text-data" dir="ltr">
              {e.before} → {e.after}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
