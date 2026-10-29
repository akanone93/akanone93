// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Dialog, Field, Icon, Input, Select, Tabs } from "@plux/ui";
import { Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useCanEdit } from "../editor/editor-provider.tsx";
import { RustMonacoView } from "./lazy-view.tsx";
import type { Diagnostic } from "./lsp-client.ts";
import { markersOfDiagnostics } from "./lsp-convert.ts";
import { type RustEditorMessages, rustEditorMessages } from "./rust-editor.messages.ts";
import { DependenciesPanel, TestsPanel } from "./rust-panels.tsx";
import { RustSession, type SessionBridge, type SessionState } from "./rust-session.ts";
import { starterTemplates } from "./templates.ts";
import { idleRun, runStoredTests, type TestRunState, type TestsClient } from "./test-runner.ts";

/** The manifest a new function starts with; dependencies are written below its `[dependencies]` table. */
export const initialManifest = `[package]
name = "function"
version = "0.1.0"
edition = "2021"

[dependencies]
`;

export interface RustEditorOverlayProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** A new bridge for each language-server session. */
  readonly createBridge: () => SessionBridge;
  readonly tests: TestsClient;
  /** The source the overlay opens on, when it was opened for a function (Code tab). */
  readonly initialSourceId?: string | undefined;
}

/** The status line for a session state, in the person's language. */
export function describeSession(state: SessionState, m: RustEditorMessages): string {
  switch (state.phase) {
    case "idle":
      return m.idle;
    case "starting":
      return m.starting;
    case "ready":
      return m.ready;
    case "failed":
      return m.failed(state.detail);
    case "closed":
      switch (state.reason) {
        case "time":
          return m.closedTime;
        case "exited":
          return m.closedExited;
        case "client":
          return m.closedClient;
        default:
          return m.closedFailed(state.detail);
      }
  }
}

/** The language-server session of the overlay: started on demand, ended with the overlay. */
function useRustSession(open: boolean, createBridge: () => SessionBridge) {
  const [state, setState] = useState<SessionState>({ phase: "idle" });
  const [diagnostics, setDiagnostics] = useState<readonly Diagnostic[]>([]);
  const session = useRef<RustSession | null>(null);
  const [language, setLanguage] = useState<RustSession | undefined>(undefined);

  const stop = useCallback(() => {
    session.current?.close();
    session.current = null;
    setLanguage(undefined);
  }, []);

  const start = useCallback(
    (sourceId: string, text: string) => {
      session.current?.close();
      setDiagnostics([]);
      const next = new RustSession(createBridge(), {
        onState: (s) => {
          if (session.current !== next) {
            return;
          }
          setState(s);
          setLanguage(s.phase === "ready" ? next : undefined);
        },
        onDiagnostics: (d) => session.current === next && setDiagnostics(d),
      });
      session.current = next;
      void next.start(sourceId, text);
    },
    [createBridge],
  );

  useEffect(() => {
    if (!open) {
      stop();
      setState({ phase: "idle" });
      setDiagnostics([]);
    }
  }, [open, stop]);
  useEffect(() => stop, [stop]);

  return { state, diagnostics, language, start, stop, change: (text: string) => session.current?.change(text) };
}

/** The test runner of the overlay: one run at a time, aborted when the overlay closes. */
function useTestRun(open: boolean, client: TestsClient) {
  const [run, setRun] = useState<TestRunState>(idleRun);
  const abort = useRef<AbortController | null>(null);
  useEffect(() => {
    if (!open) {
      abort.current?.abort();
      setRun(idleRun);
    }
  }, [open]);
  const start = (sourceId: string) => {
    abort.current?.abort();
    const controller = new AbortController();
    abort.current = controller;
    void runStoredTests(client, sourceId, setRun, controller.signal);
  };
  return { run, start };
}

/** The state of the Rust editor V3 (FN-033), shared by the overlay and the full page. */
function useRustEditor(props: RustEditorOverlayProps) {
  const { i18n } = useLingui();
  const m = useMemo(() => rustEditorMessages(i18n), [i18n]);
  const canEdit = useCanEdit();
  const [sourceId, setSourceId] = useState("");
  const [text, setText] = useState(starterTemplates[0]?.source ?? "");
  const [manifest, setManifest] = useState(initialManifest);
  const [templateId, setTemplateId] = useState<string>(starterTemplates[0]?.id ?? "");
  const { open, initialSourceId } = props;
  useEffect(() => {
    if (open && initialSourceId !== undefined) {
      setSourceId(initialSourceId);
    }
  }, [open, initialSourceId]);
  const server = useRustSession(props.open, props.createBridge);
  const tests = useTestRun(props.open, props.tests);
  const markers = useMemo(() => markersOfDiagnostics(server.diagnostics), [server.diagnostics]);
  const trimmed = sourceId.trim();
  const busy = server.state.phase === "starting";

  const edit = (next: string) => {
    setText(next);
    server.change(next);
  };

  return {
    open: props.open,
    m,
    canEdit,
    sourceId,
    setSourceId,
    text,
    manifest,
    setManifest,
    templateId,
    setTemplateId,
    server,
    tests,
    markers,
    trimmed,
    busy,
    edit,
  };
}

type RustEditor = ReturnType<typeof useRustEditor>;

/** The editor itself: Monaco over the session, tests and dependencies beside it. */
function RustEditorBody({ e }: { readonly e: RustEditor }) {
  const { open, m, canEdit, sourceId, setSourceId, text, manifest, setManifest, templateId, setTemplateId } = e;
  const { server, tests, markers, trimmed, busy, edit } = e;
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex flex-wrap items-end gap-3">
        <Field label={m.source} description={m.sourceHint}>
          <Input value={sourceId} onChange={(e) => setSourceId(e.target.value)} spellCheck={false} />
        </Field>
        <Button onClick={() => server.start(trimmed, text)} disabled={trimmed === "" || busy}>
          <Icon name="play" />
          {m.connect}
        </Button>
        {server.state.phase === "ready" && (
          <Button variant="secondary" onClick={server.stop}>
            {m.stop}
          </Button>
        )}
        <Select
          label={m.template}
          options={starterTemplates.map((t) => ({ value: t.id, label: t.title }))}
          value={templateId}
          onValueChange={setTemplateId}
        />
        <Button
          variant="secondary"
          disabled={!canEdit}
          onClick={() => edit(starterTemplates.find((t) => t.id === templateId)?.source ?? text)}
        >
          {m.useTemplate}
        </Button>
      </div>
      <p role="status" aria-live="polite" className="text-body text-secondary">
        {describeSession(server.state, m)}
      </p>
      {!canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
      <div className="grid min-h-0 flex-1 grid-cols-3 gap-3">
        <div className="col-span-2 min-h-0">
          {open && (
            <Suspense fallback={<p className="text-body text-secondary">{m.starting}</p>}>
              <RustMonacoView
                value={text}
                readOnly={!canEdit}
                label={m.editorLabel}
                language={server.language}
                markers={markers}
                onChange={edit}
              />
            </Suspense>
          )}
        </div>
        <div className="min-h-0 overflow-y-auto">
          <Tabs
            defaultValue="tests"
            tabs={[
              {
                value: "tests",
                label: m.tabTests,
                panel: <TestsPanel run={tests.run} canRun={trimmed !== ""} onRun={() => tests.start(trimmed)} m={m} />,
              },
              {
                value: "dependencies",
                label: m.tabDependencies,
                panel: <DependenciesPanel manifest={manifest} canEdit={canEdit} onChange={setManifest} m={m} />,
              },
            ]}
          />
        </div>
      </div>
    </div>
  );
}

/** The Rust editor V3 (FN-033): Monaco over the canvas, rust-analyzer through the bridge, tests and dependencies beside it. */
export function RustEditorOverlay(props: RustEditorOverlayProps) {
  const e = useRustEditor(props);
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      size="full"
      title={e.m.title}
      labels={{ close: e.m.close }}
    >
      <RustEditorBody e={e} />
    </Dialog>
  );
}

/** The same editor as a full page (A12): it is always open and ends with the page. */
export function RustEditorPage(props: Omit<RustEditorOverlayProps, "open" | "onClose">) {
  const e = useRustEditor({ ...props, open: true, onClose: () => undefined });
  return (
    <section className="flex h-full min-h-0 flex-col gap-3" aria-label={e.m.title}>
      <h1 className="text-screen">{e.m.title}</h1>
      <RustEditorBody e={e} />
    </section>
  );
}
