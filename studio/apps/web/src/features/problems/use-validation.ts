// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem } from "@plux/validator";
import { useEffect, useState } from "react";
import { projectFiles } from "./project-files.ts";
import type { ValidatorFactory } from "./validator-client.ts";

export type Validation =
  | { readonly state: "idle" }
  | { readonly state: "running" }
  | {
      readonly state: "done";
      readonly problems: readonly Problem[];
      /** Validates one page of the loaded project with an edit applied (F9); the Properties panel uses it. */
      readonly validatePage: (file: string, data: string) => Promise<readonly Problem[]>;
    }
  | { readonly state: "failed" };

type Documents = Parameters<typeof projectFiles>[0];

/** The whole project's files: the app's documents plus the plugin's own, by path. */
export function wholeProject(app: Documents, plugin: Documents): Documents {
  const byPath = new Map([...app, ...plugin].map((d) => [d.path, d]));
  return [...byPath.values()];
}

/** Only the problems in the open plugin's own files. */
export function inFiles(problems: readonly Problem[], plugin: Documents): Problem[] {
  const own = new Set(plugin.map((d) => d.path));
  return problems.filter((p) => own.has(p.file));
}

/** Loads the project into the validator when the documents arrive and validates it project-wide (validate on open). */
export function useValidation(
  createValidator: ValidatorFactory,
  app: Documents | undefined,
  plugin: Documents | undefined,
): Validation {
  const [result, setResult] = useState<Validation>({ state: "idle" });
  useEffect(() => {
    if (app === undefined || plugin === undefined) {
      return undefined;
    }
    setResult({ state: "running" });
    const validator = createValidator();
    let live = true;
    validator
      .load(projectFiles(wholeProject(app, plugin)))
      .then(
        (problems) =>
          live &&
          setResult({
            state: "done",
            problems: inFiles(problems, plugin),
            validatePage: (file, data) => validator.validatePage(file, data),
          }),
      )
      .catch(() => live && setResult({ state: "failed" }));
    return () => {
      live = false;
      validator.dispose();
    };
  }, [createValidator, app, plugin]);
  return result;
}
