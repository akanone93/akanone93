// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Problem, type ProjectFile, type PxlOp, ValidatorHost, type ValidatorWorkerLike } from "@plux/validator";
import { validatorWorkerScriptURL } from "../../lib/trusted-worker.ts";
import validatorWorkerURL from "./validator-worker.ts?worker&url";

/** What C2 needs of the validator; `ValidatorHost` implements it. */
export interface ValidatorClient {
  load(files: readonly ProjectFile[]): Promise<readonly Problem[]>;
  validatePage(file: string, data: string): Promise<readonly Problem[]>;
  /** One request of the PXL language service (PXL-007); absent in a stub without the module. */
  pxl?(op: PxlOp, scope: string, src: string, offset: number): Promise<unknown>;
  /** The expression sites of a page of the loaded project with the scope each reads. */
  pxlSites?(file: string, data: string): Promise<unknown>;
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
