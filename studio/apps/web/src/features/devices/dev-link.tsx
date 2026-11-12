// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type DevDevice, type DevEvent, DevPhoneService } from "@plux/api";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { StreamState } from "../shell/studio-events.ts";
import { devicesMessages } from "./devices.messages.ts";
import type { Buffer } from "./drawer-model.ts";
import { JoinPrompts, type JoinRequests, reconcileJoins, withJoinEvent } from "./join-prompt.tsx";
import { useDevClient, useStudioDevSession } from "./use-dev-session.ts";
import { useDeviceEvents } from "./use-device-events.ts";
import "./dev-link.css";

/** How often Studio reads the session's devices again, in milliseconds; a connection event reads them at once. */
export const devicesRefresh = 5000;

/** How often Studio reads the open requests of phones to join, in milliseconds. */
export const joinRefresh = 3000;

/** What the link's state says: where the event stream is and how many devices have their connection open. */
export type ConsoleStatus =
  | { readonly kind: "idle" | "connecting" | "reconnecting" | "ended" | "noDevice" }
  | { readonly kind: "live"; readonly devices: number };

/** The link's state from the stream's state and the number of connected devices (DEV-020). */
export function consoleStatus(session: boolean, stream: StreamState | undefined, connected: number): ConsoleStatus {
  if (!session) {
    return { kind: "idle" };
  }
  if (stream === "ended" || stream === "reconnecting") {
    return { kind: stream };
  }
  if (stream !== "live") {
    return { kind: "connecting" };
  }
  return connected === 0 ? { kind: "noDevice" } : { kind: "live", devices: connected };
}

/**
 * An app's link with its devices in Studio (DEV-002, DEV-020, ADR-0104): one dev session, one event stream, the
 * session's devices and the phones asking to join, shared by the app canvas and every plugin of the app.
 */
export interface DevLink {
  readonly appId: string;
  readonly sessionId: string | undefined;
  readonly status: ConsoleStatus;
  readonly stream: StreamState | undefined;
  readonly devices: readonly DevDevice[];
  /** The session's events as the console shows them, paused or not. */
  readonly buffer: Buffer;
  readonly pause: () => void;
  readonly resume: () => void;
  readonly joins: JoinRequests;
  /** The person answered, dismissed or let a request expire: its prompt goes. */
  readonly joinDone: (requestId: string) => void;
  /** Hears every event as it arrives; returns the unsubscribe. */
  readonly listen: (listener: (event: DevEvent) => void) => () => void;
}

const DevLinkContext = createContext<DevLink | undefined>(undefined);

/** The app's link, or undefined outside a {@link DevLinkProvider}. */
export function useDevLink(): DevLink | undefined {
  return useContext(DevLinkContext);
}

/** Runs `listener` on every event of the app's link while mounted. */
export function useDevLinkEvents(listener: (event: DevEvent) => void): void {
  const link = useDevLink();
  const current = useRef(listener);
  current.current = listener;
  const listen = link?.listen;
  useEffect(() => listen?.((event) => current.current(event)), [listen]);
}

/** The phones asking to join, read every few seconds while the session is open, merged with their events. */
function useJoins(sessionId: string | undefined) {
  const { session } = useApp();
  const phones = useMemo(() => createClient(DevPhoneService, session.transport), [session.transport]);
  const [joins, setJoins] = useState<JoinRequests>(() => new Map());
  const dismissed = useRef(new Set<string>());
  const listedBefore = useRef<ReadonlySet<string>>(new Set());
  // Asking also keeps this session the one the person's phones join (ADR-0104), and a prompt shows even when its
  // event never arrived.
  const open = useQuery({
    queryKey: ["dev-join-requests", sessionId],
    enabled: sessionId !== undefined,
    refetchInterval: joinRefresh,
    queryFn: async () => (await phones.listJoinRequests({ sessionId: sessionId ?? "" })).requests,
  });
  useEffect(() => {
    if (open.data !== undefined) {
      const listed = open.data;
      const before = listedBefore.current;
      listedBefore.current = new Set(listed.map((r) => r.requestId));
      setJoins((prev) => reconcileJoins(prev, listed, dismissed.current, before));
    }
  }, [open.data]);
  const onJoinEvent = useCallback((event: DevEvent) => {
    if (event.payload.case === "joinRequest") {
      const value = event.payload.value;
      setJoins((prev) => withJoinEvent(prev, value));
    }
  }, []);
  const joinDone = useCallback((requestId: string) => {
    dismissed.current.add(requestId);
    setJoins((prev) => {
      const next = new Map(prev);
      next.delete(requestId);
      return next;
    });
  }, []);
  return { joins, joinDone, onJoinEvent };
}

/**
 * Opens the app's link for the pages under it (DEV-002, DEV-020, ADR-0104): when `enabled` (the person may start a
 * dev session), the app's Studio session starts as the app opens, so a signed-in phone can join it from the app
 * canvas or any plugin, and its events stream once for all of them.
 */
export function DevLinkProvider({
  appId,
  enabled,
  children,
}: {
  readonly appId: string;
  readonly enabled: boolean;
  readonly children: ReactNode;
}): ReactNode {
  const sessionId = useStudioDevSession(appId, enabled);
  const client = useDevClient();
  const queryClient = useQueryClient();
  const devices = useQuery({
    queryKey: ["dev-devices", sessionId],
    enabled: sessionId !== undefined,
    queryFn: async () => (await client.listDevices({ sessionId: sessionId ?? "" })).devices,
    refetchInterval: devicesRefresh,
  });
  const { joins, joinDone, onJoinEvent } = useJoins(sessionId);
  const listeners = useRef(new Set<(event: DevEvent) => void>());
  const onEvent = useCallback(
    (event: DevEvent) => {
      if (event.payload.case === "connection") {
        void queryClient.invalidateQueries({ queryKey: ["dev-devices", sessionId] });
      }
      onJoinEvent(event);
      for (const listener of listeners.current) {
        listener(event);
      }
    },
    [queryClient, sessionId, onJoinEvent],
  );
  const [stream, setStream] = useState<StreamState | undefined>(undefined);
  const events = useDeviceEvents(sessionId, [], onEvent, setStream);
  const listen = useCallback((listener: (event: DevEvent) => void) => {
    listeners.current.add(listener);
    return () => {
      listeners.current.delete(listener);
    };
  }, []);
  const list = useMemo(() => devices.data ?? [], [devices.data]);
  const connected = list.filter((d) => d.connected).length;
  const value = useMemo<DevLink>(
    () => ({
      appId,
      sessionId,
      status: consoleStatus(sessionId !== undefined, sessionId === undefined ? undefined : stream, connected),
      stream,
      devices: list,
      buffer: events.buffer,
      pause: events.pause,
      resume: events.resume,
      joins,
      joinDone,
      listen,
    }),
    [appId, sessionId, stream, connected, list, events.buffer, events.pause, events.resume, joins, joinDone, listen],
  );
  return <DevLinkContext.Provider value={value}>{children}</DevLinkContext.Provider>;
}

/** The words of a link state. */
export function statusText(m: ReturnType<typeof devicesMessages>, s: ConsoleStatus): string {
  switch (s.kind) {
    case "idle":
      return m.consoleIdle;
    case "connecting":
      return m.consoleConnecting;
    case "reconnecting":
      return m.consoleReconnecting;
    case "ended":
      return m.consoleEnded;
    case "noDevice":
      return m.consoleNoDevice;
    case "live":
      return m.consoleLiveDevices(s.devices);
  }
}

/** The link's state as a pill: green while live, amber while it connects, grey without a device (DEV-020). */
export function DevLinkPill({ status }: { readonly status: ConsoleStatus }): ReactNode {
  const m = devicesMessages(useLingui().i18n);
  return (
    <span
      role="status"
      className="dev-console-live"
      data-live={status.kind === "live" || undefined}
      data-state={status.kind}
    >
      <span className="dev-console-dot" aria-hidden="true" />
      {statusText(m, status)}
    </span>
  );
}

/** The prompts of the phones asking to join the app's session, above whatever holds them. */
export function DevLinkPrompts(): ReactNode {
  const link = useDevLink();
  if (link === undefined) {
    return null;
  }
  return <JoinPrompts requests={link.joins} onDone={link.joinDone} />;
}

/**
 * The app canvas's view of the link: the pill, the connected devices by name, and the prompts of phones asking to
 * join above it. Nothing shows while the person may not start a dev session.
 */
export function DevLinkBar(): ReactNode {
  const link = useDevLink();
  const m = devicesMessages(useLingui().i18n);
  if (link?.sessionId === undefined) {
    return null;
  }
  const names = link.devices.filter((d) => d.connected).map((d) => d.name);
  return (
    <section aria-label={m.linkBar} className="dev-link-bar">
      <DevLinkPrompts />
      <div className="dev-link-row">
        <DevLinkPill status={link.status} />
        {names.length > 0 && <span className="dev-link-devices">{names.join(" · ")}</span>}
      </div>
    </section>
  );
}
