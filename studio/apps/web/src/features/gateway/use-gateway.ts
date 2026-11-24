// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { type ConfigValue, SecurityAdminService, type Upstream, type UserIdentityProvider } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

/**
 * The gateway of one app environment (build log row 199): its upstreams (SEC-030), its server-held configuration
 * values (DAT-090) and the identity provider whose user tokens its devices exchange (SEC-026). Reading needs
 * `app.read`; every change needs `gateway.manage` and a fresh security-key step-up (SEC-103, ADR-0108), which the
 * session's step-up host asks for when the server refuses for want of one.
 */

/** How many entries one list call returns: an environment's upstreams and values fit one page. */
const PAGE_SIZE = 500;

export function useUpstreams(app: string, environmentId: string | undefined) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["gateway-upstreams", app, environmentId],
    enabled: environmentId !== undefined,
    queryFn: () =>
      createClient(SecurityAdminService, session.transport).listUpstreams({
        appId: app,
        environmentId: environmentId ?? "",
        page: { pageSize: PAGE_SIZE },
      }),
  });
}

export function useConfigValues(app: string, environmentId: string | undefined) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["gateway-config", app, environmentId],
    enabled: environmentId !== undefined,
    queryFn: () =>
      createClient(SecurityAdminService, session.transport).listConfigValues({
        appId: app,
        environmentId: environmentId ?? "",
        page: { pageSize: PAGE_SIZE },
      }),
  });
}

/** The environment's user identity provider, or null when it trusts none. */
export function useIdentityProvider(app: string, environmentId: string | undefined) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["gateway-idp", app, environmentId],
    enabled: environmentId !== undefined,
    queryFn: async (): Promise<UserIdentityProvider | null> => {
      try {
        const got = await createClient(SecurityAdminService, session.transport).getUserIdentityProvider({
          appId: app,
          environmentId: environmentId ?? "",
        });
        return got.provider ?? null;
      } catch (e) {
        if (e instanceof ConnectError && e.code === Code.NotFound) {
          return null;
        }
        throw e;
      }
    },
  });
}

/** The changes of the gateway's three parts; each refreshes what it changed. */
export function useGatewayChanges() {
  const { session } = useApp();
  const admin = createClient(SecurityAdminService, session.transport);
  const refresh = (key: string) => session.queryClient.invalidateQueries({ queryKey: [key] });
  return {
    saveUpstream: async (upstream: Upstream) => {
      await (upstream.id === "" ? admin.createUpstream({ upstream }) : admin.updateUpstream({ upstream }));
      await refresh("gateway-upstreams");
    },
    deleteUpstream: async (id: string) => {
      await admin.deleteUpstream({ id });
      await refresh("gateway-upstreams");
    },
    saveValue: async (app: string, environmentId: string, value: ConfigValue, expectedVersion: bigint) => {
      await admin.setConfigValue({ appId: app, environmentId, value, expectedVersion });
      await refresh("gateway-config");
    },
    deleteValue: async (app: string, environmentId: string, key: string) => {
      await admin.deleteConfigValue({ appId: app, environmentId, key });
      await refresh("gateway-config");
    },
    saveProvider: async (provider: UserIdentityProvider) => {
      await admin.setUserIdentityProvider({ provider });
      await refresh("gateway-idp");
    },
    deleteProvider: async (app: string, environmentId: string) => {
      await admin.deleteUserIdentityProvider({ appId: app, environmentId });
      await refresh("gateway-idp");
    },
  };
}
