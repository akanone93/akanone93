// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { TestScenarioResult } from "@plux/api";
import { StatusPill } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { Frame } from "../releases/frame.tsx";
import { Table } from "../security/parts.tsx";
import { useScope } from "../security/use-security.ts";
import { type TestsMessages, testsMessages } from "./tests.messages.ts";
import { useTestRun } from "./use-tests.ts";

const pillOf = (status: string) => (status === "passed" ? "success" : status === "failed" ? "danger" : "neutral");

function statusName(m: TestsMessages, status: string): string {
  return status === "passed" ? m.passed : status === "failed" ? m.failed : m.skipped;
}

function Scenario({ m, scenario }: { readonly m: TestsMessages; readonly scenario: TestScenarioResult }): ReactElement {
  return (
    <details open={scenario.status === "failed"} className="flex flex-col gap-2">
      <summary className="flex items-center gap-3 text-body">
        <span>{scenario.name}</span>
        <StatusPill status={pillOf(scenario.status)} label={statusName(m, scenario.status)} />
        <span className="text-secondary">{`${scenario.durationMs} ms`}</span>
      </summary>
      {scenario.steps.length === 0 ? (
        <p className="text-body text-secondary">{m.noSteps}</p>
      ) : (
        <Table
          caption={scenario.name}
          heads={[m.colStep, m.colStatus, m.colMessage]}
          rows={scenario.steps.map((s) => ({
            key: String(s.index),
            cells: [
              s.description,
              <StatusPill key="s" status={pillOf(s.status)} label={statusName(m, s.status)} />,
              s.message,
            ],
          }))}
        />
      )}
    </details>
  );
}

/** A46: one reported run, with the result of every scenario and step (TST-002). */
export function TestRunPage(): ReactElement {
  const m = testsMessages(useLingui().i18n);
  const { app, env } = useScope();
  const { org = "", runId = "" } = useParams({ strict: false }) as { org?: string; runId?: string };
  const query = useTestRun(app, runId);
  const run = query.data;
  return (
    <Frame
      id="test-run-title"
      title={m.runTitle}
      caption={m.runCaption}
      loading={query.isPending}
      failed={query.isError || run === undefined}
    >
      <Link className="text-body" to="/o/$org/apps/$app/tests" params={{ org, app }} search={{ env }}>
        {m.back}
      </Link>
      <p className="text-body">
        <span className="sr-only">{m.summary}</span>
        {`${m.passed}: ${run?.passed ?? 0} · ${m.failed}: ${run?.failed ?? 0} · ${m.skipped}: ${run?.skipped ?? 0}`}
      </p>
      {(run?.scenarios ?? []).map((s) => (
        <Scenario key={s.id} m={m} scenario={s} />
      ))}
    </Frame>
  );
}
