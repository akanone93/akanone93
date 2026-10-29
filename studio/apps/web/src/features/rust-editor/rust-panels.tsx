// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button, Field, Icon, Input, TypeTag } from "@plux/ui";
import { useMemo, useState } from "react";
import { checkDependency, listDependencies, removeDependency, setDependency } from "./dependencies.ts";
import type { RustEditorMessages } from "./rust-editor.messages.ts";
import type { TestRunState } from "./test-runner.ts";

export interface TestsPanelProps {
  readonly run: TestRunState;
  readonly canRun: boolean;
  readonly onRun: () => void;
  readonly m: RustEditorMessages;
}

/** The results of the stored test cases (FN-034), one row per case as it arrives. */
export function TestsPanel({ run, canRun, onRun, m }: TestsPanelProps) {
  const running = run.status === "running";
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-3">
        <Button variant="secondary" size="sm" onClick={onRun} disabled={!canRun || running}>
          <Icon name="eye" />
          {m.runTests}
        </Button>
        <span role="status" aria-live="polite" className="text-body text-secondary">
          {running && m.runningTests}
          {run.status === "finished" && m.testsPassed(run.passed, run.failed)}
          {run.status === "idle" && m.noTests}
        </span>
      </div>
      {run.status === "failed" && (
        <p role="alert" className="text-body">
          {m.testsFailedToRun(run.error)}
        </p>
      )}
      <ul aria-label={m.tabTests} className="flex flex-col gap-1">
        {run.results.map((r) => (
          <li key={r.name} className="text-body">
            <TypeTag>{r.passed ? m.testPass : m.testFail}</TypeTag> {r.name}
            {r.message !== "" && <span className="text-secondary"> {r.message}</span>}
          </li>
        ))}
      </ul>
      {run.diagnostics !== "" && (
        <section aria-label={m.buildDiagnostics}>
          <h3 className="text-label font-medium">{m.buildDiagnostics}</h3>
          <pre className="max-h-40 overflow-auto font-mono text-caption">{run.diagnostics}</pre>
        </section>
      )}
    </div>
  );
}

export interface DependenciesPanelProps {
  readonly manifest: string;
  readonly canEdit: boolean;
  readonly onChange: (manifest: string) => void;
  readonly m: RustEditorMessages;
}

/** The `[dependencies]` of the function's Cargo manifest (FN-033). */
export function DependenciesPanel({ manifest, canEdit, onChange, m }: DependenciesPanelProps) {
  const dependencies = useMemo(() => listDependencies(manifest), [manifest]);
  const [name, setName] = useState("");
  const [version, setVersion] = useState("");
  // The messages of `checkDependency` are English; which field is wrong is what the panel needs, said in the person's language.
  const nameOk = checkDependency({ name, version: "1" }) === undefined;
  const versionOk = checkDependency({ name: "x", version }) === undefined;
  const nameProblem = name !== "" && !nameOk ? m.crateInvalid : undefined;
  const versionProblem = version !== "" && !versionOk ? m.versionInvalid : undefined;
  const complete = nameOk && versionOk && name !== "";
  const add = () => {
    if (!complete) {
      return;
    }
    onChange(setDependency(manifest, { name, version }));
    setName("");
    setVersion("");
  };
  return (
    <div className="flex flex-col gap-2">
      {dependencies.length === 0 ? (
        <p className="text-body text-secondary">{m.noDependencies}</p>
      ) : (
        <ul aria-label={m.tabDependencies} className="flex flex-col gap-1">
          {dependencies.map((d) => (
            <li key={d.name} className="flex items-center gap-2 text-body">
              <TypeTag>{d.name}</TypeTag>
              <span className="text-secondary">{d.version}</span>
              {canEdit && (
                <Button
                  variant="secondary"
                  size="sm"
                  aria-label={m.removeDependency(d.name)}
                  onClick={() => onChange(removeDependency(manifest, d.name))}
                >
                  <Icon name="trash" />
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      {canEdit && (
        <div className="flex flex-wrap items-end gap-2">
          <Field label={m.crate} error={nameProblem}>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label={m.version} error={versionProblem}>
            <Input value={version} onChange={(e) => setVersion(e.target.value)} />
          </Field>
          <Button size="sm" onClick={add} disabled={!complete}>
            {m.addDependency}
          </Button>
        </div>
      )}
      <p className="text-caption text-secondary">{m.vendorNote}</p>
      <section aria-label={m.manifest}>
        <h3 className="text-label font-medium">{m.manifest}</h3>
        <pre className="max-h-32 overflow-auto font-mono text-caption">{manifest}</pre>
      </section>
    </div>
  );
}
