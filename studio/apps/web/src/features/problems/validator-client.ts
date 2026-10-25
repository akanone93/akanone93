// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Problem, type ProjectFile, ValidatorHost, type ValidatorWorkerLike } from "@plux/validator";
import { validatorWorkerScriptURL } from "../../lib/trusted-worker.ts";
import validatorWorkerURL from "./validator-worker.ts?worker&url";

/** What C2 needs of the validator; `ValidatorHost` implements it. */
export interface ValidatorClient {
  load(files: readonly ProjectFile[]): Promise<readonly Problem[]>;
  validatePage(file: string, data: string): Promise<readonly Problem[]>;
  dispose(): void;
}

export type ValidatorFactory = () => ValidatorClient;

/** The validator in its own worker, started through the Trusted Types helper. */
export const browserValidator: ValidatorFactory = () =>
  new ValidatorHost(
    new Worker(validatorWorkerScriptURL(validatorWorkerURL, window, import.meta.env.DEV), {
      type: "module",
    }) as unknown as ValidatorWorkerLike,
  );
