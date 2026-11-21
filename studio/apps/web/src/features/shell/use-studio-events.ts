// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useCallback, useEffect, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { type StreamState, watchStudioEvents } from "./studio-events.ts";

/**
 * Runs the events stream of the organisation the page is in, or of the home one on a page outside any (ADR-0096); a
 * move to another organisation restarts it there. Inbox events refresh the inbox queries. Without any organisation to
 * name (a person in none) there is no stream to run, so none is started rather than one the server refuses forever.
 */
export function useStudioEvents(): { state: StreamState; restart: () => void } {
  const { session } = useApp();
  const queryClient = useQueryClient();
  const org = (useParams({ strict: false }) as { org?: string }).org;
  const scope = org ?? session.home();
  const [state, setState] = useState<StreamState>("connecting");
  const [generation, setGeneration] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: generation and the organisation restart the watch
  useEffect(() => {
    if (scope === undefined) {
      return;
    }
    const controller = new AbortController();
    void watchStudioEvents({
      transport: session.transport,
      signal: controller.signal,
      onState: setState,
      onInbox: () => void queryClient.invalidateQueries({ queryKey: ["inbox"] }),
      onReset: () => void queryClient.invalidateQueries({ queryKey: ["inbox"] }),
    });
    return () => controller.abort();
  }, [session, queryClient, generation, scope]);
  const restart = useCallback(() => setGeneration((g) => g + 1), []);
  return { state, restart };
}
