// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The one Trusted Types policy of Studio (`plux-worker`, allowed by the BFF's
 * CSP). Studio requires Trusted Types for scripts, so a worker can only start
 * from a `TrustedScriptURL`; each use of this policy approves exactly one of
 * Studio's own workers — the canvas, the validator, the lock heartbeat or one
 * of the two Monaco workers (S5.1) — at its hashed build
 * asset on this origin, and refuses every other address.
 */
export const WORKER_POLICY = "plux-worker";

/** The built canvas worker: `/assets/canvas-worker-<hash>.js`. */
const BUILT_WORKER = /^\/assets\/canvas-worker-[A-Za-z0-9_-]+\.js$/;

interface TrustedTypesLike {
  createPolicy(
    name: string,
    rules: { createScriptURL(input: string): string },
  ): {
    createScriptURL(input: string): unknown;
  };
}

/** The built validator worker: `/assets/validator-worker-<hash>.js` (S2.1). */
const BUILT_LOCK_WORKER = /^\/assets\/lock-worker-[A-Za-z0-9_-]+\.js$/;

/** Whether a URL is the lock heartbeat worker's (F1.2): same origin, the built asset or, in development, the source. */
export function isLockWorkerURL(url: string, origin: string, dev: boolean): boolean {
  let u: URL;
  try {
    u = new URL(url, origin);
  } catch {
    return false;
  }
  if (u.origin !== origin || u.username !== "" || u.password !== "" || u.hash !== "") {
    return false;
  }
  return dev ? u.pathname.endsWith("/lock-worker.ts") : BUILT_LOCK_WORKER.test(u.pathname) && u.search === "";
}

const BUILT_VALIDATOR_WORKER = /^\/assets\/validator-worker-[A-Za-z0-9_-]+\.js$/;

/** Whether `url` is the canvas worker's address; `dev` also admits the dev server's module path. */
export function isCanvasWorkerURL(url: string, origin: string, dev: boolean): boolean {
  let u: URL;
  try {
    u = new URL(url, origin);
  } catch {
    return false;
  }
  if (u.origin !== origin || u.username !== "" || u.password !== "" || u.hash !== "") {
    return false;
  }
  return dev ? u.pathname.endsWith("/canvas-worker.ts") : BUILT_WORKER.test(u.pathname) && u.search === "";
}

/** Whether `url` is the validator worker's address; `dev` also admits the dev server's module path. */
export function isValidatorWorkerURL(url: string, origin: string, dev: boolean): boolean {
  let u: URL;
  try {
    u = new URL(url, origin);
  } catch {
    return false;
  }
  if (u.origin !== origin || u.username !== "" || u.password !== "" || u.hash !== "") {
    return false;
  }
  return dev ? u.pathname.endsWith("/validator-worker.ts") : BUILT_VALIDATOR_WORKER.test(u.pathname) && u.search === "";
}

/** The Monaco workers of the JSON editor (S5.1): the editor worker and the JSON language worker. */
export type MonacoWorker = "editor" | "json";

const BUILT_MONACO_WORKER: Readonly<Record<MonacoWorker, RegExp>> = {
  editor: /^\/assets\/monaco-editor-worker-[A-Za-z0-9_-]+\.js$/,
  json: /^\/assets\/monaco-json-worker-[A-Za-z0-9_-]+\.js$/,
};

/** Whether `url` is that Monaco worker's address; `dev` also admits the dev server's module path. */
export function isMonacoWorkerURL(which: MonacoWorker, url: string, origin: string, dev: boolean): boolean {
  let u: URL;
  try {
    u = new URL(url, origin);
  } catch {
    return false;
  }
  if (u.origin !== origin || u.username !== "" || u.password !== "" || u.hash !== "") {
    return false;
  }
  return dev
    ? u.pathname.endsWith(`/monaco-${which}-worker.ts`)
    : BUILT_MONACO_WORKER[which].test(u.pathname) && u.search === "";
}

type Scope = { readonly location: { readonly origin: string }; readonly trustedTypes?: TrustedTypesLike };

/** Approves `url` with the one policy where Trusted Types exist; throws when `accept` refuses it. */
function workerScriptURL(url: string, scope: Scope, accept: (url: string, origin: string) => boolean): string | URL {
  const check = (input: string): string => {
    if (!accept(input, scope.location.origin)) {
      throw new TypeError("not an approved worker");
    }
    return input;
  };
  if (scope.trustedTypes === undefined) {
    return check(url);
  }
  const policy = scope.trustedTypes.createPolicy(WORKER_POLICY, { createScriptURL: check });
  // A TrustedScriptURL is accepted wherever the Worker constructor takes a URL.
  return policy.createScriptURL(url) as string;
}

/**
 * The canvas worker's address as the browser needs it: a `TrustedScriptURL`
 * where Trusted Types exist, the plain string elsewhere. Throws for any other
 * address. The policy is created per call (the CSP allows duplicates), so no
 * state is kept.
 */
export function canvasWorkerScriptURL(url: string, scope: Scope, dev: boolean): string | URL {
  return workerScriptURL(url, scope, (u, origin) => isCanvasWorkerURL(u, origin, dev));
}

/** The validator worker's address, through the same policy as the canvas worker. */
export function validatorWorkerScriptURL(url: string, scope: Scope, dev: boolean): string | URL {
  return workerScriptURL(url, scope, (u, origin) => isValidatorWorkerURL(u, origin, dev));
}

/** The lock heartbeat worker's address, through the same policy as the canvas worker. */
export function lockWorkerScriptURL(url: string, scope: Scope, dev: boolean): string | URL {
  return workerScriptURL(url, scope, (u, origin) => isLockWorkerURL(u, origin, dev));
}

/** A Monaco worker's address, through the same policy as the canvas worker. */
export function monacoWorkerScriptURL(which: MonacoWorker, url: string, scope: Scope, dev: boolean): string | URL {
  return workerScriptURL(url, scope, (u, origin) => isMonacoWorkerURL(which, u, origin, dev));
}
