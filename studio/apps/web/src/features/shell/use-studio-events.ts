// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { type StreamState, watchStudioEvents } from "./studio-events.ts";

/** Runs the events stream for the shell's lifetime; inbox events refresh the inbox queries. */
export function useStudioEvents(): { state: StreamState; restart: () => void } {
  const { session } = useApp();
  const queryClient = useQueryClient();
  const [state, setState] = useState<StreamState>("connecting");
  const [generation, setGeneration] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: generation restarts the watch
  useEffect(() => {
    const controller = new AbortController();
    void watchStudioEvents({
      transport: session.transport,
      signal: controller.signal,
      onState: setState,
      onInbox: () => void queryClient.invalidateQueries({ queryKey: ["inbox"] }),
      onReset: () => void queryClient.invalidateQueries({ queryKey: ["inbox"] }),
    });
    return () => controller.abort();
  }, [session, queryClient, generation]);
  const restart = useCallback(() => setGeneration((g) => g + 1), []);
  return { state, restart };
}
