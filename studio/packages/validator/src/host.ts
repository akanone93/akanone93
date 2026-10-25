// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem, ProjectFile, ValidatorRequest, ValidatorResponse } from "./protocol.ts";

/** The part of a Worker the host uses; a stub implements it in tests. */
export interface ValidatorWorkerLike {
  onmessage: ((e: { data: ValidatorResponse }) => void) | null;
  onerror: ((e: { message?: string }) => void) | null;
  postMessage(message: ValidatorRequest): void;
  terminate(): void;
}

/** A request was replaced by a newer one for the same target, or the host was disposed. */
export class ValidatorCancelled extends Error {
  constructor() {
    super("validation superseded");
    this.name = "ValidatorCancelled";
  }
}

interface Pending {
  readonly key: string;
  readonly resolve: (p: readonly Problem[]) => void;
  readonly reject: (e: Error) => void;
}

/**
 * Talks to the validator worker; the main thread never validates. Every request has an id;
 * a newer `load`, or a newer `validatePage` for the same file, cancels the older one.
 */
export class ValidatorHost {
  readonly #worker: ValidatorWorkerLike;
  readonly #pending = new Map<number, Pending>();
  /** Requests wait here until the worker has instantiated the module and said `ready`. */
  #outbox: ValidatorRequest[] | null = [];
  #next = 1;

  constructor(worker: ValidatorWorkerLike) {
    this.#worker = worker;
    worker.onmessage = (e) => this.#receive(e.data);
    worker.onerror = (e) => this.#failAll(new Error(e.message ?? "the validator worker failed"));
  }

  /** Loads the project once; resolves with the project's own problems. */
  load(files: readonly ProjectFile[]): Promise<readonly Problem[]> {
    return this.#send("load", (id) => ({ type: "load", id, files }));
  }

  /** Validates one page of the loaded project. */
  validatePage(file: string, data: string): Promise<readonly Problem[]> {
    return this.#send(`validate:${file}`, (id) => ({ type: "validate", id, file, data }));
  }

  dispose(): void {
    this.#failAll(new ValidatorCancelled());
    this.#worker.terminate();
  }

  #send(key: string, make: (id: number) => ValidatorRequest): Promise<readonly Problem[]> {
    for (const [id, p] of this.#pending) {
      if (p.key === key) {
        this.#pending.delete(id);
        this.#post({ type: "cancel", id });
        p.reject(new ValidatorCancelled());
      }
    }
    const id = this.#next++;
    return new Promise((resolve, reject) => {
      this.#pending.set(id, { key, resolve, reject });
      this.#post(make(id));
    });
  }

  #post(req: ValidatorRequest): void {
    if (this.#outbox === null) {
      this.#worker.postMessage(req);
    } else if (req.type === "cancel") {
      // Not sent yet: drop the request instead of sending it and its cancellation.
      this.#outbox = this.#outbox.filter((q) => q.id !== req.id);
    } else {
      this.#outbox.push(req);
    }
  }

  #receive(msg: ValidatorResponse): void {
    if (msg.type === "ready") {
      const waiting = this.#outbox ?? [];
      this.#outbox = null;
      for (const req of waiting) {
        this.#worker.postMessage(req);
      }
      return;
    }
    const p = this.#pending.get(msg.id);
    if (p === undefined) {
      return; // superseded: its answer is dropped
    }
    this.#pending.delete(msg.id);
    if (msg.type === "result") {
      p.resolve(msg.problems);
    } else {
      p.reject(new Error(msg.message));
    }
  }

  #failAll(e: Error): void {
    const all = [...this.#pending.values()];
    this.#pending.clear();
    for (const p of all) {
      p.reject(e);
    }
  }
}
