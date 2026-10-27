// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { FunctionVersion } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { type ReactElement, useEffect, useRef, useState } from "react";
import { rustEditorMessages } from "../rust-editor/index.ts";
import { TestsPanel } from "../rust-editor/rust-panels.tsx";
import { idleRun, runStoredTests, type TestRunState, type TestsClient } from "../rust-editor/test-runner.ts";
import { Table } from "../security/parts.tsx";
import type { FunctionsMessages } from "./functions.messages.ts";
import { useLogs, useMetrics, useVersions } from "./use-functions.ts";

type Scope = { readonly app: string; readonly environmentId: string; readonly name: string };

/** The immutable versions, the aliases and the control that points an alias at a version (FN-006). */
export function VersionsPanel({
  scope,
  aliases,
  canPromote,
  onAlias,
  failed,
  m,
}: {
  readonly scope: Scope;
  readonly aliases: readonly { name: string; version: number }[];
  readonly canPromote: boolean;
  readonly onAlias: (alias: string, version: number) => void;
  readonly failed: boolean;
  readonly m: FunctionsMessages;
}): ReactElement {
  const versions = useVersions(scope.app, scope.environmentId, scope.name);
  const [alias, setAlias] = useState("production");
  return (
    <div className="flex flex-col gap-3">
      <ul aria-label={m.colAlias} className="flex flex-col gap-1">
        {aliases.map((a) => (
          <li key={a.name} className="text-body">
            {m.aliasAt(a.name, String(a.version))}
          </li>
        ))}
      </ul>
      <Field label={m.aliasLabel}>
        <Input value={alias} onChange={(e) => setAlias(e.target.value)} />
      </Field>
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.aliasFailed}
        </p>
      )}
      <Table
        caption={m.tabVersions}
        heads={[m.colVersion, m.colTests, m.colCreated, ""]}
        rows={(versions.data?.versions ?? []).map((v) => ({
          key: String(v.version),
          cells: [
            m.versionN(String(v.version)),
            v.testsPassed ? m.testsPassedYes : m.testsPassedNo,
            v.createdAt === undefined ? "" : timestampDate(v.createdAt).toLocaleString(),
            <Button
              key="p"
              size="sm"
              variant="secondary"
              disabled={!canPromote || alias.trim() === ""}
              aria-label={`${m.promoteAlias} ${v.version}`}
              onClick={() => onAlias(alias.trim(), v.version)}
            >
              {m.promoteAlias}
            </Button>,
          ],
        }))}
      />
    </div>
  );
}

/** What was verified about the latest version: signature, tests and provenance (FN-009, FN-019). */
export function AssurancePanel({ v, m }: { readonly v: FunctionVersion; readonly m: FunctionsMessages }): ReactElement {
  const p = v.provenance;
  const deps = p?.dependencies ?? [];
  return (
    <div className="flex flex-col gap-3">
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-body">
        <dt className="text-secondary">{m.assuranceSigned}</dt>
        <dd>{v.signature.length > 0 ? m.assuranceSigned : m.assuranceUnsigned}</dd>
        <dt className="text-secondary">{m.assuranceTests}</dt>
        <dd>{v.testsPassed ? m.testsPassedYes : m.testsPassedNo}</dd>
        <dt className="text-secondary">{m.assuranceModule}</dt>
        <dd className="break-all">{v.moduleSha256}</dd>
        <dt className="text-secondary">{m.assuranceSource}</dt>
        <dd className="break-all">{p?.sourceSha256 ?? ""}</dd>
        <dt className="text-secondary">{m.assuranceToolchain}</dt>
        <dd>{p?.toolchain ?? ""}</dd>
        <dt className="text-secondary">{m.assuranceBuilder}</dt>
        <dd>{p?.builderIdentity ?? ""}</dd>
      </dl>
      {deps.length === 0 ? (
        <p className="text-body text-secondary">{m.noDeps}</p>
      ) : (
        <Table
          caption={m.assuranceDeps}
          heads={[m.assuranceDeps, m.colVersion]}
          rows={deps.map((d) => ({ key: d.name, cells: [d.name, d.version] }))}
        />
      )}
    </div>
  );
}

/** Where the version runs and the contract of its input and output. */
export function CapabilitiesPanel({
  v,
  m,
}: {
  readonly v: FunctionVersion;
  readonly m: FunctionsMessages;
}): ReactElement {
  return (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-body">
      <dt className="text-secondary">{m.capPlacement}</dt>
      <dd>{v.placement === "device" ? m.placementDevice : m.placementServer}</dd>
      <dt className="text-secondary">{m.capAbi}</dt>
      <dd>{String(v.abiVersion)}</dd>
      <dt className="text-secondary">{m.capInput}</dt>
      <dd>
        <pre className="whitespace-pre-wrap font-mono text-caption">{v.inputSchema}</pre>
      </dd>
      <dt className="text-secondary">{m.capOutput}</dt>
      <dd>
        <pre className="whitespace-pre-wrap font-mono text-caption">{v.outputSchema}</pre>
      </dd>
    </dl>
  );
}

/** FN-035: invocation counts, errors by kind, latency percentiles, memory and cache hits. */
export function MetricsPanel({ scope, m }: { readonly scope: Scope; readonly m: FunctionsMessages }): ReactElement {
  const metrics = useMetrics(scope.app, scope.environmentId, scope.name);
  const d = metrics.data;
  if (d === undefined) {
    return <p className="text-body text-secondary">{metrics.isError ? m.loadFailed : m.loading}</p>;
  }
  const errors = Object.entries(d.errorsByKind);
  return (
    <div className="flex flex-col gap-3">
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-body">
        <dt className="text-secondary">{m.metricInvocations}</dt>
        <dd>{String(d.invocations)}</dd>
        <dt className="text-secondary">{m.metricP50}</dt>
        <dd>{String(d.latencyP50Micros)}</dd>
        <dt className="text-secondary">{m.metricP95}</dt>
        <dd>{String(d.latencyP95Micros)}</dd>
        <dt className="text-secondary">{m.metricP99}</dt>
        <dd>{String(d.latencyP99Micros)}</dd>
        <dt className="text-secondary">{m.metricMemory}</dt>
        <dd>{String(d.memoryPeakBytes)}</dd>
        <dt className="text-secondary">{m.metricCache}</dt>
        <dd>{String(d.cacheHits)}</dd>
      </dl>
      {errors.length === 0 ? (
        <p className="text-body text-secondary">{m.noErrors}</p>
      ) : (
        <Table
          caption={m.metricErrors}
          heads={[m.metricErrors, m.metricInvocations]}
          rows={errors.map(([kind, n]) => ({ key: kind, cells: [kind, String(n)] }))}
        />
      )}
    </div>
  );
}

/** FN-035: the retained log lines of the function. */
export function LogsPanel({ scope, m }: { readonly scope: Scope; readonly m: FunctionsMessages }): ReactElement {
  const logs = useLogs(scope.app, scope.environmentId, scope.name);
  const lines = logs.data?.lines ?? [];
  if (logs.isError) {
    return <p className="text-body text-secondary">{m.loadFailed}</p>;
  }
  return lines.length === 0 ? (
    <p className="text-body text-secondary">{m.noLogs}</p>
  ) : (
    <Table
      caption={m.logsTitle}
      heads={[m.colTime, m.colVersion, m.colLevel, m.colMessage]}
      rows={lines.map((l, i) => ({
        key: `${i}`,
        cells: [
          l.time === undefined ? "" : timestampDate(l.time).toLocaleString(),
          String(l.version),
          l.level,
          l.message,
        ],
      }))}
    />
  );
}

/** FN-034: runs the stored test cases of an uploaded source in the build sandbox, one result per case. */
export function StoredTestsPanel({
  client,
  m,
}: {
  readonly client: TestsClient;
  readonly m: FunctionsMessages;
}): ReactElement {
  const editor = rustEditorMessages(useLingui().i18n);
  const [sourceId, setSourceId] = useState("");
  const [run, setRun] = useState<TestRunState>(idleRun);
  const abort = useRef<AbortController | null>(null);
  useEffect(() => () => abort.current?.abort(), []);
  const start = () => {
    abort.current?.abort();
    const controller = new AbortController();
    abort.current = controller;
    void runStoredTests(client, sourceId.trim(), setRun, controller.signal);
  };
  return (
    <div className="flex flex-col gap-3">
      <Field label={m.sourceLabel}>
        <Input value={sourceId} onChange={(e) => setSourceId(e.target.value)} spellCheck={false} />
      </Field>
      <TestsPanel run={run} canRun={sourceId.trim() !== ""} onRun={start} m={editor} />
    </div>
  );
}
