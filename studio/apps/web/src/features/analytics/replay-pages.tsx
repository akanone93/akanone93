// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { Replay, ReplayEvent } from "@plux/api";
import { Button } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import { type ReactElement, useEffect, useMemo, useState } from "react";
import { Frame } from "../releases/frame.tsx";
import { Table } from "../security/parts.tsx";
import { analyticsMessages } from "./analytics.messages.ts";
import { AreaNav, useAnalyticsScope } from "./parts.tsx";
import { extentOf, frameAt, type ReplayFrame, tapPoint } from "./replay-model.ts";
import { useReplay, useReplays } from "./use-analytics.ts";

/** How often a playing replay advances, and by how much. */
const TICK_MS = 100;

const startedOf = (r: Replay) => (r.startedAt === undefined ? "" : timestampDate(r.startedAt).toISOString());

/** The A47 list: the replays of the environment, newest first. */
export function AnalyticsReplaysPage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { org, app, search, env } = useAnalyticsScope();
  const replays = useReplays(app, env);
  const rows = replays.data ?? [];
  return (
    <Frame
      id="replays-title"
      title={m.replaysTitle}
      caption={m.replaysCaption}
      loading={replays.isPending}
      failed={replays.isError}
    >
      <AreaNav />
      {rows.length === 0 && <p className="text-body text-secondary">{m.noReplays}</p>}
      <Table
        caption={m.replaysTitle}
        heads={[m.replayStarted, m.replayRoute, m.replayDuration, m.replayEvents, m.replayTruncated]}
        rows={rows.map((r) => ({
          key: r.id,
          cells: [
            <Link
              key="open"
              to="/o/$org/apps/$app/analytics/replays/$replayId"
              params={{ org, app, replayId: r.id } as never}
              search={search}
            >
              {startedOf(r)}
            </Link>,
            r.route,
            r.durationMs,
            r.eventCount,
            r.truncated ? "✓" : "",
          ],
        }))}
      />
    </Frame>
  );
}

/** The recorded layout of one moment, drawn to scale; hidden nodes show their rectangle only. */
function ReplayCanvas({ frame, label, hidden }: { frame: ReplayFrame; label: string; hidden: string }): ReactElement {
  const { width, height } = extentOf(frame.nodes);
  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${width} ${height}`} className="max-h-96 w-full border">
      <title>{label}</title>
      {frame.nodes.map((n) => (
        <g key={n.path} data-node={n.path} data-masked={n.masked ? "true" : "false"}>
          <rect
            x={n.x}
            y={n.y}
            width={n.width}
            height={n.height}
            fill={n.masked ? "currentColor" : "none"}
            fillOpacity={n.masked ? 0.12 : 0}
            stroke="currentColor"
            strokeOpacity={0.5}
            vectorEffect="non-scaling-stroke"
          />
          {n.masked && n.height > 12 && (
            <text x={n.x + 4} y={n.y + 12} fontSize={10}>
              {hidden}
            </text>
          )}
        </g>
      ))}
      {frame.taps.map((t) => {
        const p = tapPoint(t, frame.nodes);
        return p === undefined ? null : (
          <circle
            key={`${t.nodePath}-${t.fx}-${t.fy}`}
            data-tap={t.nodePath}
            cx={p.x}
            cy={p.y}
            r={8}
            fill="currentColor"
            fillOpacity={0.4}
          />
        );
      })}
    </svg>
  );
}

/** Advances [at] while [playing], and stops at [end]. */
function usePlayback(end: number) {
  const [at, setAt] = useState(0);
  const [playing, setPlaying] = useState(false);
  useEffect(() => {
    if (!playing) {
      return;
    }
    const timer = setInterval(() => {
      setAt((v) => Math.min(end, v + TICK_MS));
    }, TICK_MS);
    return () => clearInterval(timer);
  }, [playing, end]);
  useEffect(() => {
    if (at >= end) {
      setPlaying(false);
    }
  }, [at, end]);
  return { at, setAt, playing, toggle: () => setPlaying((p) => !p) };
}

const lastAt = (events: readonly ReplayEvent[], duration: number) =>
  Math.max(
    duration,
    events.reduce((n, e) => Math.max(n, e.atMs), 0),
  );

/** A47: plays one replay back on a canvas with a timeline (ANL-050). */
export function AnalyticsReplayPage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { org, app, search } = useAnalyticsScope();
  const { replayId } = useParams({ strict: false }) as { replayId: string };
  const q = useReplay(app, replayId);
  const events = q.data?.events;
  const end = lastAt(events ?? [], q.data?.replay?.durationMs ?? 0);
  const { at, setAt, playing, toggle } = usePlayback(end);
  const frame = useMemo(() => frameAt(events ?? [], at), [events, at]);
  return (
    <Frame id="replay-title" title={m.replayTitle} loading={q.isPending} failed={q.isError}>
      <AreaNav />
      <Link to="/o/$org/apps/$app/analytics/replays" params={{ org, app }} search={search}>
        {m.backReplays}
      </Link>
      <p className="text-body text-secondary">{frame.route}</p>
      <ReplayCanvas frame={frame} label={m.replayCanvas} hidden={m.replayMasked} />
      <div className="flex items-center gap-3">
        <Button onClick={toggle}>{playing ? m.replayPause : m.replayPlay}</Button>
        <input
          type="range"
          aria-label={m.replayTimeline}
          className="w-full"
          min={0}
          max={end}
          step={TICK_MS}
          value={at}
          onChange={(e) => setAt(Number(e.target.value))}
        />
      </div>
      <Table
        caption={m.replayState}
        heads={[m.replayStateName, m.replayStateType, m.replayStateValue]}
        rows={[...frame.state].map(([name, s]) => ({ key: name, cells: [name, s.type, s.value] }))}
      />
    </Frame>
  );
}
