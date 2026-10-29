// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Icon, Select, TypeTag } from "@plux/ui";
import type { Problem } from "@plux/validator";
import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import { useCanEdit, useEditor, useEditSession } from "../editor/editor-provider.tsx";
import type { JsonValue } from "../editor/json-patch.ts";
import { type JsonEditorMessages, jsonEditorMessages } from "./json-editor.messages.ts";
import { MonacoView } from "./lazy-view.tsx";
import { lineColumn, locate } from "./locate.ts";
import { mergeInto } from "./merge.ts";
import type { EditorMarker } from "./monaco-view.types.ts";
import type { LoadPublished } from "./use-published.ts";

/** Milliseconds after the last keystroke before the validator worker checks the text (the same as PXL fields, F9). */
export const JSON_DEBOUNCE_MS = 300;

export interface JsonDocument {
  readonly path: string;
  readonly content: Uint8Array;
}

/** What the validation of the text says. */
export type Verdict =
  | { readonly state: "checking" }
  | { readonly state: "unavailable" }
  | {
      readonly state: "done";
      readonly value: Record<string, unknown> | undefined;
      readonly problems: readonly Problem[];
    };

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/** The text of a document as the editor shows it: two-space indentation and a final newline. */
export function formatDocument(value: unknown): string {
  return `${JSON.stringify(value, null, 2)}\n`;
}

/** Checks `text`: syntax first (in the page), then the project's rules through the validator worker. */
export async function judge(
  text: string,
  path: string,
  validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined,
  syntax: (message: string) => string,
  notObject: string,
): Promise<Verdict> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e);
    return { state: "done", value: undefined, problems: [problem(path, syntax(message))] };
  }
  if (!isObject(parsed)) {
    return { state: "done", value: undefined, problems: [problem(path, notObject)] };
  }
  if (validatePage === undefined) {
    return { state: "unavailable" };
  }
  const found = await validatePage(path, text);
  return { state: "done", value: parsed, problems: found.filter((p) => p.file === path || p.file === "") };
}

function problem(file: string, message: string): Problem {
  return { code: "", severity: "error", file, path: "", message };
}

/** Findings as markers in the text: a finding with a pointer sits on the value it names, others on the first line. */
export function markersOf(text: string, problems: readonly Problem[]): EditorMarker[] {
  return problems.flatMap((p) => {
    if (p.severity === "info") {
      return [];
    }
    const span = p.path === "" ? undefined : locate(text, p.path);
    const start = lineColumn(text, span?.start ?? 0);
    const end = lineColumn(text, span?.end ?? Math.max(1, text.indexOf("\n")));
    return [
      {
        severity: p.severity,
        message: p.code === "" ? p.message : `${p.code}: ${p.message}`,
        startLine: start.line,
        startColumn: start.column,
        endLine: end.line,
        endColumn: end.column,
      },
    ];
  });
}

export interface JsonEditorOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The plugin's documents as the server holds them; unsaved edits come from the editor store. */
  readonly documents: readonly JsonDocument[];
  /** The document to start with, usually the one the selection is in. */
  readonly initialPath: string | undefined;
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
  readonly loadPublished: LoadPublished;
}

type Published =
  | { readonly state: "loading" }
  | { readonly state: "failed" }
  | { readonly state: "ready"; readonly text: string | null };

/** The buffer the user edits: seeded from the document each time the overlay opens or another document is chosen. */
function useBuffer(props: JsonEditorOverlayProps) {
  const local = useEditor((st) => st.docs);
  const paths = useMemo(() => props.documents.map((d) => d.path).sort(), [props.documents]);
  const [chosen, setPath] = useState<string | undefined>(undefined);
  const path = chosen ?? props.initialPath ?? paths[0] ?? "";
  const [text, setText] = useState("");
  const seeded = useRef("");
  const current = useMemo(
    () =>
      documentText(
        local[path]?.content,
        props.documents.find((d) => d.path === path),
      ),
    [local, path, props.documents],
  );
  useEffect(() => {
    if (!props.open) {
      seeded.current = "";
    } else if (seeded.current !== path) {
      seeded.current = path;
      setText(current);
    }
  }, [props.open, path, current]);
  useEffect(() => {
    if (!props.open) {
      setPath(undefined);
    }
  }, [props.open]);
  return { paths, path, setPath, text, setText, current };
}

/** The document as the editor shows it: the session's unsaved content over the server's. */
function documentText(edited: unknown, doc: JsonDocument | undefined): string {
  if (edited !== undefined) {
    return formatDocument(edited);
  }
  if (doc === undefined) {
    return "";
  }
  const raw = new TextDecoder().decode(doc.content);
  try {
    return formatDocument(JSON.parse(raw));
  } catch {
    return raw;
  }
}

/** The verdict on `text`, `JSON_DEBOUNCE_MS` after the last keystroke. */
function useVerdict(
  open: boolean,
  text: string,
  path: string,
  validatePage: JsonEditorOverlayProps["validatePage"],
  m: JsonEditorMessages,
): Verdict {
  const [verdict, setVerdict] = useState<Verdict>({ state: "checking" });
  const { syntax, notObject } = m;
  useEffect(() => {
    if (!open || path === "") {
      return undefined;
    }
    let live = true;
    setVerdict({ state: "checking" });
    const timer = setTimeout(() => {
      judge(text, path, validatePage, syntax, notObject)
        .then((v) => live && setVerdict(v))
        .catch(() => live && setVerdict({ state: "unavailable" }));
    }, JSON_DEBOUNCE_MS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [open, text, path, validatePage, syntax, notObject]);
  return verdict;
}

/** The last published version of the document, fetched when the comparison is switched on. */
function usePublishedText(open: boolean, diff: boolean, path: string, load: LoadPublished): Published {
  const [published, setPublished] = useState<Published>({ state: "loading" });
  useEffect(() => {
    if (!open || !diff || path === "") {
      return undefined;
    }
    let live = true;
    setPublished({ state: "loading" });
    load(path)
      .then((t) => live && setPublished({ state: "ready", text: t }))
      .catch(() => live && setPublished({ state: "failed" }));
    return () => {
      live = false;
    };
  }, [open, diff, path, load]);
  return published;
}

function statusOf(verdict: Verdict, valid: boolean, changed: boolean, m: JsonEditorMessages): string {
  if (verdict.state !== "done") {
    return verdict.state === "checking" ? m.checking : m.unavailable;
  }
  if (!valid) {
    return m.invalid;
  }
  return changed ? m.valid : m.nothingToApply;
}

function PublishedNotes({ diff, published, m }: { diff: boolean; published: Published; m: JsonEditorMessages }) {
  if (!diff) {
    return null;
  }
  if (published.state === "loading") {
    return <p className="text-caption text-secondary">{m.loadingPublished}</p>;
  }
  if (published.state === "failed") {
    return (
      <p role="alert" className="text-caption">
        {m.publishedFailed}
      </p>
    );
  }
  return published.text === null ? <p className="text-caption text-secondary">{m.noPublished}</p> : null;
}

function ProblemList({ problems, empty, m }: { problems: readonly Problem[]; empty: string; m: JsonEditorMessages }) {
  return (
    <section aria-label={m.problems} className="max-h-40 overflow-y-auto">
      <h3 className="text-label font-medium">{m.problems}</h3>
      {problems.length === 0 ? (
        <p className="text-body text-secondary">{empty}</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {problems.map((p) => (
            <li key={`${p.code}${p.path}${p.message}`} className="text-body">
              {p.code !== "" && <TypeTag>{p.code}</TypeTag>} {p.message}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** The JSON editor of S5.1 (STU-064): Monaco over the canvas, validated as you type, applied only when valid. */
export function JsonEditorOverlay(props: JsonEditorOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => jsonEditorMessages(i18n), [i18n]);
  const canEdit = useCanEdit();
  const session = useEditSession();
  const { paths, path, setPath, text, setText, current } = useBuffer(props);
  const [diff, setDiff] = useState(false);
  const [note, setNote] = useState("");
  const applied = useRef(0);
  const published = usePublishedText(props.open, diff, path, props.loadPublished);
  const verdict = useVerdict(props.open, text, path, props.validatePage, m);

  const problems = verdict.state === "done" ? verdict.problems : [];
  const markers = useMemo(() => markersOf(text, problems), [text, problems]);
  const valid =
    verdict.state === "done" && verdict.value !== undefined && !problems.some((p) => p.severity === "error");
  const changed = text !== current;
  const canApply = canEdit && valid && changed;
  const status = statusOf(verdict, valid, changed, m);

  // A note ("Applied") belongs to the text it was made for.
  // biome-ignore lint/correctness/useExhaustiveDependencies: the note clears when the text or document changes
  useEffect(() => setNote(""), [text, path]);

  const apply = () => {
    if (verdict.state !== "done" || verdict.value === undefined || !canApply) {
      return;
    }
    const next = verdict.value as Record<string, JsonValue>;
    applied.current += 1;
    session.edit(path, (draft) => mergeInto(draft, next), { group: `json-editor:${path}:${applied.current}` });
    setNote(m.applied);
  };

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      size="full"
      title={m.title}
      labels={{ close: m.close }}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <Select
            label={m.document}
            options={paths.map((p) => ({ value: p, label: p }))}
            value={path}
            onValueChange={setPath}
          />
          <Button variant="secondary" aria-pressed={diff} onClick={() => setDiff((d) => !d)}>
            <Icon name="eye" />
            {m.diff}
          </Button>
          <Button onClick={apply} disabled={!canApply}>
            {m.apply}
          </Button>
          <span role="status" aria-live="polite" className="text-body text-secondary">
            {note === "" ? status : note}
          </span>
        </div>
        {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
        <PublishedNotes diff={diff} published={published} m={m} />
        <div className="min-h-0 flex-1">
          {props.open && (
            <Suspense fallback={<p className="text-body text-secondary">{m.checking}</p>}>
              <MonacoView
                value={text}
                baseline={published.state === "ready" ? (published.text ?? "") : undefined}
                diff={diff && published.state === "ready" && published.text !== null}
                readOnly={!canEdit}
                label={m.editorLabel}
                markers={markers}
                onChange={setText}
              />
            </Suspense>
          )}
        </div>
        <ProblemList problems={problems} empty={verdict.state === "done" ? m.noProblems : status} m={m} />
      </div>
    </Dialog>
  );
}
