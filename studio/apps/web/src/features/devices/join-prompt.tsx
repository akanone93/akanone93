// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { timestampMs } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { DevJoinState, DevPhoneService, type JoinRequestEvent, JoinRequestEventSchema } from "@plux/api";
import { Button, Icon } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useEffect, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { joinMessages } from "./join.messages.ts";
import { secondsLeft } from "./pairing-dialog.tsx";

/** The latest event of each phone's request to join the session, by request (DEV-002, ADR-0104). */
export type JoinRequests = ReadonlyMap<string, JoinRequestEvent>;

/** `requests` with `event` as the latest of its request. */
export function withJoinEvent(requests: JoinRequests, event: JoinRequestEvent): JoinRequests {
  const next = new Map(requests);
  next.set(event.requestId, event);
  return next;
}

/** The open requests as `ListJoinRequests` answers them, waiting or with a wrong number. */
export type OpenJoinRequest = Pick<JoinRequestEvent, "requestId" | "state" | "phoneName" | "expiresAt">;

/**
 * `requests` brought in line with what Studio last read of the open ones (ADR-0104): a request it lists is shown,
 * unless the person dismissed it; one it listed before and lists no more has ended (joined, denied or expired). A
 * request an event brought and no listing has named yet stays: its listing may simply be older than the event.
 */
export function reconcileJoins(
  requests: JoinRequests,
  open: readonly OpenJoinRequest[],
  dismissed: ReadonlySet<string>,
  listedBefore: ReadonlySet<string>,
): JoinRequests {
  const next = new Map(requests);
  const listed = new Set(open.map((r) => r.requestId));
  for (const id of requests.keys()) {
    if (listedBefore.has(id) && !listed.has(id)) {
      next.delete(id);
    }
  }
  for (const r of open) {
    if (!dismissed.has(r.requestId) && next.get(r.requestId)?.state !== r.state) {
      next.set(r.requestId, create(JoinRequestEventSchema, r));
    }
  }
  return next;
}

/** The requests a prompt is shown for: those waiting for a number, and those whose number was wrong. */
export function promptedJoins(requests: JoinRequests): JoinRequestEvent[] {
  return [...requests.values()].filter((e) => e.state === DevJoinState.WAITING || e.state === DevJoinState.FAILED);
}

function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}

/**
 * A signed-in phone asking to join this dev session (DEV-002, ADR-0104). While the request waits, the two-digit number
 * to pick on the phone, which the server gives only to the session's person; a request that expired, was answered or
 * replayed from an earlier visit shows nothing. After a wrong pick the phone stays out until the person lets it try
 * again.
 */
function JoinCard({
  event,
  onDone,
}: {
  readonly event: JoinRequestEvent;
  readonly onDone: () => void;
}): ReactElement | null {
  const m = joinMessages(useLingui().i18n);
  const { session } = useApp();
  const client = useMemo(() => createClient(DevPhoneService, session.transport), [session.transport]);
  const now = useNow();
  const [failed, setFailed] = useState(false);
  const status = useQuery({
    queryKey: ["dev-join-request", event.requestId, event.state],
    queryFn: () => client.getJoinRequest({ requestId: event.requestId }),
    retry: false,
    staleTime: Number.POSITIVE_INFINITY,
  });
  const expiresAt = event.expiresAt === undefined ? 0 : timestampMs(event.expiresAt);
  const waiting = event.state === DevJoinState.WAITING && status.data?.state === DevJoinState.WAITING;
  const left = secondsLeft(expiresAt, now);
  useEffect(() => {
    if (event.state === DevJoinState.WAITING && expiresAt > 0 && left === 0) {
      onDone();
    }
  }, [event.state, expiresAt, left, onDone]);
  const act = (call: () => Promise<unknown>) => {
    setFailed(false);
    call().then(onDone, () => setFailed(true));
  };
  if (waiting && (status.data?.number ?? 0) > 0) {
    const n = status.data?.number ?? 0;
    return (
      <div role="alertdialog" aria-labelledby={`join-${event.requestId}`} className="dev-join" data-state="waiting">
        <div className="dev-join-head">
          <Icon name="device-mobile" />
          <span id={`join-${event.requestId}`} className="dev-join-title">
            {m.asks(event.phoneName)}
          </span>
        </div>
        <output className="dev-join-number" aria-label={m.number(n)}>
          {String(n)}
        </output>
        <p className="dev-join-body">{m.pick}</p>
        <div className="dev-join-actions">
          <span className="dev-join-meta">{m.expires(left)}</span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => act(() => client.denyJoinRequest({ requestId: event.requestId }))}
          >
            {m.deny}
          </Button>
        </div>
        {failed && (
          <p role="alert" className="dev-join-meta">
            {m.failed}
          </p>
        )}
      </div>
    );
  }
  if (event.state === DevJoinState.FAILED && status.data?.state === DevJoinState.FAILED) {
    return (
      <div role="alertdialog" aria-labelledby={`join-${event.requestId}`} className="dev-join" data-state="failed">
        <div className="dev-join-head">
          <Icon name="warning-circle" />
          <span id={`join-${event.requestId}`} className="dev-join-title">
            {m.wrong(event.phoneName)}
          </span>
        </div>
        <p className="dev-join-body">{m.wrongBody}</p>
        <div className="dev-join-actions">
          <Button size="sm" variant="ghost" onClick={onDone}>
            {m.dismiss}
          </Button>
          <Button size="sm" onClick={() => act(() => client.allowJoinRetry({ requestId: event.requestId }))}>
            {m.retry}
          </Button>
        </div>
        {failed && (
          <p role="alert" className="dev-join-meta">
            {m.failed}
          </p>
        )}
      </div>
    );
  }
  return null;
}

/** The prompts of the phones asking to join, above the console; each goes once answered, expired or dismissed. */
export function JoinPrompts({
  requests,
  onDone,
}: {
  readonly requests: JoinRequests;
  readonly onDone: (requestId: string) => void;
}): ReactElement | null {
  const shown = promptedJoins(requests);
  if (shown.length === 0) {
    return null;
  }
  return (
    <div className="dev-join-stack">
      {shown.map((e) => (
        <JoinCard key={`${e.requestId}:${e.state}`} event={e} onDone={() => onDone(e.requestId)} />
      ))}
    </div>
  );
}
