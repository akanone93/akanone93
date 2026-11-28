// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type ReactElement, useState } from "react";
import { StepUpDialog } from "../../components/dialogs/index.ts";
import { useStepUp } from "../security/use-security.ts";

/**
 * Runs a destructive action only after a verified WebAuthn step-up D3 (SEC-103); a cancelled dialog runs nothing. A
 * failure keeps its error, so the caller can show the server's reason with it (SRV-006).
 */
export function useGuarded(): {
  readonly guard: (action: () => Promise<unknown>) => void;
  readonly dialog: ReactElement | null;
  readonly failed: boolean;
  readonly error: unknown;
} {
  const stepUp = useStepUp();
  const [pending, setPending] = useState<{
    requestId: string;
    options: CredentialRequestOptions;
    action: () => Promise<unknown>;
  }>();
  const [outcome, setOutcome] = useState<{ readonly failed: boolean; readonly error?: unknown }>({ failed: false });
  const fail = (error: unknown) => setOutcome({ failed: true, error });
  const guard = (action: () => Promise<unknown>) => {
    setOutcome({ failed: false });
    stepUp.begin().then((c) => setPending({ ...c, action }), fail);
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
          stepUp.complete(requestId, credential).then(action).catch(fail);
        }}
      />
    );
  return { guard, dialog, failed: outcome.failed, error: outcome.error };
}
