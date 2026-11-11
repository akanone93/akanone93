// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Transport } from "@connectrpc/connect";
import {
  createStudioTransport,
  DevPhoneService,
  NotificationService,
  PreferencesService,
  withStudioHeaders,
} from "@plux/api";
import { QueryClient } from "@tanstack/react-query";

/**
 * The browser session: the transport, the query cache, and the CSRF token and active organisation
 * held in memory only (SEC-101, STU-002). One instance is created by the app root; nothing here is global.
 */
export interface Session {
  readonly transport: Transport;
  readonly queryClient: QueryClient;
  readonly setCsrfToken: (token: string | undefined) => void;
  /** Names the organisation of the page in every request; undefined on a page outside any. */
  readonly setOrganization: (id: string | undefined) => void;
  /** The organisation requests name, if any. */
  readonly organization: () => string | undefined;
  /**
   * The person's home organisation: the last one a page named, kept on the pages outside any (the profile, the
   * installation). The sidebar shows its navigation, and the person's own preferences and inbox are read in it.
   */
  readonly home: () => string | undefined;
  /** Sets the home organisation, after a reload on a page outside any. */
  readonly setHome: (id: string | undefined) => void;
}

/**
 * The services whose calls are the person's own rather than the page's: preferences, the inbox and the phones signed
 * in to the Plux Dev app (ADR-0104) are kept per organisation on the server, and a page outside any, such as the
 * profile, still reaches them in the home organisation.
 */
const personal: ReadonlySet<string> = new Set([
  PreferencesService.typeName,
  NotificationService.typeName,
  DevPhoneService.typeName,
]);

/** Creates the session; tests pass their own transport, which gets the same headers. */
export function createSession(baseUrl: string, transport?: Transport): Session {
  let csrf: string | undefined;
  let organization: string | undefined;
  let home: string | undefined;
  const headers = {
    csrfToken: () => csrf,
    organization: (service: string) => organization ?? (personal.has(service) ? home : undefined),
  };
  return {
    transport:
      transport === undefined ? createStudioTransport({ baseUrl, ...headers }) : withStudioHeaders(transport, headers),
    queryClient: new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 30_000 } } }),
    setCsrfToken: (token) => {
      csrf = token;
    },
    setOrganization: (id) => {
      organization = id;
      if (id !== undefined) {
        home = id;
      }
    },
    organization: () => organization,
    home: () => home,
    setHome: (id) => {
      home = id;
    },
  };
}

const uuidLength = 36;

/** `:org` is the immutable organisation ID optionally followed by a readable slug, which is ignored (R1). */
export function organizationIdOf(param: string): string {
  return param.length > uuidLength && param[uuidLength] === "-" ? param.slice(0, uuidLength) : param;
}
