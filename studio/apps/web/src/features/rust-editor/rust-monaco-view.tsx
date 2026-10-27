// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// The editor core and the Rust tokenizer only (its definition loads as a separate chunk when the first Rust model
// opens); the language features come from the language-server bridge, not from a Monaco language service.
import * as monaco from "monaco-editor/editor/editor.api.js";
import "monaco-editor/languages/definitions/rust/register.js";
import { useEffect, useRef } from "react";
import type { EditorMarker } from "../json-editor/monaco-view.types.ts";
import type { CompletionItem, MarkupContent } from "./lsp-client.ts";
import { hoverTexts, markupText, toLspPosition, toMonacoRange } from "./lsp-convert.ts";
import type { LanguageApi } from "./rust-session.ts";

export interface RustViewProps {
  readonly value: string;
  readonly readOnly: boolean;
  readonly label: string;
  /** The language server's answers; undefined until the session is ready. */
  readonly language: LanguageApi | undefined;
  readonly markers: readonly EditorMarker[];
  readonly onChange: (text: string) => void;
}

const MARKER_OWNER = "rust-analyzer";
const LANGUAGE = "rust";

/** LSP `CompletionItemKind` (1-25) as Monaco's. */
const kinds = (() => {
  const k = monaco.languages.CompletionItemKind;
  return [
    k.Text,
    k.Method,
    k.Function,
    k.Constructor,
    k.Field,
    k.Variable,
    k.Class,
    k.Interface,
    k.Module,
    k.Property,
    k.Unit,
    k.Value,
    k.Enum,
    k.Keyword,
    k.Snippet,
    k.Color,
    k.File,
    k.Reference,
    k.Folder,
    k.EnumMember,
    k.Constant,
    k.Struct,
    k.Event,
    k.Operator,
    k.TypeParameter,
  ];
})();

const dark = (): boolean => globalThis.matchMedia?.("(prefers-color-scheme: dark)").matches === true;

/** `{ documentation }` for a present markup, nothing otherwise (the property may not be set to undefined). */
const documentation = (content: MarkupContent | undefined) =>
  content === undefined ? {} : { documentation: { value: markupText(content) } };

function suggestion(item: CompletionItem, fallback: monaco.IRange): monaco.languages.CompletionItem {
  const edit = item.textEdit;
  const range = edit === undefined ? fallback : toMonacoRange("range" in edit ? edit.range : edit.replace);
  const text = edit?.newText ?? item.insertText ?? item.label;
  return {
    label: item.label,
    kind: kinds[(item.kind ?? 1) - 1] ?? monaco.languages.CompletionItemKind.Text,
    ...(item.detail === undefined ? {} : { detail: item.detail }),
    ...documentation(item.documentation),
    insertText: text,
    ...(item.insertTextFormat === 2
      ? { insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet }
      : {}),
    range,
    ...(item.sortText === undefined ? {} : { sortText: item.sortText }),
    ...(item.filterText === undefined ? {} : { filterText: item.filterText }),
  };
}

/** Registers the language-server-backed providers; `api()` is read on each request so the session may come later. */
function registerProviders(api: () => LanguageApi | undefined): monaco.IDisposable {
  const providers = [
    monaco.languages.registerCompletionItemProvider(LANGUAGE, {
      triggerCharacters: [".", ":", "(", "<"],
      async provideCompletionItems(model, position) {
        const items = (await api()?.completion(toLspPosition(position.lineNumber, position.column))) ?? [];
        const word = model.getWordUntilPosition(position);
        const fallback = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn,
        };
        return { suggestions: items.map((i) => suggestion(i, fallback)) };
      },
    }),
    monaco.languages.registerHoverProvider(LANGUAGE, {
      async provideHover(_model, position) {
        const hover = await api()?.hover(toLspPosition(position.lineNumber, position.column));
        if (hover === null || hover === undefined) {
          return null;
        }
        return {
          contents: hoverTexts(hover.contents).map((value) => ({ value })),
          ...(hover.range === undefined ? {} : { range: toMonacoRange(hover.range) }),
        };
      },
    }),
    monaco.languages.registerSignatureHelpProvider(LANGUAGE, {
      signatureHelpTriggerCharacters: ["(", ","],
      async provideSignatureHelp(_model, position) {
        const help = await api()?.signatureHelp(toLspPosition(position.lineNumber, position.column));
        if (help === null || help === undefined || help.signatures.length === 0) {
          return null;
        }
        return {
          value: {
            activeSignature: help.activeSignature ?? 0,
            activeParameter: help.activeParameter ?? 0,
            signatures: help.signatures.map((s) => ({
              label: s.label,
              ...documentation(s.documentation),
              parameters: (s.parameters ?? []).map((p) => ({ label: p.label, ...documentation(p.documentation) })),
            })),
          },
          dispose: () => undefined,
        };
      },
    }),
    monaco.languages.registerDocumentFormattingEditProvider(LANGUAGE, {
      async provideDocumentFormattingEdits() {
        const edits = (await api()?.format()) ?? [];
        return edits.map((e) => ({ range: toMonacoRange(e.range), text: e.newText }));
      },
    }),
  ];
  return {
    dispose: () => {
      for (const p of providers) {
        p.dispose();
      }
    },
  };
}

/** Monaco's Rust editor (FN-033): tokenizing from Monaco, everything else from rust-analyzer through the bridge. */
export default function RustMonacoView(props: RustViewProps) {
  const host = useRef<HTMLDivElement>(null);
  const model = useRef<monaco.editor.ITextModel | null>(null);
  const onChange = useRef(props.onChange);
  onChange.current = props.onChange;
  const language = useRef(props.language);
  language.current = props.language;
  const { readOnly, label, value } = props;

  // The value seeds the model once per editor; later changes arrive through the effect below.
  // biome-ignore lint/correctness/useExhaustiveDependencies: see above
  useEffect(() => {
    const element = host.current;
    if (element === null) {
      return undefined;
    }
    const providers = registerProviders(() => language.current);
    const text = monaco.editor.createModel(value, LANGUAGE);
    model.current = text;
    const subscription = text.onDidChangeContent(() => onChange.current(text.getValue()));
    const editor = monaco.editor.create(element, {
      model: text,
      theme: dark() ? "vs-dark" : "vs",
      readOnly,
      ariaLabel: label,
      automaticLayout: true,
      minimap: { enabled: false },
      codeLens: false,
      lightbulb: { enabled: monaco.editor.ShowLightbulbIconMode.Off },
      stickyScroll: { enabled: false },
      scrollBeyondLastLine: false,
      tabSize: 4,
      fontFamily: "var(--font-mono)",
    });
    editor.focus();
    return () => {
      subscription.dispose();
      editor.dispose();
      providers.dispose();
      text.dispose();
      model.current = null;
    };
  }, [readOnly, label]);

  useEffect(() => {
    const m = model.current;
    if (m !== null && m.getValue() !== value) {
      m.setValue(value);
    }
  }, [value]);

  useEffect(() => {
    const m = model.current;
    if (m === null) {
      return;
    }
    const sev = { error: monaco.MarkerSeverity.Error, warning: monaco.MarkerSeverity.Warning } as const;
    monaco.editor.setModelMarkers(
      m,
      MARKER_OWNER,
      props.markers.map((x) => ({
        severity: sev[x.severity],
        message: x.message,
        startLineNumber: x.startLine,
        startColumn: x.startColumn,
        endLineNumber: x.endLine,
        endColumn: x.endColumn,
      })),
    );
  }, [props.markers]);

  return <div ref={host} className="h-full w-full" />;
}
