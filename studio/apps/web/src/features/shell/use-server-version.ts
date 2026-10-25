// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { fetchServerVersion, type ServerVersion } from "../../lib/server-version.ts";

const key = ["server-version"] as const;

/** `/version`, read once at start, then again on window focus and whenever the events stream reconnects (E14). */
export function useServerVersion(streamState: string): ServerVersion | undefined {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: key,
    queryFn: () => fetchServerVersion(),
    staleTime: Number.POSITIVE_INFINITY,
    refetchOnWindowFocus: "always",
    retry: false,
  });
  useEffect(() => {
    if (streamState === "live") {
      void queryClient.invalidateQueries({ queryKey: key });
    }
  }, [streamState, queryClient]);
  return query.data;
}
