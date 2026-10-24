// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Transport } from "@connectrpc/connect";
import { createStudioTransport } from "@plux/api";
import { QueryClient } from "@tanstack/react-query";

/**
 * The browser session: the transport, the query cache, and the CSRF token and active organisation
 * held in memory only (SEC-101, STU-002). One instance is created by the app root; nothing here is global.
 */
export interface Session {
  readonly transport: Transport;
  readonly queryClient: QueryClient;
  readonly setCsrfToken: (token: string | undefined) => void;
  readonly setOrganization: (id: string | undefined) => void;
}

/** Creates the session; tests pass their own transport. */
export function createSession(baseUrl: string, transport?: Transport): Session {
  let csrf: string | undefined;
  let organization: string | undefined;
  return {
    transport: transport ?? createStudioTransport({ baseUrl, csrfToken: () => csrf, organization: () => organization }),
    queryClient: new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 30_000 } } }),
    setCsrfToken: (token) => {
      csrf = token;
    },
    setOrganization: (id) => {
      organization = id;
    },
  };
}

const uuidLength = 36;

/** `:org` is the immutable organisation ID optionally followed by a readable slug, which is ignored (R1). */
export function organizationIdOf(param: string): string {
  return param.length > uuidLength && param[uuidLength] === "-" ? param.slice(0, uuidLength) : param;
}
