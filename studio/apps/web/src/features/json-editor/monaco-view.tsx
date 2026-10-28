// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// The editor core and JSON only: the package root also registers every language and bundles the
// TypeScript, CSS and HTML workers, none of which Studio uses.
import * as monaco from "monaco-editor/editor/editor.api.js";
import "monaco-editor/language/json/monaco.contribution.js";
import { useEffect, useRef } from "react";
import { isDark } from "../../lib/color-scheme.ts";
import type { MonacoViewProps } from "./monaco-view.types.ts";

const MARKER_OWNER = "plux-validator";
const severity = { error: monaco.MarkerSeverity.Error, warning: monaco.MarkerSeverity.Warning } as const;

/** Features that render markdown or HTML through policies the CSP does not admit are off (monaco-env.ts). */
const options: monaco.editor.IStandaloneEditorConstructionOptions & monaco.editor.IDiffEditorConstructionOptions = {
  automaticLayout: true,
  minimap: { enabled: false },
  hover: { enabled: "off" },
  parameterHints: { enabled: false },
  quickSuggestions: { other: false, comments: false, strings: false },
  suggestOnTriggerCharacters: false,
  codeLens: false,
  lightbulb: { enabled: monaco.editor.ShowLightbulbIconMode.Off },
  stickyScroll: { enabled: false },
  scrollBeyondLastLine: false,
  tabSize: 2,
  fontFamily: "var(--font-mono)",
};

const dark = isDark;

/** Monaco's JSON editor, or its diff editor against `baseline` (S5.1). Loaded lazily; the one place Monaco is used. */
export default function MonacoView(props: MonacoViewProps) {
  const host = useRef<HTMLDivElement>(null);
  const model = useRef<monaco.editor.ITextModel | null>(null);
  const onChange = useRef(props.onChange);
  onChange.current = props.onChange;
  const { diff, readOnly, label, value, baseline } = props;

  // The value seeds the model once per editor; later changes arrive through the effect below.
  // biome-ignore lint/correctness/useExhaustiveDependencies: see above
  useEffect(() => {
    const element = host.current;
    if (element === null) {
      return undefined;
    }
    const modified = monaco.editor.createModel(value, "json");
    model.current = modified;
    const subscription = modified.onDidChangeContent(() => onChange.current(modified.getValue()));
    const theme = dark() ? "vs-dark" : "vs";
    const original = diff ? monaco.editor.createModel(baseline ?? "", "json") : null;
    let dispose: () => void;
    if (diff && original !== null) {
      const editor = monaco.editor.createDiffEditor(element, {
        ...options,
        theme,
        readOnly,
        originalEditable: false,
        renderSideBySide: true,
        ariaLabel: label,
      });
      editor.setModel({ original, modified });
      dispose = () => editor.dispose();
    } else {
      const editor = monaco.editor.create(element, { ...options, theme, readOnly, model: modified, ariaLabel: label });
      editor.focus();
      dispose = () => editor.dispose();
    }
    return () => {
      subscription.dispose();
      dispose();
      original?.dispose();
      modified.dispose();
      model.current = null;
    };
  }, [diff, readOnly, label, baseline]);

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
    monaco.editor.setModelMarkers(
      m,
      MARKER_OWNER,
      props.markers.map((x) => ({
        severity: severity[x.severity],
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
