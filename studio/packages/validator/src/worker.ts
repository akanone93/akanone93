// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  parseProblems,
  parseValue,
  type ValidatorEngine,
  type ValidatorRequest,
  type ValidatorResponse,
} from "./protocol.ts";

/** The part of a worker's global scope the server uses. */
export interface ValidatorScope {
  onmessage: ((e: { data: ValidatorRequest }) => void) | null;
  postMessage(message: ValidatorResponse): void;
}

/**
 * Serves the validator protocol. Requests queue and run one macrotask apart, so a `cancel`
 * that arrives while others wait removes them before they run (a running validation is not
 * interrupted; WebAssembly here is single threaded).
 */
export function serveValidatorWorker(
  scope: ValidatorScope,
  engine: ValidatorEngine,
  yieldTurn: () => Promise<void> = () => new Promise((r) => setTimeout(r, 0)),
): void {
  const queue: Exclude<ValidatorRequest, { type: "cancel" }>[] = [];
  let draining = false;

  const run = (req: (typeof queue)[number]): void => {
    try {
      switch (req.type) {
        case "load":
          scope.postMessage({ type: "result", id: req.id, problems: parseProblems(engine.load(req.files)) });
          break;
        case "validate":
          scope.postMessage({
            type: "result",
            id: req.id,
            problems: parseProblems(engine.validatePage(req.file, req.data)),
          });
          break;
        case "pxl":
          scope.postMessage({
            type: "value",
            id: req.id,
            value: parseValue(engine.pxl(req.op, req.scope, req.src, req.offset)),
          });
          break;
        case "sites":
          scope.postMessage({ type: "value", id: req.id, value: parseValue(engine.pxlSites(req.file, req.data)) });
          break;
      }
    } catch (e) {
      scope.postMessage({ type: "failure", id: req.id, message: e instanceof Error ? e.message : String(e) });
    }
  };

  const drain = async (): Promise<void> => {
    if (draining) {
      return;
    }
    draining = true;
    await yieldTurn();
    for (let req = queue.shift(); req !== undefined; req = queue.shift()) {
      run(req);
      await yieldTurn();
    }
    draining = false;
  };

  scope.onmessage = (e) => {
    const req = e.data;
    if (req.type === "cancel") {
      const at = queue.findIndex((q) => q.id === req.id);
      if (at >= 0) {
        queue.splice(at, 1);
      }
      return;
    }
    queue.push(req);
    void drain();
  };
  scope.postMessage({ type: "ready" });
}
