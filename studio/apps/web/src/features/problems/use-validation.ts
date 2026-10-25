// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem } from "@plux/validator";
import { useEffect, useState } from "react";
import { projectFiles } from "./project-files.ts";
import type { ValidatorFactory } from "./validator-client.ts";

export type Validation =
  | { readonly state: "idle" }
  | { readonly state: "running" }
  | { readonly state: "done"; readonly problems: readonly Problem[] }
  | { readonly state: "failed" };

type Documents = Parameters<typeof projectFiles>[0];

/** Loads the project into the validator when the documents arrive and validates it project-wide (validate on open). */
export function useValidation(createValidator: ValidatorFactory, documents: Documents | undefined): Validation {
  const [result, setResult] = useState<Validation>({ state: "idle" });
  useEffect(() => {
    if (documents === undefined) {
      return undefined;
    }
    setResult({ state: "running" });
    const validator = createValidator();
    let live = true;
    validator
      .load(projectFiles(documents))
      .then((problems) => live && setResult({ state: "done", problems }))
      .catch(() => live && setResult({ state: "failed" }));
    return () => {
      live = false;
      validator.dispose();
    };
  }, [createValidator, documents]);
  return result;
}
