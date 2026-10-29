// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type ReactElement, useState } from "react";
import { StepUpDialog } from "../../components/dialogs/index.ts";
import { useStepUp } from "../security/use-security.ts";

/** Runs a destructive action only after a verified WebAuthn step-up D3 (SEC-103); a cancelled dialog runs nothing. */
export function useGuarded(): {
  readonly guard: (action: () => Promise<unknown>) => void;
  readonly dialog: ReactElement | null;
  readonly failed: boolean;
} {
  const stepUp = useStepUp();
  const [pending, setPending] = useState<{
    requestId: string;
    options: CredentialRequestOptions;
    action: () => Promise<unknown>;
  }>();
  const [failed, setFailed] = useState(false);
  const guard = (action: () => Promise<unknown>) => {
    setFailed(false);
    stepUp.begin().then(
      (c) => setPending({ ...c, action }),
      () => setFailed(true),
    );
  };
  const dialog =
    pending === undefined ? null : (
      <StepUpDialog
        open
        onOpenChange={(o) => {
          if (!o) {
            setPending(undefined);
          }
        }}
        options={pending.options}
        onAssertion={(credential) => {
          const { requestId, action } = pending;
          setPending(undefined);
          stepUp
            .complete(requestId, credential)
            .then(action)
            .catch(() => setFailed(true));
        }}
      />
    );
  return { guard, dialog, failed };
}
