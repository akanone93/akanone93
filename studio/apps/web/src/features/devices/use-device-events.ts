// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { type DevEvent, DevSessionService } from "@plux/api";
import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { StreamState } from "../shell/studio-events.ts";
import { type Buffer, emptyBuffer, pause, push, resume } from "./drawer-model.ts";
import { watchDeviceEvents } from "./watch-device-events.ts";

type Action = { type: "event"; event: DevEvent } | { type: "pause" } | { type: "resume" } | { type: "clear" };

function reduce(buffer: Buffer, action: Action): Buffer {
  switch (action.type) {
    case "event":
      return push(buffer, action.event);
    case "pause":
      return pause(buffer);
    case "resume":
      return resume(buffer);
    case "clear":
      return emptyBuffer;
  }
}

/**
 * Streams a dev session's events (DEV-020, DEV-021), resuming after the last sequence and retrying with
 * the shared backoff, like the studio events stream. Pausing freezes the view, not the stream.
 */
export function useDeviceEvents(sessionId: string, deviceIds: readonly string[], onEvent?: (event: DevEvent) => void) {
  const { session } = useApp();
  const client = useMemo(() => createClient(DevSessionService, session.transport), [session.transport]);
  const [buffer, dispatch] = useReducer(reduce, emptyBuffer);
  const [state, setState] = useState<StreamState>("connecting");
  const devices = deviceIds.join(",");
  const seen = useRef(onEvent);
  seen.current = onEvent;
  // biome-ignore lint/correctness/useExhaustiveDependencies: the device list is compared by its joined value
  useEffect(() => {
    const controller = new AbortController();
    dispatch({ type: "clear" });
    void watchDeviceEvents({
      client,
      sessionId,
      deviceIds,
      signal: controller.signal,
      onState: setState,
      onEvent: (event) => {
        dispatch({ type: "event", event });
        seen.current?.(event);
      },
    });
    return () => controller.abort();
  }, [client, sessionId, devices]);
  return {
    buffer,
    state,
    pause: () => dispatch({ type: "pause" }),
    resume: () => dispatch({ type: "resume" }),
  };
}
