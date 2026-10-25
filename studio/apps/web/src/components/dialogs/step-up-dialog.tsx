// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { dialogMessages } from "./dialogs.messages.ts";

export interface StepUpDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** The server's challenge, already parsed; this dialog starts no auth flow of its own (SEC-103). */
  readonly options: CredentialRequestOptions;
  readonly onAssertion: (credential: Credential) => void;
}

/** D3: asks the authenticator once; the caller sends the assertion on. */
export function StepUpDialog({ open, onOpenChange, options, onAssertion }: StepUpDialogProps): ReactElement {
  const m = dialogMessages(useLingui().i18n);
  const [failed, setFailed] = useState(false);
  const run = async () => {
    setFailed(false);
    try {
      const credential = await navigator.credentials.get(options);
      if (credential === null) {
        setFailed(true);
        return;
      }
      onAssertion(credential);
      onOpenChange(false);
    } catch {
      setFailed(true);
    }
  };
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      size="sm"
      title={m.stepUpTitle}
      description={m.stepUpBody}
      labels={{ close: m.close }}
    >
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.stepUpFailed}
        </p>
      )}
      <div className="flex justify-end">
        <Button onClick={() => void run()}>{m.stepUpUse}</Button>
      </div>
    </Dialog>
  );
}
