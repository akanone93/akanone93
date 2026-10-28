// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DevEvent } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { useAppDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import { pagesOf, recordedSteps } from "./session-steps.ts";

/** The Studio dev session the device loop already created for the app (`useStudioDevSession`); never creates one. */
export function useHeldDevSession(appId: string): string | undefined {
  const held = useQuery({
    queryKey: ["studio-dev-session", appId],
    enabled: false,
    staleTime: Number.POSITIVE_INFINITY,
    queryFn: () => Promise.resolve(""),
  });
  return held.data === undefined || held.data === "" ? undefined : held.data;
}

/**
 * The tap steps the held device-loop session recorded, resolved against the app's pages. `onEvent` is fed by
 * the session watcher; `sessionId` is undefined when the device loop has not been opened for this app.
 */
export function useSessionSteps(appId: string) {
  const [events, setEvents] = useState<readonly DevEvent[]>([]);
  const onEvent = useCallback((e: DevEvent) => setEvents((prev) => [...prev, e]), []);
  const project = useAppDocuments(appId);
  const pages = useMemo(() => pagesOf(project.data ?? []), [project.data]);
  const recorded = useMemo(() => recordedSteps(events, pages), [events, pages]);
  return { sessionId: useHeldDevSession(appId), onEvent, recorded };
}
