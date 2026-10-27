// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import {
  AppService,
  DeviceService,
  DocumentService,
  IdentityService,
  ManifestService,
  SecurityAdminService,
} from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useParams, useSearch } from "@tanstack/react-router";
import { loadUser } from "../../lib/guards.ts";
import { useApp } from "../../lib/use-app.tsx";
import { defaultEnvironment } from "./security.schema.ts";
import { assertionFields, requestOptions } from "./step-up.ts";

const appDocument = "app.json";

/** The app id of the path and the environment key of `?env` (default `development`). */
export function useScope() {
  const app = (useParams({ strict: false }) as { app?: string }).app ?? "";
  const env = (useSearch({ strict: false }) as { env?: string }).env ?? defaultEnvironment;
  return { app, env };
}

/** The signed-in person's permissions (F14): actions the person lacks are disabled, never hidden mid-page. */
export function usePermissions(): ReadonlySet<string> {
  const { session } = useApp();
  const user = useQuery({ queryKey: ["current-user"], queryFn: () => loadUser(session) });
  return new Set(user.data?.permissions ?? []);
}

/** The environment of the scope, found by its key. */
export function useEnvironment(app: string, env: string) {
  const { session } = useApp();
  const list = useQuery({
    queryKey: ["environments", app],
    queryFn: () => createClient(AppService, session.transport).listEnvironments({ appId: app }),
  });
  const found = list.data?.environments.find((e) => e.key === env);
  return {
    environment: found,
    loading: list.isPending,
    failed: list.isError || (list.isSuccess && found === undefined),
  };
}

export function useDevices(app: string, environmentId: string | undefined) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["devices", app, environmentId],
    enabled: environmentId !== undefined,
    queryFn: () =>
      createClient(DeviceService, session.transport).listDevices({ appId: app, environmentId: environmentId ?? "" }),
  });
}

export function useDevice(id: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["device", id],
    queryFn: () => createClient(DeviceService, session.transport).getDevice({ id }),
  });
}

export function useRevokeDevice() {
  const { session } = useApp();
  return async (deviceId: string, reason: string) => {
    await createClient(DeviceService, session.transport).revokeDevice({ deviceId, reason });
    await session.queryClient.invalidateQueries({ queryKey: ["device", deviceId] });
    await session.queryClient.invalidateQueries({ queryKey: ["devices"] });
  };
}

export function useEffectiveSecurity(app: string, environmentId: string | undefined) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["security-config", app, environmentId],
    enabled: environmentId !== undefined,
    queryFn: () =>
      createClient(SecurityAdminService, session.transport).getEffectiveSecurityConfig({
        appId: app,
        environmentId: environmentId ?? "",
      }),
  });
}

/** Sends a profile change with the overrides the environment already has, so the change never resets them. */
export function useSetProfile() {
  const { session } = useApp();
  return async (
    app: string,
    environmentId: string,
    profile: string,
    overridesJson: Uint8Array,
    expectedVersion: bigint,
  ) => {
    await createClient(SecurityAdminService, session.transport).setSecurityConfig({
      appId: app,
      environmentId,
      profile,
      overridesJson,
      expectedVersion,
    });
    await session.queryClient.invalidateQueries({ queryKey: ["security-config"] });
  };
}

/** The WebAuthn step-up of the signed-in session (SEC-103): begin returns the challenge, complete verifies the assertion. */
export function useStepUp() {
  const { session } = useApp();
  const identity = createClient(IdentityService, session.transport);
  return {
    begin: async () => {
      const { requestId, optionsJson } = await identity.beginStepUp({});
      return { requestId, options: requestOptions(optionsJson) };
    },
    complete: async (requestId: string, credential: Credential) => {
      await identity.completeStepUp({ requestId, ...assertionFields(credential) });
    },
  };
}

/** The public signing keys and root metadata of an environment (SEC-051): ids and states only. */
export function useRootKeys(app: string, env: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["root-keys", app, env],
    queryFn: () => createClient(ManifestService, session.transport).getRootKeys({ appId: app, environment: env }),
  });
}

/** The app document, whose `capabilities` are the approved grants (SEC-080). */
export function useAppDocument(app: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["app-document", app],
    queryFn: async () => {
      const { document } = await createClient(DocumentService, session.transport).getDocument({
        appId: app,
        pluginId: "",
        path: appDocument,
      });
      const json = JSON.parse(new TextDecoder().decode(document?.content)) as Record<string, unknown>;
      return { json, revision: document?.revision ?? 0n };
    },
  });
}

export function useSaveCapabilities() {
  const { session } = useApp();
  return async (app: string, json: Record<string, unknown>, revision: bigint) => {
    await createClient(DocumentService, session.transport).putDocument({
      appId: app,
      pluginId: "",
      path: appDocument,
      content: new TextEncoder().encode(JSON.stringify(json)),
      ifRevision: revision,
    });
    await session.queryClient.invalidateQueries({ queryKey: ["app-document", app] });
  };
}
