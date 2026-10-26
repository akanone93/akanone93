// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { autocompletion, type Completion, type CompletionContext, completionKeymap } from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { type Diagnostic, linter, lintKeymap } from "@codemirror/lint";
import { Compartment, EditorState, StateEffect, StateField } from "@codemirror/state";
import {
  EditorView,
  hoverTooltip,
  keymap,
  showTooltip,
  type Tooltip,
  ViewPlugin,
  type ViewUpdate,
} from "@codemirror/view";
import type { PxlCompletionKind, PxlService, PxlSignatureHelp } from "@plux/validator";
import { type ReactElement, useEffect, useRef } from "react";

/** Milliseconds after the last keystroke before diagnostics and signature help are asked for. */
export const PXL_EDITOR_DELAY_MS = 300;

/** What the editor needs to talk to the language service about one expression. */
export interface PxlEditorSource {
  readonly service: PxlService;
  /** The serialised scope of the expression's site; undefined when the page has no such site. */
  scope(): Promise<string | undefined>;
}

export interface PxlEditorProps {
  readonly value: string;
  readonly label: string;
  readonly readOnly: boolean;
  readonly source: PxlEditorSource;
  readonly onChange: (value: string) => void;
  readonly onBlur: () => void;
  /** The result type after each check; empty when the expression has errors. */
  readonly onType?: (type: string) => void;
}

const completionType: Record<PxlCompletionKind, string> = {
  variable: "variable",
  root: "namespace",
  field: "property",
  function: "function",
  method: "method",
  keyword: "keyword",
};

const theme = EditorView.theme({
  "&": { backgroundColor: "transparent", color: "var(--color-primary)", fontSize: "inherit" },
  "&.cm-focused": { outline: "none" },
  ".cm-content": {
    fontFamily: "var(--font-mono, ui-monospace, monospace)",
    padding: "6px 0",
    caretColor: "var(--color-primary)",
  },
  ".cm-line": { padding: "0" },
  ".cm-scroller": { overflowX: "auto", overflowY: "hidden", lineHeight: "1.4" },
  ".cm-placeholder": { color: "var(--color-tertiary)" },
  ".cm-tooltip": {
    backgroundColor: "var(--color-raised)",
    color: "var(--color-primary)",
    border: "1px solid var(--color-strong)",
    borderRadius: "6px",
    padding: "4px 8px",
    maxWidth: "420px",
  },
  ".cm-tooltip-autocomplete": { padding: "0" },
  ".cm-tooltip-autocomplete > ul > li": { padding: "2px 8px" },
  ".cm-tooltip-autocomplete > ul > li[aria-selected]": {
    backgroundColor: "var(--color-accent-halo)",
    color: "var(--color-primary)",
  },
  ".cm-completionDetail": { color: "var(--color-tertiary)", marginInlineStart: "8px", fontStyle: "normal" },
  ".cm-pxl-type": { fontFamily: "var(--font-mono, ui-monospace, monospace)" },
  ".cm-pxl-active": { fontWeight: "700", color: "var(--color-accent)" },
  ".cm-diagnostic-error": { borderInlineStartColor: "var(--color-danger, #c0392b)" },
});

const showSignature = StateEffect.define<Tooltip | null>();

const signatureField = StateField.define<Tooltip | null>({
  create: () => null,
  update(value, tr) {
    for (const e of tr.effects) {
      if (e.is(showSignature)) {
        return e.value;
      }
    }
    return tr.docChanged || tr.selection ? null : value;
  },
  provide: (f) => showTooltip.from(f),
});

/** Draws a signature help tooltip with the active parameter in bold. */
function signatureDom(help: PxlSignatureHelp): HTMLElement {
  const dom = document.createElement("div");
  dom.className = "cm-pxl-signature";
  const sig = help.signatures[help.activeSignature] ?? help.signatures[0];
  if (sig === undefined) {
    return dom;
  }
  const open = sig.label.indexOf("(");
  const close = sig.label.lastIndexOf(")");
  const head = open < 0 || close < open ? sig.label : sig.label.slice(0, open + 1);
  dom.append(document.createTextNode(head));
  if (open >= 0 && close > open) {
    sig.parameters.forEach((p, i) => {
      const span = document.createElement("span");
      span.textContent = p;
      if (i === help.activeParameter) {
        span.className = "cm-pxl-active";
      }
      dom.append(span, document.createTextNode(i < sig.parameters.length - 1 ? ", " : ""));
    });
    dom.append(document.createTextNode(sig.label.slice(close)));
  }
  return dom;
}

/** The extensions that connect an editor to the language service of one expression. */
function languageExtensions(source: () => PxlEditorSource, onType: () => ((type: string) => void) | undefined) {
  const withScope = async <T,>(use: (service: PxlService, scope: string) => Promise<T>): Promise<T | null> => {
    const s = source();
    const scope = await s.scope();
    return scope === undefined ? null : use(s.service, scope);
  };
  const complete = async (ctx: CompletionContext) => {
    const text = ctx.state.doc.toString();
    const list = await withScope((service, scope) => service.complete(scope, text, ctx.pos));
    if (list === null || (list.items.length === 0 && !ctx.explicit)) {
      return null;
    }
    return {
      from: list.from,
      validFor: /^\w*$/,
      options: list.items.map(
        (i): Completion => ({
          label: i.label,
          type: completionType[i.kind],
          ...(i.detail === undefined ? {} : { detail: i.detail }),
          apply: i.insert,
        }),
      ),
    };
  };
  const lint = linter(
    async (view): Promise<Diagnostic[]> => {
      const text = view.state.doc.toString();
      if (text === "") {
        onType()?.("");
        return [];
      }
      const result = await withScope((service, scope) => service.check(scope, text));
      if (result === null || view.state.doc.toString() !== text) {
        return [];
      }
      onType()?.(result.type);
      const end = text.length;
      return result.diagnostics.map((d) => ({
        from: Math.min(d.from, end),
        to: Math.min(Math.max(d.to, d.from), end),
        severity: d.severity,
        message: d.message,
        source: d.code,
      }));
    },
    { delay: PXL_EDITOR_DELAY_MS },
  );
  const hover = hoverTooltip(async (view, pos) => {
    const text = view.state.doc.toString();
    const h = await withScope((service, scope) => service.hover(scope, text, pos));
    if (h === null || h === undefined) {
      return null;
    }
    return {
      pos: h.from,
      end: h.to,
      above: true,
      create: () => {
        const dom = document.createElement("div");
        const type = document.createElement("div");
        type.className = "cm-pxl-type";
        type.textContent = h.type;
        dom.append(type);
        for (const sig of h.signatures) {
          const line = document.createElement("div");
          line.className = "cm-pxl-type";
          line.textContent = sig;
          dom.append(line);
        }
        return { dom };
      },
    };
  });
  const signature = ViewPlugin.fromClass(
    class {
      #timer: ReturnType<typeof setTimeout> | undefined;
      update(u: ViewUpdate): void {
        if (!u.docChanged && !u.selectionSet) {
          return;
        }
        clearTimeout(this.#timer);
        this.#timer = setTimeout(() => void this.#ask(u.view), 120);
      }
      async #ask(view: EditorView): Promise<void> {
        const text = view.state.doc.toString();
        const pos = view.state.selection.main.head;
        const help = await withScope((service, scope) => service.signature(scope, text, pos));
        if (view.state.doc.toString() !== text || view.state.selection.main.head !== pos) {
          return;
        }
        view.dispatch({
          effects: showSignature.of(
            help === null || help === undefined || help.signatures.length === 0
              ? null
              : { pos, above: false, create: () => ({ dom: signatureDom(help) }) },
          ),
        });
      }
      destroy(): void {
        clearTimeout(this.#timer);
      }
    },
  );
  return [autocompletion({ override: [complete], icons: false }), lint, hover, signatureField, signature];
}

/** The nonce of the page's scripts, which CodeMirror reuses for its style element under the strict CSP. */
function styleNonce(): string | undefined {
  const n = document.querySelector<HTMLScriptElement>("script[nonce]")?.nonce;
  return n === undefined || n === "" ? undefined : n;
}

/**
 * An inline PXL expression editor on CodeMirror 6 (A4.6): one line, with completion, hover types,
 * inline errors and signature help from the validator module's language service (PXL-007).
 */
export default function PxlEditor(props: PxlEditorProps): ReactElement {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const latest = useRef(props);
  latest.current = props;
  const editable = useRef(new Compartment());

  useEffect(() => {
    const parent = host.current;
    if (parent === null) {
      return undefined;
    }
    const nonce = styleNonce();
    const v = new EditorView({
      parent,
      state: EditorState.create({
        doc: latest.current.value,
        extensions: [
          theme,
          ...(nonce === undefined ? [] : [EditorView.cspNonce.of(nonce)]),
          history(),
          keymap.of([...completionKeymap, ...lintKeymap, ...historyKeymap, ...defaultKeymap]),
          EditorState.transactionFilter.of((tr) => (tr.docChanged && tr.newDoc.lines > 1 ? [] : tr)),
          EditorView.contentAttributes.of({
            "aria-label": latest.current.label,
            "aria-multiline": "false",
            spellcheck: "false",
          }),
          editable.current.of([
            EditorState.readOnly.of(latest.current.readOnly),
            EditorView.editable.of(!latest.current.readOnly),
          ]),
          ...languageExtensions(
            () => latest.current.source,
            () => latest.current.onType,
          ),
          EditorView.updateListener.of((u) => {
            if (u.docChanged && !u.transactions.some((t) => t.isUserEvent("pxl.sync"))) {
              latest.current.onChange(u.state.doc.toString());
            }
          }),
          EditorView.domEventHandlers({
            blur: () => {
              latest.current.onBlur();
            },
          }),
        ],
      }),
    });
    view.current = v;
    return () => {
      v.destroy();
      view.current = null;
    };
  }, []);

  const { value, readOnly } = props;
  useEffect(() => {
    const v = view.current;
    if (v === null) {
      return;
    }
    if (v.state.doc.toString() !== value) {
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value }, userEvent: "pxl.sync" });
    }
    v.dispatch({
      effects: editable.current.reconfigure([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]),
    });
  }, [value, readOnly]);

  return (
    <div
      ref={host}
      className="w-full min-h-(--density-field) overflow-hidden rounded-sm border border-hairline bg-raised px-3 text-body text-primary hover:border-strong focus-within:border-accent"
    />
  );
}
