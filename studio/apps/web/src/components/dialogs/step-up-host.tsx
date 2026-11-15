// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { IdentityService } from "@plux/api";
import { type ReactElement, useEffect, useRef, useState } from "react";
import { assertionFields, requestOptions } from "../../features/security/step-up.ts";
import type { Session } from "../../lib/session.ts";
import { sharedBroker } from "../../lib/step-up-transport.ts";
import { StepUpDialog } from "./step-up-dialog.tsx";

interface Pending {
  readonly requestId: string;
  readonly options: CredentialRequestOptions;
  readonly settle: (confirmed: boolean) => void;
}

/**
 * The one place that asks for a step-up when any call is refused for want of one (SEC-103, ADR-0075): it registers
 * itself with the session, opens D3 with the server's challenge, and tells the waiting call whether the person
 * confirmed. Before, each page caught the refusal on its own and most dropped it, so a change that needed the key
 * only said "The change was not saved" (creating a team, adding a member, a policy).
 */
export function StepUpHost({ session }: { readonly session: Session }): ReactElement | null {
  const [pending, setPending] = useState<Pending | null>(null);
  const live = useRef(pending);
  live.current = pending;
  // The dialog closes itself after an assertion; that close is not a cancel.
  const asserted = useRef(false);
  useEffect(() => {
    const identity = createClient(IdentityService, session.transport);
    const broker = sharedBroker(async () => {
      let begun: { requestId: string; optionsJson: string };
      try {
        begun = await identity.beginStepUp({});
        // refusal-ok: the waiting call is told "no" and shows its own refusal (PLX-6200) with its reason.
      } catch {
        return false;
      }
      return new Promise<boolean>((resolve) => {
        asserted.current = false;
        setPending({ requestId: begun.requestId, options: requestOptions(begun.optionsJson), settle: resolve });
      });
    });
    session.setStepUp(broker);
    return () => {
      session.setStepUp(undefined);
      live.current?.settle(false);
    };
  }, [session]);
  if (pending === null) {
    return null;
  }
  return (
    <StepUpDialog
      open
      onOpenChange={(open) => {
        if (!open && !asserted.current) {
          setPending(null);
          pending.settle(false);
        }
      }}
      options={pending.options}
      onAssertion={(credential) => {
        asserted.current = true;
        setPending(null);
        createClient(IdentityService, session.transport)
          .completeStepUp({ requestId: pending.requestId, ...assertionFields(credential) })
          .then(
            () => pending.settle(true),
            () => pending.settle(false),
          );
      }}
    />
  );
}
