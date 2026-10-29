// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** The slice of `LanguageServerService` the bridge uses; the generated Connect client satisfies it. */
export interface BridgeClient {
  open(
    request: { sourceId: string },
    options: { signal: AbortSignal },
  ): AsyncIterable<{
    event:
      | { case: "opened"; value: { sessionId: string; rootUri: string; sessionSeconds: number } }
      | { case: "message"; value: Uint8Array }
      | { case: "closed"; value: { reason: string; detail: string } }
      | { case: undefined; value?: undefined };
  }>;
  send(request: { sessionId: string; sequence: bigint; message: Uint8Array }): Promise<unknown>;
}

export interface BridgeHandlers {
  /** One JSON-RPC message of the language server, decoded from UTF-8. */
  onMessage(message: string): void;
  /** The session ended; `reason` is "client", "time", "exited" or "failed". */
  onClosed(reason: string, detail: string): void;
}

export interface BridgeSession {
  readonly sessionId: string;
  /** The root of the function's crate, such as `file:///workspace/functions/bank.fee/`; documents live below it. */
  readonly rootUri: string;
  /** Seconds the session may run, from `lsp.sessionMinutes`. */
  readonly sessionSeconds: number;
}

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/**
 * One editor session over the `LanguageServerService` stream (FN-033, ADR-0099). Browsers cannot stream requests, so
 * messages go out as unary calls; the server applies them by sequence number, so the bridge sends them strictly one at a
 * time and in the order given. Nothing is retried: a failed send ends the session, and the editor opens a new one.
 */
export class LanguageBridge {
  readonly #client: BridgeClient;
  readonly #abort = new AbortController();
  #session: BridgeSession | undefined;
  #sequence = 1n;
  #queue: Promise<void> = Promise.resolve();
  #ended = false;

  constructor(client: BridgeClient) {
    this.#client = client;
  }

  /** Opens the session on an uploaded source; resolves once the server has started the sandbox. */
  async open(sourceId: string, handlers: BridgeHandlers): Promise<BridgeSession> {
    const events = this.#client.open({ sourceId }, { signal: this.#abort.signal })[Symbol.asyncIterator]();
    const first = await events.next();
    const event = first.value?.event;
    if (first.done || event?.case !== "opened") {
      throw new Error("the language server did not start");
    }
    const session = {
      sessionId: event.value.sessionId,
      rootUri: event.value.rootUri,
      sessionSeconds: event.value.sessionSeconds,
    };
    this.#session = session;
    void this.#pump(events, handlers);
    return session;
  }

  async #pump(
    events: AsyncIterator<{ event: { case: string | undefined; value?: unknown } }>,
    handlers: BridgeHandlers,
  ) {
    let reason = "client";
    let detail = "";
    try {
      for (let step = await events.next(); !step.done; step = await events.next()) {
        const event = step.value.event;
        if (event.case === "message") {
          handlers.onMessage(decoder.decode(event.value as Uint8Array));
        } else if (event.case === "closed") {
          ({ reason, detail } = event.value as { reason: string; detail: string });
        }
      }
    } catch (err) {
      if (!this.#abort.signal.aborted) {
        reason = "failed";
        detail = err instanceof Error ? err.message : "the stream broke";
      }
    }
    this.#ended = true;
    handlers.onClosed(reason, detail);
  }

  /** Sends one JSON-RPC message; resolves when the server accepted it, rejects if the session is over or refused it. */
  send(message: unknown): Promise<void> {
    const session = this.#session;
    if (session === undefined || this.#ended) {
      return Promise.reject(new Error("the language-server session is not open"));
    }
    const body = encoder.encode(JSON.stringify(message));
    const sequence = this.#sequence;
    this.#sequence += 1n;
    const next = this.#queue.then(async () => {
      if (this.#ended) {
        throw new Error("the language-server session ended before this message was sent");
      }
      await this.#client.send({ sessionId: session.sessionId, sequence, message: body });
    });
    // A failure ends the chain: later messages would arrive out of sequence, so they fail with it.
    this.#queue = next.catch(() => {
      this.#ended = true;
      this.#abort.abort();
    });
    return next;
  }

  /** Ends the session; the server kills the sandbox when the stream closes. */
  close(): void {
    this.#ended = true;
    this.#abort.abort();
  }
}
