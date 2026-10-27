// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type ReactElement, useState } from "react";
import { StepUpDialog } from "../../components/dialogs/index.ts";
import { useStepUp } from "../security/use-security.ts";
import { refusalOf } from "./use-approvals.ts";

/**
 * D3 before a sensitive action (SEC-103): `begin` fetches the challenge and opens the dialog; the action runs only after
 * the assertion has been verified. A cancelled or failed dialog sends nothing. A refusal of the action itself is kept
 * in `problem` as the server worded it.
 */
export function useStepUpGate(act: () => Promise<void>, fallback: string) {
  const stepUp = useStepUp();
  const [challenge, setChallenge] = useState<{ requestId: string; options: CredentialRequestOptions }>();
  const [problem, setProblem] = useState<string>();
  const fail = (error: unknown) => setProblem(refusalOf(error) || fallback);
  const begin = () => {
    setProblem(undefined);
    stepUp.begin().then(setChallenge, () => setProblem(fallback));
  };
  const confirmed = (requestId: string, credential: Credential) => {
    stepUp
      .complete(requestId, credential)
      .then(act)
      .then(() => setProblem(undefined), fail);
  };
  const dialog: ReactElement | null =
    challenge === undefined ? null : (
      <StepUpDialog
        open
        onOpenChange={(open) => {
          if (!open) {
            setChallenge(undefined);
          }
        }}
        options={challenge.options}
        onAssertion={(credential) => confirmed(challenge.requestId, credential)}
      />
    );
  return { begin, dialog, problem };
}
