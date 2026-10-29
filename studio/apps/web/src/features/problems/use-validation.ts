// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Problem, PxlService, type PxlSites, parseSites } from "@plux/validator";
import { useEffect, useState } from "react";
import { projectFiles } from "./project-files.ts";
import type { ValidatorClient, ValidatorFactory } from "./validator-client.ts";

/** The PXL language service of the loaded project (PXL-007, S5.2): the service and the scope of each expression site. */
export interface PxlLanguage {
  readonly service: PxlService;
  /** The sites of a page, with each one's scope; the page must carry an expression where the field is. */
  readonly sites: (file: string, data: string) => Promise<PxlSites>;
}

export type Validation =
  | { readonly state: "idle" }
  | { readonly state: "running" }
  | {
      readonly state: "done";
      readonly problems: readonly Problem[];
      /** Validates one page of the loaded project with an edit applied (F9); the Properties panel uses it. */
      readonly validatePage: (file: string, data: string) => Promise<readonly Problem[]>;
      /** Undefined when the validator has no language service. */
      readonly language: PxlLanguage | undefined;
      /** The Go module's pure tools (importers, migration preview; L-12); undefined in a stub without them. */
      readonly tool: ValidatorClient["tool"];
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

function languageOf(validator: ValidatorClient): PxlLanguage | undefined {
  const { pxl, pxlSites } = validator;
  if (pxl === undefined || pxlSites === undefined) {
    return undefined;
  }
  return {
    service: new PxlService({ pxl: (op, scope, src, offset) => pxl.call(validator, op, scope, src, offset) }),
    sites: async (file, data) => parseSites(await pxlSites.call(validator, file, data)),
  };
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
            language: languageOf(validator),
            tool:
              validator.tool === undefined
                ? undefined
                : (name, args) => validator.tool?.(name, args) ?? Promise.resolve(null),
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
