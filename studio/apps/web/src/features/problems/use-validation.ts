// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  type Problem,
  type ProjectFile,
  PxlService,
  type PxlSites,
  parseSites,
  ValidatorCancelled,
} from "@plux/validator";
import { useEffect, useRef, useState } from "react";
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
      /**
       * Evaluates a batch of page expressions with design-time values for the canvas (ADR-0112); the same function
       * for as long as the worker lives, so the canvas keeps its answers across checks. Absent in a stub without it.
       */
      readonly evaluate?: (request: string) => Promise<unknown>;
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

/** The validator's design-time evaluation (ADR-0112), made once per worker; none when it has none. */
function evaluatorOf(
  validator: ValidatorClient,
  made: WeakMap<ValidatorClient, (request: string) => Promise<unknown>>,
): { readonly evaluate?: (request: string) => Promise<unknown> } {
  const { pxlEval } = validator;
  if (pxlEval === undefined) {
    return {};
  }
  let evaluate = made.get(validator);
  if (evaluate === undefined) {
    evaluate = (request) => pxlEval.call(validator, request);
    made.set(validator, evaluate);
  }
  return { evaluate };
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

/** Whether two loads hold the same files, path for path and byte for byte. */
function sameFiles(a: readonly ProjectFile[] | undefined, b: readonly ProjectFile[]): boolean {
  if (a === undefined || a.length !== b.length) {
    return false;
  }
  return a.every((f, i) => f.path === b[i]?.path && f.data === b[i]?.data);
}

/**
 * Loads the project into the validator when the documents and the asset list arrive and validates it project-wide
 * (validate on open). One validator worker serves the canvas for its lifetime: a change of documents (an edit, or a
 * refetch when the window regains focus) loads them into the same worker, and only when their content changed. The
 * last finished check stays the result, its page check usable, until the next one finishes; before, each refetch
 * started a new worker and the JSON editor and the Properties panel found the validator "not ready" meanwhile (#252).
 */
export function useValidation(
  createValidator: ValidatorFactory,
  app: Documents | undefined,
  plugin: Documents | undefined,
  /** The names of the app's stored asset files; validation waits for them. */
  assetFiles: readonly string[] | undefined = NO_ASSETS,
): Validation {
  const [result, setResult] = useState<Validation>({ state: "idle" });
  const [validator, setValidator] = useState<ValidatorClient | undefined>(undefined);
  // One evaluation function per worker, so the canvas keeps its answers while the worker lives.
  const evaluators = useRef(new WeakMap<ValidatorClient, (request: string) => Promise<unknown>>());
  // The files the current worker holds, and the worker itself: a worker let go is never loaded again.
  const loaded = useRef<readonly ProjectFile[] | undefined>(undefined);
  const current = useRef<ValidatorClient | undefined>(undefined);
  // The newest load: an older one that finishes late, or one of a validator let go, is dropped.
  const generation = useRef(0);
  // A load that failed (a worker that died, say) starts a new worker once for the same files, then reports the failure.
  const [restarts, setRestarts] = useState(0);
  const retried = useRef<readonly ProjectFile[] | undefined>(undefined);
  // biome-ignore lint/correctness/useExhaustiveDependencies: a restart asks for a new worker by its count alone
  useEffect(() => {
    const created = createValidator();
    loaded.current = undefined;
    current.current = created;
    setValidator(created);
    return () => {
      generation.current += 1;
      created.dispose();
    };
  }, [createValidator, restarts]);
  useEffect(() => {
    if (
      validator === undefined ||
      validator !== current.current ||
      app === undefined ||
      plugin === undefined ||
      assetFiles === undefined
    ) {
      return;
    }
    const files = [...projectFiles(wholeProject(app, plugin)), ...assetStandIns(assetFiles)];
    if (sameFiles(loaded.current, files)) {
      return;
    }
    loaded.current = files;
    generation.current += 1;
    const mine = generation.current;
    setResult((prev) => (prev.state === "done" ? prev : { state: "running" }));
    validator.load(files).then(
      (problems) => {
        if (mine === generation.current) {
          setResult({
            state: "done",
            problems: inFiles(problems, plugin),
            validatePage: (file, data) => validator.validatePage(file, data),
            language: languageOf(validator),
            ...evaluatorOf(validator, evaluators.current),
            tool:
              validator.tool === undefined
                ? undefined
                : (name, args) => validator.tool?.(name, args) ?? Promise.resolve(null),
          });
        }
      },
      (e: unknown) => {
        // A newer load replaced this one; its own result comes.
        if (mine !== generation.current || e instanceof ValidatorCancelled) {
          return;
        }
        if (!sameFiles(retried.current, files)) {
          retried.current = files;
          loaded.current = undefined;
          setRestarts((n) => n + 1);
          return;
        }
        // These files failed on a new worker too: they are not loaded again until they change.
        setResult({ state: "failed" });
      },
    );
  }, [validator, app, plugin, assetFiles]);
  return result;
}
