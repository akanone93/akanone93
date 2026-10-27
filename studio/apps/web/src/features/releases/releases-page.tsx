// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { Field, Input, StatusPill } from "@plux/ui";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { Table } from "../security/parts.tsx";
import { useEnvironment, useScope } from "../security/use-security.ts";
import { Frame } from "./frame.tsx";
import { releasesMessages, stateNames } from "./releases.messages.ts";
import { defaultChannel } from "./releases.schema.ts";
import { useReleases, useRollouts } from "./use-releases.ts";

/** A3: the releases of one environment, newest first, with the rollout of each on the channel `?channel`. */
export function ReleasesPage(): ReactElement {
  const m = releasesMessages(useLingui().i18n);
  const { app, env } = useScope();
  const channel = (useSearch({ strict: false }) as { channel?: string }).channel ?? defaultChannel;
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const navigate = useNavigate();
  const environment = useEnvironment(app, env);
  const releases = useReleases(app, environment.environment?.id);
  const rollouts = useRollouts(app);
  const here = (rollouts.data?.rollouts ?? []).filter((r) => r.environmentKey === env && r.channel === channel);
  return (
    <Frame
      id="releases-title"
      title={m.timelineTitle}
      caption={m.timelineCaption}
      loading={environment.loading || releases.isPending || rollouts.isPending}
      failed={environment.failed || releases.isError || rollouts.isError}
    >
      <div className="flex items-end gap-3">
        <Field label={m.channelLabel}>
          <Input
            value={channel}
            onChange={(e) =>
              void navigate({
                to: ".",
                replace: true,
                search: (s: object) => ({
                  ...s,
                  channel: e.target.value === defaultChannel ? undefined : e.target.value,
                }),
              } as never)
            }
          />
        </Field>
        <Link className="text-body" to="/o/$org/apps/$app/releases/kill-switch" params={{ org, app }} search={{ env }}>
          {m.killSwitchLink}
        </Link>
      </div>
      {(releases.data?.releases ?? []).length === 0 ? (
        <p className="text-body text-secondary">{m.noReleases}</p>
      ) : (
        <Table
          caption={m.timelineTitle}
          heads={[m.colRelease, m.colCreated, m.colNotes, m.colRollout, m.colShare]}
          rows={(releases.data?.releases ?? []).map((r) => {
            const rollout = here.find((x) => x.candidateSequence === r.sequence);
            return {
              key: r.id,
              cells: [
                <Link
                  key="r"
                  to="/o/$org/apps/$app/releases/$releaseId"
                  params={{ org, app, releaseId: String(r.sequence) } as never}
                  search={{ env }}
                >
                  {String(r.sequence)}
                </Link>,
                r.createdAt === undefined ? m.never : timestampDate(r.createdAt).toLocaleString(),
                r.rollbackOf > 0n ? m.rollbackOf(String(r.rollbackOf)) : r.notes,
                rollout === undefined ? (
                  m.noRollout
                ) : (
                  <StatusPill key="s" status={pillOf(rollout.state)} label={stateNames(m)[rollout.state] ?? ""} />
                ),
                rollout === undefined ? "" : `${rollout.basisPoints / 100}%`,
              ],
            };
          })}
        />
      )}
    </Frame>
  );
}

export function pillOf(state: number): "success" | "warning" | "danger" | "neutral" {
  if (state === 2 || state === 5) {
    return "success";
  }
  if (state === 3 || state === 4) {
    return "warning";
  }
  return state === 7 || state === 8 ? "danger" : "neutral";
}
