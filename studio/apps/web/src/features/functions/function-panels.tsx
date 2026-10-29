// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { FunctionCapabilities, FunctionLogLine, FunctionVersion } from "@plux/api";
import { Button, Field, Input, Select } from "@plux/ui";
import { Fragment, type ReactElement, useEffect, useRef, useState } from "react";
import { rustEditorMessages } from "../rust-editor/index.ts";
import { TestsPanel } from "../rust-editor/rust-panels.tsx";
import { idleRun, runStoredTests, type TestRunState, type TestsClient } from "../rust-editor/test-runner.ts";
import { Table } from "../security/parts.tsx";
import type { FunctionsMessages } from "./functions.messages.ts";
import { useEnvironments, useLogs, useMetrics, useVersions } from "./use-functions.ts";

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

/** What was verified about the latest version: signature, tests, provenance and the assurance it requires (FN-009, FN-019, SEC-007). */
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
        <dt className="text-secondary">{m.assuranceRequired}</dt>
        <dd>{v.assurance === "" ? m.assuranceDefault : v.assurance}</dd>
        <dt className="text-secondary">{m.sourceLabel}</dt>
        <dd className="break-all">{v.sourceId}</dd>
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

/** Where the version runs, what its build declared it may use (FN-012) and the contract of its input and output. */
export function CapabilitiesPanel({
  v,
  m,
}: {
  readonly v: FunctionVersion;
  readonly m: FunctionsMessages;
}): ReactElement {
  return (
    <div className="flex flex-col gap-4">
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
      <h3 className="text-body font-medium">{m.capGranted}</h3>
      {v.capabilities === undefined ? (
        <p className="text-body text-secondary">{m.noDeclaration}</p>
      ) : (
        <GrantedList c={v.capabilities} m={m} />
      )}
    </div>
  );
}

/** The host capabilities a version was granted; everything not listed is denied (FN-012). */
function GrantedList({ c, m }: { readonly c: FunctionCapabilities; readonly m: FunctionsMessages }): ReactElement {
  const names = (list: readonly string[]) => (list.length === 0 ? m.capNone : list.join(", "));
  const flag = (on: boolean) => (on ? m.capGrantedYes : m.capGrantedNo);
  const rows: readonly (readonly [string, string])[] = [
    [m.capHttp, names(c.http)],
    [m.capKv, flag(c.kv)],
    [m.capSecrets, names(c.secrets)],
    [m.capFunctions, names(c.functions)],
    [m.capClaims, flag(c.claims)],
  ];
  return (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-body">
      {rows.map(([label, value]) => (
        <Fragment key={label}>
          <dt className="text-secondary">{label}</dt>
          <dd className="break-all">{value}</dd>
        </Fragment>
      ))}
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

/** FN-035: the retained log lines of the function, in the environment chosen from the app's own. */
export function LogsPanel({ scope, m }: { readonly scope: Scope; readonly m: FunctionsMessages }): ReactElement {
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  const environmentId = chosen ?? scope.environmentId;
  const environments = useEnvironments(scope.app);
  const logs = useLogs(scope.app, environmentId, scope.name);
  const lines = logs.data?.lines ?? [];
  const options = (environments.data?.environments ?? []).map((e) => ({ value: e.id, label: e.key }));
  return (
    <div className="flex flex-col gap-3">
      <Field label={m.envLabel}>
        <Select label={m.envLabel} options={options} value={environmentId} onValueChange={setChosen} />
      </Field>
      {logs.isError ? <p className="text-body text-secondary">{m.loadFailed}</p> : <LogTable lines={lines} m={m} />}
    </div>
  );
}

/** The log lines as a table, or the empty state. */
function LogTable({
  lines,
  m,
}: {
  readonly lines: readonly FunctionLogLine[];
  readonly m: FunctionsMessages;
}): ReactElement {
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

/** FN-034: runs the stored test cases of the version's own source in the build sandbox, one result per case. */
export function StoredTestsPanel({
  client,
  sourceId,
  m,
}: {
  readonly client: TestsClient;
  /** The source the latest version was built from; undefined while no version is loaded. */
  readonly sourceId: string | undefined;
  readonly m: FunctionsMessages;
}): ReactElement {
  const editor = rustEditorMessages(useLingui().i18n);
  const [run, setRun] = useState<TestRunState>(idleRun);
  const abort = useRef<AbortController | null>(null);
  useEffect(() => () => abort.current?.abort(), []);
  const source = sourceId ?? "";
  const start = () => {
    abort.current?.abort();
    const controller = new AbortController();
    abort.current = controller;
    void runStoredTests(client, source, setRun, controller.signal);
  };
  return (
    <div className="flex flex-col gap-3">
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-body">
        <dt className="text-secondary">{m.sourceLabel}</dt>
        <dd className="break-all">{source}</dd>
      </dl>
      <TestsPanel run={run} canRun={source !== ""} onRun={start} m={editor} />
    </div>
  );
}
