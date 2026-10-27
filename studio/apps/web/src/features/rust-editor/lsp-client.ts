// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The few Language Server Protocol 3.17 shapes the Rust editor uses (FN-033), and the JSON-RPC layer under them. Only
 * the fields the editor reads are typed; rust-analyzer sends more, and the rest is ignored.
 */

export interface Position {
  /** Zero-based line. */
  line: number;
  /** Zero-based offset in UTF-16 code units, the unit Monaco's columns use too. */
  character: number;
}

export interface Range {
  start: Position;
  end: Position;
}

/** `1` error, `2` warning, `3` information, `4` hint. */
export type DiagnosticSeverity = 1 | 2 | 3 | 4;

export interface Diagnostic {
  range: Range;
  severity?: DiagnosticSeverity;
  message: string;
  code?: string | number;
}

export interface TextEdit {
  range: Range;
  newText: string;
}

export type MarkupContent = string | { kind?: string; language?: string; value: string };

export interface CompletionItem {
  label: string;
  kind?: number;
  detail?: string;
  documentation?: MarkupContent;
  insertText?: string;
  /** `2` marks the insert text as a snippet. */
  insertTextFormat?: 1 | 2;
  textEdit?: TextEdit | { newText: string; insert: Range; replace: Range };
  sortText?: string;
  filterText?: string;
}

export interface Hover {
  contents: MarkupContent | MarkupContent[];
  range?: Range;
}

export interface ParameterInformation {
  label: string | [number, number];
  documentation?: MarkupContent;
}

export interface SignatureInformation {
  label: string;
  documentation?: MarkupContent;
  parameters?: ParameterInformation[];
}

export interface SignatureHelp {
  signatures: SignatureInformation[];
  activeSignature?: number;
  activeParameter?: number;
}

export interface PublishDiagnostics {
  uri: string;
  diagnostics: Diagnostic[];
}

/** What the client sends through: the language-server bridge satisfies it. */
export interface RpcTransport {
  send(message: unknown): Promise<void>;
}

/** JSON-RPC error codes the client answers with. */
const METHOD_NOT_FOUND = -32601;
const REQUEST_CANCELLED = -32800;

export class RpcError extends Error {
  readonly code: number;
  constructor(code: number, message: string) {
    super(message);
    this.name = "RpcError";
    this.code = code;
  }
}

interface Pending {
  resolve(value: unknown): void;
  reject(reason: unknown): void;
  timer: ReturnType<typeof setTimeout>;
}

type Message = {
  id?: number | string;
  method?: string;
  params?: unknown;
  result?: unknown;
  error?: { code: number; message: string };
};

/** Milliseconds a request waits for its answer before it fails; rust-analyzer's cold start is the slow case. */
export const REQUEST_TIMEOUT_MS = 30_000;

/**
 * A JSON-RPC 2.0 peer over the bridge. It correlates answers with requests, hands notifications to their handlers and
 * answers the requests rust-analyzer makes of its client: configuration is empty (the server's defaults hold), progress
 * and dynamic registrations are accepted, anything else is "method not found".
 */
export class LspClient {
  readonly #transport: RpcTransport;
  readonly #timeoutMs: number;
  readonly #pending = new Map<number, Pending>();
  readonly #handlers = new Map<string, (params: unknown) => void>();
  #next = 1;
  #closed = false;

  constructor(transport: RpcTransport, timeoutMs: number = REQUEST_TIMEOUT_MS) {
    this.#transport = transport;
    this.#timeoutMs = timeoutMs;
  }

  /** Registers the one handler of a server notification. */
  onNotification(method: string, handler: (params: unknown) => void): void {
    this.#handlers.set(method, handler);
  }

  /** Sends a request and resolves with its result; rejects with `RpcError` when the server refuses or the client closes. */
  request<T>(method: string, params: unknown): Promise<T> {
    if (this.#closed) {
      return Promise.reject(new RpcError(REQUEST_CANCELLED, "the language-server session is closed"));
    }
    const id = this.#next++;
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.#pending.delete(id);
        reject(new RpcError(REQUEST_CANCELLED, `${method} timed out`));
      }, this.#timeoutMs);
      this.#pending.set(id, { resolve: resolve as (v: unknown) => void, reject, timer });
      this.#transport.send({ jsonrpc: "2.0", id, method, params }).catch((err: unknown) => {
        clearTimeout(timer);
        this.#pending.delete(id);
        reject(err);
      });
    });
  }

  notify(method: string, params: unknown): Promise<void> {
    return this.#closed ? Promise.resolve() : this.#transport.send({ jsonrpc: "2.0", method, params });
  }

  /** Takes one message from the server, as the bridge decoded it. Malformed messages are dropped. */
  receive(raw: string): void {
    let message: Message;
    try {
      message = JSON.parse(raw) as Message;
    } catch {
      return;
    }
    if (typeof message !== "object" || message === null) {
      return;
    }
    if (message.method === undefined) {
      this.#settle(message);
    } else if (message.id === undefined) {
      this.#handlers.get(message.method)?.(message.params);
    } else {
      this.#answer(message.id, message.method, message.params);
    }
  }

  #settle(message: Message): void {
    if (typeof message.id !== "number") {
      return;
    }
    const pending = this.#pending.get(message.id);
    if (pending === undefined) {
      return;
    }
    this.#pending.delete(message.id);
    clearTimeout(pending.timer);
    if (message.error !== undefined) {
      pending.reject(new RpcError(message.error.code, message.error.message));
    } else {
      pending.resolve(message.result ?? null);
    }
  }

  #answer(id: number | string, method: string, params: unknown): void {
    let reply: Record<string, unknown>;
    switch (method) {
      case "workspace/configuration": {
        const items = (params as { items?: unknown[] } | undefined)?.items ?? [];
        reply = { result: items.map(() => null) };
        break;
      }
      case "window/workDoneProgress/create":
      case "client/registerCapability":
      case "client/unregisterCapability":
      case "workspace/semanticTokens/refresh":
      case "workspace/inlayHint/refresh":
      case "workspace/diagnostic/refresh":
        reply = { result: null };
        break;
      default:
        reply = { error: { code: METHOD_NOT_FOUND, message: `${method} is not supported` } };
    }
    // A failed send ends the bridge session, which closes the client through the bridge's own handler.
    this.#transport.send({ jsonrpc: "2.0", id, ...reply }).catch(() => undefined);
  }

  /** Fails every open request; later requests fail at once. */
  close(): void {
    this.#closed = true;
    for (const [id, pending] of this.#pending) {
      clearTimeout(pending.timer);
      pending.reject(new RpcError(REQUEST_CANCELLED, "the language-server session closed"));
      this.#pending.delete(id);
    }
  }
}
