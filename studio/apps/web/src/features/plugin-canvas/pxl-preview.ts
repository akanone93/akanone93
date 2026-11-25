// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { MockState } from "../preview/switches.ts";
import { type DesignScope, readsItem, type SourceMocks } from "./mock-bindings.ts";

/**
 * Design-time evaluation of PXL on the canvas (STU-005, ADR-0112): an expression the canvas's own readers do not
 * resolve — arithmetic such as `40.0 * device.textScale`, a call such as `format.number(item)` or `t(…)` — is
 * evaluated by the PXL engine Studio ships, in the validator's worker, with the page's design-time values: each
 * expression as the page's check compiled it, in its own scope. The canvas never waits for it: a pass over the screens
 * reads what was evaluated before, leaves the rest as it is and asks for it afterwards, in a batch per page; each
 * page's answers redraw only its screens. Answers are kept by page version, values and expression, so an unchanged screen asks
 * nothing again.
 */

/** Sends one batch, as JSON, to the engine; resolves with its JSON answer. */
export type EvalTransport = (request: string) => Promise<unknown>;

/** A page as the canvas draws it: its file and its document, unsaved edits included. */
export interface PageSource {
  readonly file: string;
  readonly doc: object;
}

/** What one pass over the screens shares: the locale and its messages. */
export interface PassContext {
  readonly locale: string;
  readonly messages: Readonly<Record<string, string>>;
}

/** The data sources a screen reads (the plugin's and its page's own), with the mock state chosen for each. */
export interface ScreenData {
  readonly mocks: SourceMocks;
  readonly states: Readonly<Record<string, MockState>>;
}

/** A screen's `data` root, with a key for its content. */
interface DataRoot {
  readonly roots: Record<string, unknown>;
  readonly key: string;
}

/** A design scope's evaluator of expressions: a value, or undefined until the engine answered or when it failed. */
export type Evaluate = (expr: string, scope: DesignScope) => unknown;

/** An answer kept: the value, or that the expression does not evaluate. */
type Answer = { readonly value: unknown } | typeof FAILED;
const FAILED = Symbol("failed");

/** How many answers are kept; past it they are dropped and asked for again as needed. */
const ANSWERS_KEPT = 20_000;

/**
 * The roots of expressions the canvas does not ask the engine for as a page's: a component's own (`props`,
 * `component`), compiled with the component and asked for as its (`forComponent`), and an action's (`event`, `steps`),
 * which no widget draws.
 */
const NOT_ASKED = /(?<![.\w$])(props|component|event|steps)\b/;

/** Inside a component, its own roots are asked for; an action's still are not. */
const NOT_ASKED_IN_COMPONENT = /(?<![.\w$])(event|steps)\b/;

/** A request waiting for the next batch. */
interface Ask {
  readonly screen: string;
  readonly page: PageSource;
  readonly values: ValuesSet;
  readonly src: string;
}

/** A set of root values: a page's, or a list item's over its page's. */
interface ValuesSet {
  readonly key: string;
  readonly roots: Record<string, unknown>;
  readonly base?: ValuesSet;
}

/** 53 bits of a string's hash (cyrb53): a short key for long JSON. */
export function hash53(text: string): string {
  let h1 = 0xdeadbeef;
  let h2 = 0x41c6ce57;
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i);
    h1 = Math.imul(h1 ^ c, 2654435761);
    h2 = Math.imul(h2 ^ c, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(36);
}

const isObj = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/** What a source holds when it is `empty`: no items, no fields, no text. */
function emptyOf(mock: unknown): unknown {
  return Array.isArray(mock) ? [] : isObj(mock) ? {} : typeof mock === "string" ? "" : null;
}

/** The `data` root: each source under its selected mock state, as PXL reads it (`value`, `loading`, `error`). */
export function dataRoot(mocks: SourceMocks, states: Readonly<Record<string, MockState>>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [name, mock] of mocks) {
    const state = states[name] ?? "success";
    out[name] = {
      value: state === "success" ? mock : state === "empty" ? emptyOf(mock) : null,
      loading: state === "loading",
      error: state === "error" ? { message: "" } : null,
      hasMore: false,
      status: state === "loading" ? "loading" : state === "error" ? "error" : "ready",
    };
  }
  return out;
}

/** The roots a page's scope gives, without a list item's. */
function pageRoots(scope: DesignScope, data: Record<string, unknown>): Record<string, unknown> {
  return {
    params: scope.params,
    page: scope.page,
    plugin: scope.plugin,
    app: scope.app,
    data,
    ...(scope.device === undefined ? {} : { device: scope.device }),
    ...(scope.flags === undefined ? {} : { flags: scope.flags }),
    ...(scope.props === undefined ? {} : { props: scope.props }),
    ...(scope.component === undefined ? {} : { component: scope.component }),
  };
}

/** Evaluates the screens' expressions with the PXL engine; one per canvas. */
export class PxlPreview {
  readonly #send: EvalTransport;
  readonly #answered: (screens: ReadonlySet<string>) => void;
  readonly #now: number;
  readonly #answers = new Map<string, Answer>();
  readonly #asks = new Map<string, Ask>();
  readonly #inFlight = new Set<string>();
  readonly #versions = new WeakMap<object, number>();
  readonly #sets = new WeakMap<DesignScope, ValuesSet>();
  /** Each page scope's values set, by its page state object: a list item's scope spreads the same roots. */
  readonly #pageSets = new WeakMap<object, { readonly scope: DesignScope; readonly set: ValuesSet }>();
  #nextVersion = 1;
  #context: PassContext | undefined;
  /** Each screen's data root, by its mocks, made once for the same mocks and states. */
  readonly #dataRoots = new WeakMap<SourceMocks, { readonly states: ScreenData["states"]; readonly root: DataRoot }>();
  /** The locale and messages the answers are in, the messages as JSON; a change drops the answers. */
  #words = "";
  #messages = "";
  #sentMessages = "";
  /** Counts the changes of locale and messages: an answer to an older batch is not kept. */
  #generation = 0;

  /**
   * `send` reaches the engine; `answered` is told which screens have new answers, to draw them again; `now` is the
   * frozen current time PXL's `now` reads.
   */
  constructor(send: EvalTransport, answered: (screens: ReadonlySet<string>) => void, now: number = Date.now()) {
    this.#send = send;
    this.#answered = answered;
    this.#now = now;
  }

  /** Starts a pass over the screens; another locale or other messages drop what was answered in the former ones. */
  begin(context: PassContext): void {
    if (this.#context?.messages !== context.messages || this.#context?.locale !== context.locale) {
      this.#messages = JSON.stringify(context.messages);
      const words = `${context.locale}\u0000${hash53(this.#messages)}`;
      if (words !== this.#words) {
        this.#words = words;
        this.#answers.clear();
        this.#generation += 1;
      }
    }
    this.#context = context;
  }

  /** The evaluator a screen's design scopes carry; its expressions are the page's, its data the screen's. */
  forScreen(screen: string, page: PageSource, data: ScreenData): Evaluate {
    return this.#evaluator(screen, page, data, NOT_ASKED);
  }

  /**
   * The evaluator of a component's own expressions on a screen (ADR-0112): they read the instance's `props` and the
   * component's state, which the scope of its drawn tree holds, beside the screen's data.
   */
  forComponent(screen: string, component: PageSource, data: ScreenData): Evaluate {
    return this.#evaluator(screen, component, data, NOT_ASKED_IN_COMPONENT);
  }

  #evaluator(screen: string, page: PageSource, data: ScreenData, notAsked: RegExp): Evaluate {
    const version = this.#versionOf(page.doc);
    const root = this.#dataRoot(data);
    return (src, scope) => {
      // An expression of a list's item template has its value only in a copy, with the copy's item.
      if ((scope.index === undefined && readsItem(src)) || notAsked.test(src)) {
        return undefined;
      }
      const values = this.#setOf(scope, root);
      const key = `${page.file}\u0000${version}\u0000${values.key}\u0000${src}`;
      const answer = this.#answers.get(key);
      if (answer !== undefined) {
        return answer === FAILED ? undefined : answer.value;
      }
      if (!this.#inFlight.has(key)) {
        this.#asks.set(key, { screen, page, values, src });
      }
      return undefined;
    };
  }

  /** Whether a pass left expressions to evaluate. */
  get pending(): boolean {
    return this.#asks.size > 0;
  }

  /**
   * Sends what the passes asked for, one batch per page, so each page's screens are drawn as soon as its own answers
   * come; the answers are kept and their screens told.
   */
  flush(): void {
    const context = this.#context;
    if (this.#asks.size === 0 || context === undefined) {
      return;
    }
    const byPage = new Map<object, [string, Ask][]>();
    for (const entry of this.#asks) {
      const group = byPage.get(entry[1].page.doc);
      if (group === undefined) {
        byPage.set(entry[1].page.doc, [entry]);
      } else {
        group.push(entry);
      }
    }
    this.#asks.clear();
    for (const asks of byPage.values()) {
      this.#sendBatch(asks, context);
    }
  }

  #sendBatch(asks: readonly [string, Ask][], context: PassContext): void {
    for (const [key] of asks) {
      this.#inFlight.add(key);
    }
    const request = this.#request(
      asks.map(([, ask]) => ask),
      context,
    );
    const keys = asks.map(([key]) => key);
    const screens = new Set(asks.map(([, ask]) => ask.screen));
    const generation = this.#generation;
    void this.#send(request).then(
      (answer) => this.#receive(keys, answer, screens, generation),
      () => {
        // A worker that failed or was let go answers nothing; the expressions are asked for again on a later pass.
        for (const key of keys) {
          this.#inFlight.delete(key);
        }
      },
    );
  }

  #receive(keys: readonly string[], answer: unknown, screens: ReadonlySet<string>, generation: number): void {
    if (generation !== this.#generation) {
      // Answered in a locale or with messages no longer drawn: asked for again in the current ones.
      for (const key of keys) {
        this.#inFlight.delete(key);
      }
      return;
    }
    const results = isObj(answer) && Array.isArray(answer["results"]) ? answer["results"] : [];
    if (this.#answers.size + keys.length > ANSWERS_KEPT) {
      this.#answers.clear();
    }
    keys.forEach((key, i) => {
      this.#inFlight.delete(key);
      const r: unknown = results[i];
      this.#answers.set(key, isObj(r) && "value" in r ? { value: r["value"] } : FAILED);
    });
    this.#answered(screens);
  }

  /** The batch's JSON: each page and values set once, items by index. */
  #request(asks: readonly Ask[], context: PassContext): string {
    const pages: { file: string; data: object }[] = [];
    const pageAt = new Map<object, number>();
    const values: { base?: number; roots: Record<string, unknown> }[] = [];
    const valuesAt = new Map<string, number>();
    const setIndex = (set: ValuesSet): number => {
      const known = valuesAt.get(set.key);
      if (known !== undefined) {
        return known;
      }
      const base = set.base === undefined ? undefined : setIndex(set.base);
      const at = values.length;
      values.push(base === undefined ? { roots: set.roots } : { base, roots: set.roots });
      valuesAt.set(set.key, at);
      return at;
    };
    const items = asks.map((ask) => {
      let page = pageAt.get(ask.page.doc);
      if (page === undefined) {
        page = pages.length;
        pages.push({ file: ask.page.file, data: ask.page.doc });
        pageAt.set(ask.page.doc, page);
      }
      return { page, values: setIndex(ask.values), src: ask.src };
    });
    const sendMessages = this.#messages !== this.#sentMessages;
    this.#sentMessages = this.#messages;
    return JSON.stringify({
      locale: context.locale,
      ...(sendMessages ? { messages: context.messages } : {}),
      now: this.#now,
      pages,
      values,
      items,
    });
  }

  /** A document's version: the same object keeps its number, an edited one (a new object) gets a new one. */
  #versionOf(doc: object): number {
    let v = this.#versions.get(doc);
    if (v === undefined) {
      v = this.#nextVersion++;
      this.#versions.set(doc, v);
    }
    return v;
  }

  /** A screen's data root, made once for the same mocks and states. */
  #dataRoot(data: ScreenData): DataRoot {
    const kept = this.#dataRoots.get(data.mocks);
    if (kept?.states === data.states) {
      return kept.root;
    }
    const roots = dataRoot(data.mocks, data.states);
    const root = { roots, key: hash53(JSON.stringify(roots)) };
    this.#dataRoots.set(data.mocks, { states: data.states, root });
    return root;
  }

  /** The values set of a scope: a page's roots, or a list item's over them; kept per scope object. */
  #setOf(scope: DesignScope, data: DataRoot): ValuesSet {
    const kept = this.#sets.get(scope);
    if (kept !== undefined) {
      return kept;
    }
    const pageSet = this.#pageSetOf(scope, data);
    const set: ValuesSet =
      scope.index === undefined
        ? pageSet
        : {
            key: `${pageSet.key}.${hash53(JSON.stringify([scope.item ?? null, scope.index]))}`,
            roots: { item: scope.item ?? null, index: scope.index },
            base: pageSet,
          };
    this.#sets.set(scope, set);
    return set;
  }

  /** The values set of a scope's page roots, made once for the same roots and data. */
  #pageSetOf(scope: DesignScope, data: DataRoot): ValuesSet {
    const kept = this.#pageSets.get(scope.page);
    const was = kept?.scope;
    if (
      kept !== undefined &&
      was?.params === scope.params &&
      was.plugin === scope.plugin &&
      was.app === scope.app &&
      was.device === scope.device &&
      was.flags === scope.flags &&
      was.props === scope.props &&
      was.component === scope.component &&
      kept.set.roots["data"] === data.roots
    ) {
      return kept.set;
    }
    const roots = pageRoots(scope, data.roots);
    const set: ValuesSet = { key: `${data.key}.${hash53(JSON.stringify({ ...roots, data: undefined }))}`, roots };
    this.#pageSets.set(scope.page, { scope, set });
    return set;
  }
}
