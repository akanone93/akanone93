// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The one Trusted Types policy of Studio (`plux-canvas-worker`, allowed by the
 * BFF's CSP). Studio requires Trusted Types for scripts, so a worker can only
 * start from a `TrustedScriptURL`; this policy approves the canvas worker's
 * own hashed build asset on this origin and refuses every other address.
 */
export const CANVAS_WORKER_POLICY = "plux-canvas-worker";

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

/**
 * The canvas worker's address as the browser needs it: a `TrustedScriptURL`
 * where Trusted Types exist, the plain string elsewhere. Throws for any other
 * address. The policy is created per call (the CSP allows duplicates), so no
 * state is kept.
 */
export function canvasWorkerScriptURL(
  url: string,
  scope: { readonly location: { readonly origin: string }; readonly trustedTypes?: TrustedTypesLike },
  dev: boolean,
): string | URL {
  const check = (input: string): string => {
    if (!isCanvasWorkerURL(input, scope.location.origin, dev)) {
      throw new TypeError("not the canvas worker");
    }
    return input;
  };
  if (scope.trustedTypes === undefined) {
    return check(url);
  }
  const policy = scope.trustedTypes.createPolicy(CANVAS_WORKER_POLICY, { createScriptURL: check });
  // A TrustedScriptURL is accepted wherever the Worker constructor takes a URL.
  return policy.createScriptURL(url) as string;
}
