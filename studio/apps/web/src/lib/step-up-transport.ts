// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { ConnectError, type Transport } from "@connectrpc/connect";

/**
 * Asks the person for a step-up with their security key (SEC-103) and resolves whether they gave it. Several calls
 * refused at once share one ceremony.
 */
export type StepUpBroker = () => Promise<boolean>;

/** Whether a call was refused for want of a recent step-up: PLX-6200 required, PLX-6201 too old (ADR-0075). */
export function needsStepUp(error: unknown): boolean {
  return error instanceof ConnectError && /^PLX-620[01]\b/.test(error.rawMessage);
}

/**
 * Whether a method only reads: its name starts with a reading verb. A refused read is shown by its page, with a
 * button to confirm (`Refusal`); only a change the person asked for opens the dialog by itself.
 */
export function isRead(methodName: string): boolean {
  return /^(Get|List|Watch|Search|Explain|Describe|Query|Validate|Read|Count|Check|Preview|Lookup|Stat)[A-Z]?/.test(
    methodName,
  );
}

/**
 * Wraps a transport so that a change the server refuses for want of a step-up asks the person once, through the
 * broker the app root registers, and is sent again after they confirm; the CLI does the same (SEC-103). A refused call
 * did nothing on the server and released its idempotency key, so sending it again acts once. Without a broker (tests,
 * a page before the root mounts), or when the person cancels, the refusal is returned as it came. Reads are not
 * asked for (their pages offer the confirmation), nor are streams: no stream needs a step-up.
 */
export function withStepUp(inner: Transport, broker: () => StepUpBroker | undefined): Transport {
  return {
    unary: async (method, signal, timeoutMs, header, input, context) => {
      try {
        return await inner.unary(method, signal, timeoutMs, header, input, context);
      } catch (error) {
        const ask = broker();
        if (
          !needsStepUp(error) ||
          isRead(method.name) ||
          ask === undefined ||
          signal?.aborted === true ||
          !(await ask())
        ) {
          throw error;
        }
        return inner.unary(method, signal, timeoutMs, header, input, context);
      }
    },
    stream: (method, signal, timeoutMs, header, input, context) =>
      inner.stream(method, signal, timeoutMs, header, input, context),
  };
}

/** One ceremony at a time: callers asking while one runs wait for its outcome. */
export function sharedBroker(run: () => Promise<boolean>): StepUpBroker {
  let pending: Promise<boolean> | undefined;
  return () => {
    pending ??= run().finally(() => {
      pending = undefined;
    });
    return pending;
  };
}
