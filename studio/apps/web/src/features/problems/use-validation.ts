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

/**
 * The problems the open plugin's canvas shows: those in its own files, and those in the app's own documents
 * (`app.json`, the theme, the translations: everything outside `plugins/`), which every plugin depends on and which
 * only the whole-project check on opening finds, such as a missing or unused translation (PLX-1606, PLX-1608).
 * Another plugin's problems are that plugin's.
 */
export function inFiles(problems: readonly Problem[], plugin: Documents): Problem[] {
  const own = new Set(plugin.map((d) => d.path));
  return problems.filter((p) => own.has(p.file) || (p.file !== "" && !p.file.startsWith("plugins/")));
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

/**
 * The asset files the server stores, as empty stand-ins under `assets/`: the browser validator checks that each file
 * `assets/index.json` lists exists, and was never given them, so it called every uploaded image missing (PLX-1020). The
 * server checks the files' bytes (size, executable content) on upload and on publish; the stand-ins only say which
 * names exist, so a file that is listed but not stored is still reported.
 */
export function assetStandIns(names: readonly string[]): { readonly path: string; readonly data: string }[] {
  return names.map((name) => ({ path: `assets/${name}`, data: "" }));
}

/** One empty list for callers without assets: a fresh `[]` each render would restart the validation each render. */
const NO_ASSETS: readonly string[] = [];

/**
 * Loads the project into the validator when the documents and the asset list arrive and validates it project-wide
 * (validate on open).
 */
export function useValidation(
  createValidator: ValidatorFactory,
  app: Documents | undefined,
  plugin: Documents | undefined,
  /** The names of the app's stored asset files; validation waits for them. */
  assetFiles: readonly string[] | undefined = NO_ASSETS,
): Validation {
  const [result, setResult] = useState<Validation>({ state: "idle" });
  useEffect(() => {
    if (app === undefined || plugin === undefined || assetFiles === undefined) {
      return undefined;
    }
    setResult({ state: "running" });
    const validator = createValidator();
    let live = true;
    validator
      .load([...projectFiles(wholeProject(app, plugin)), ...assetStandIns(assetFiles)])
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
  }, [createValidator, app, plugin, assetFiles]);
  return result;
}
