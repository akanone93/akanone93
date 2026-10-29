// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { BridgeHandlers, BridgeSession } from "./lsp-bridge.ts";
import {
  type CompletionItem,
  type Diagnostic,
  type Hover,
  LspClient,
  type Position,
  type SignatureHelp,
  type TextEdit,
} from "./lsp-client.ts";
import { publishedDiagnostics } from "./lsp-convert.ts";

/** The slice of `LanguageBridge` a session uses. */
export interface SessionBridge {
  open(sourceId: string, handlers: BridgeHandlers): Promise<BridgeSession>;
  send(message: unknown): Promise<void>;
  close(): void;
}

export type SessionState =
  | { readonly phase: "idle" }
  | { readonly phase: "starting" }
  | { readonly phase: "ready"; readonly sessionSeconds: number }
  | { readonly phase: "closed"; readonly reason: string; readonly detail: string }
  | { readonly phase: "failed"; readonly detail: string };

export interface SessionEvents {
  onState(state: SessionState): void;
  /** The diagnostics of the open document, replacing the earlier ones. */
  onDiagnostics(diagnostics: readonly Diagnostic[]): void;
}

/** Where the editor's document lives below the crate root. */
export const DOCUMENT_PATH = "src/lib.rs";

/** What the editor asks of the language server; Monaco's providers call these. */
export interface LanguageApi {
  completion(position: Position): Promise<CompletionItem[]>;
  hover(position: Position): Promise<Hover | null>;
  signatureHelp(position: Position): Promise<SignatureHelp | null>;
  format(): Promise<TextEdit[]>;
}

const capabilities = {
  general: { positionEncodings: ["utf-16"] },
  textDocument: {
    synchronization: { didSave: false, dynamicRegistration: false },
    completion: { completionItem: { snippetSupport: true, documentationFormat: ["markdown", "plaintext"] } },
    hover: { contentFormat: ["markdown", "plaintext"] },
    signatureHelp: { signatureInformation: { documentationFormat: ["markdown", "plaintext"] } },
    formatting: {},
    publishDiagnostics: {},
  },
};

/**
 * One editing session of the Rust editor (FN-033, ADR-0099): the bridge to rust-analyzer in the build sandbox, the
 * initialisation handshake, and the single document `src/lib.rs` kept in step with the editor's text. The session is
 * the editor's `LanguageApi` while it is ready and answers nothing before.
 */
export class RustSession implements LanguageApi {
  readonly #bridge: SessionBridge;
  readonly #events: SessionEvents;
  readonly #client: LspClient;
  #uri = "";
  #version = 1;
  #ready = false;
  #ended = false;

  constructor(bridge: SessionBridge, events: SessionEvents, requestTimeoutMs?: number) {
    this.#bridge = bridge;
    this.#events = events;
    this.#client = new LspClient(bridge, requestTimeoutMs);
    this.#client.onNotification("textDocument/publishDiagnostics", (params) => {
      const diagnostics = publishedDiagnostics(params, this.#uri);
      if (diagnostics !== undefined) {
        this.#events.onDiagnostics(diagnostics);
      }
    });
  }

  /** Starts the sandbox on an uploaded source, initialises the server and opens the document with `text`. */
  async start(sourceId: string, text: string): Promise<void> {
    this.#events.onState({ phase: "starting" });
    try {
      const session = await this.#bridge.open(sourceId, {
        onMessage: (m) => this.#client.receive(m),
        onClosed: (reason, detail) => this.#end({ phase: "closed", reason, detail }),
      });
      const rootUri = session.rootUri.endsWith("/") ? session.rootUri : `${session.rootUri}/`;
      this.#uri = `${rootUri}${DOCUMENT_PATH}`;
      await this.#client.request("initialize", {
        processId: null,
        rootUri,
        workspaceFolders: [{ uri: rootUri, name: "function" }],
        capabilities,
        clientInfo: { name: "Plux Studio" },
      });
      await this.#client.notify("initialized", {});
      await this.#client.notify("textDocument/didOpen", {
        textDocument: { uri: this.#uri, languageId: "rust", version: this.#version, text },
      });
      this.#ready = true;
      this.#events.onState({ phase: "ready", sessionSeconds: session.sessionSeconds });
    } catch (err) {
      this.#end({ phase: "failed", detail: err instanceof Error ? err.message : "the language server did not start" });
    }
  }

  #end(state: SessionState): void {
    if (this.#ended) {
      return;
    }
    this.#ended = true;
    this.#ready = false;
    this.#client.close();
    this.#bridge.close();
    this.#events.onState(state);
  }

  /** Tells the server the document's new full text. */
  change(text: string): void {
    if (!this.#ready) {
      return;
    }
    this.#version += 1;
    this.#client
      .notify("textDocument/didChange", {
        textDocument: { uri: this.#uri, version: this.#version },
        contentChanges: [{ text }],
      })
      .catch(() => undefined);
  }

  /** Ends the session; the server kills the sandbox when the stream closes. */
  close(): void {
    this.#end({ phase: "closed", reason: "client", detail: "" });
  }

  #position(position: Position) {
    return { textDocument: { uri: this.#uri }, position };
  }

  async completion(position: Position): Promise<CompletionItem[]> {
    if (!this.#ready) {
      return [];
    }
    const result = await this.#client.request<CompletionItem[] | { items: CompletionItem[] } | null>(
      "textDocument/completion",
      this.#position(position),
    );
    return result === null ? [] : Array.isArray(result) ? result : result.items;
  }

  hover(position: Position): Promise<Hover | null> {
    return this.#ready
      ? this.#client.request<Hover | null>("textDocument/hover", this.#position(position))
      : Promise.resolve(null);
  }

  signatureHelp(position: Position): Promise<SignatureHelp | null> {
    return this.#ready
      ? this.#client.request<SignatureHelp | null>("textDocument/signatureHelp", this.#position(position))
      : Promise.resolve(null);
  }

  async format(): Promise<TextEdit[]> {
    if (!this.#ready) {
      return [];
    }
    const edits = await this.#client.request<TextEdit[] | null>("textDocument/formatting", {
      textDocument: { uri: this.#uri },
      options: { tabSize: 4, insertSpaces: true },
    });
    return edits ?? [];
  }
}
