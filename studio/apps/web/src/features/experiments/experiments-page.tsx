// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { Field, Select, StatusPill } from "@plux/ui";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { Frame } from "../releases/frame.tsx";
import { Table } from "../security/parts.tsx";
import { useEnvironment, usePermissions, useScope } from "../security/use-security.ts";
import { experimentsMessages, stateNames } from "./experiments.messages.ts";
import { useExperiments } from "./use-experiments.ts";

const pills = { 1: "neutral", 2: "success", 3: "warning", 4: "neutral" } as const;

/** A6: the experiments of one environment, filtered by `?status`. */
export function ExperimentsPage(): ReactElement {
  const m = experimentsMessages(useLingui().i18n);
  const { app, env } = useScope();
  const status = (useSearch({ strict: false }) as { status?: number }).status ?? 0;
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const navigate = useNavigate();
  const names = stateNames(m);
  const environment = useEnvironment(app, env);
  const list = useExperiments(app, environment.environment?.id, status as never);
  const canCreate = usePermissions().has("experiments.manage");
  const options = [
    { value: "0", label: m.anyState },
    ...[1, 2, 3, 4].map((s) => ({ value: String(s), label: names[s] ?? "" })),
  ];
  return (
    <Frame
      id="experiments-title"
      title={m.listTitle}
      caption={m.listCaption}
      loading={environment.loading || list.isPending}
      failed={environment.failed || list.isError}
    >
      <div className="flex items-end gap-3">
        <Field label={m.statusLabel}>
          <Select
            options={options}
            value={String(status)}
            onValueChange={(v) =>
              void navigate({
                to: ".",
                replace: true,
                search: (s: object) => ({ ...s, status: v === "0" ? undefined : Number(v) }),
              } as never)
            }
          />
        </Field>
        {canCreate && (
          <Link className="text-body" to="/o/$org/apps/$app/experiments/new" params={{ org, app }} search={{ env }}>
            {m.newLink}
          </Link>
        )}
        <Link className="text-body" to="/o/$org/apps/$app/experiments/flags" params={{ org, app }} search={{ env }}>
          {m.flagsLink}
        </Link>
      </div>
      {(list.data?.experiments ?? []).length === 0 ? (
        <p className="text-body text-secondary">{m.noExperiments}</p>
      ) : (
        <Table
          caption={m.listTitle}
          heads={[m.colKey, m.statusLabel, m.colStarted, m.colWinner]}
          rows={(list.data?.experiments ?? []).map((x) => ({
            key: x.id,
            cells: [
              <Link
                key="k"
                to="/o/$org/apps/$app/experiments/$expId"
                params={{ org, app, expId: x.id } as never}
                search={{ env }}
              >
                {x.key}
              </Link>,
              <StatusPill key="s" status={pills[x.state as 1] ?? "neutral"} label={names[x.state] ?? ""} />,
              x.startedAt === undefined ? m.never : timestampDate(x.startedAt).toLocaleString(),
              x.winner,
            ],
          }))}
        />
      )}
    </Frame>
  );
}
