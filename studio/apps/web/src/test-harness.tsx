// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import type { Messages } from "@lingui/core";
import {
  DocumentSchema,
  DocumentService,
  FederationService,
  GetCurrentUserResponseSchema,
  GovernanceService,
  IdentityService,
  InboxItemSchema,
  MemberSchema,
  NotificationService,
  OrganizationSchema,
  OrgService,
  PluginSchema,
  PluginService,
  PreferencesSchema,
  PreferencesService,
  UserSchema,
} from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { App } from "./app.tsx";
import type { SurfaceFactory } from "./features/plugin-canvas/surface.ts";
import type { ValidatorFactory } from "./features/problems/validator-client.ts";
import { I18nProvider } from "./i18n-provider.tsx";
import { createSession } from "./lib/session.ts";
import { configureZod } from "./lib/zod.ts";
import { createAppRouter } from "./router.tsx";

export const orgA = "0192f000-0000-7000-8000-00000000000a";
export const orgB = "0192f000-0000-7000-8000-00000000000b";

export interface Upstream {
  signedIn?: boolean;
  permissions?: string[];
  installationAdmin?: boolean;
  csrfToken?: string;
  memberships?: string[];
  stored?: { theme: string; language: string; density: string };
  explained?: { permission: string }[];
  inbox?: { id: string; eventType: string; title: string; read: boolean }[];
  documents?: { kind: string; path: string; json: unknown }[];
  surface?: SurfaceFactory;
  validator?: ValidatorFactory;
  /** What ListDocuments returns for the whole app (no plugin); defaults to `documents`. */
  /** The app's plugins (ids) and their documents, for ListPlugins and ListDocuments. */
  pluginDocuments?: Record<string, { kind: string; path: string; json: unknown }[]>;
  appDocuments?: { kind: string; path: string; json: unknown }[];
  updates?: { paths: string[]; theme: string; language: string; density: string }[];
}

/** An in-memory upstream for the shell: the same RPCs the BFF forwards. */
export function createUpstream(state: Upstream) {
  const stored = state.stored ?? { theme: "", language: "", density: "" };
  return createRouterTransport(({ service }) => {
    service(IdentityService, {
      getCurrentUser: () => {
        if (state.signedIn === false) {
          throw new ConnectError("no session", Code.Unauthenticated);
        }
        return create(GetCurrentUserResponseSchema, {
          user: create(UserSchema, { id: "u1", displayName: "Ada", email: "ada@example.org" }),
          memberships: (state.memberships ?? [orgA, orgB]).map((id) =>
            create(MemberSchema, { organizationId: id, userId: "u1" }),
          ),
          permissions: state.permissions ?? [],
          installationAdmin: state.installationAdmin ?? false,
          csrfToken: state.csrfToken ?? "",
        });
      },
    });
    service(OrgService, {
      listOrganizations: () => ({
        organizations: [
          create(OrganizationSchema, { id: orgA, key: "alpha", name: "Alpha Org" }),
          create(OrganizationSchema, { id: orgB, key: "beta", name: "Beta Org" }),
        ],
      }),
    });
    service(PreferencesService, {
      getPreferences: () => ({ preferences: create(PreferencesSchema, stored) }),
      updatePreferences: (req) => {
        Object.assign(stored, { ...req.preferences });
        state.updates?.push({ paths: req.updateMask?.paths ?? [], ...stored });
        return { preferences: create(PreferencesSchema, stored) };
      },
    });
    service(GovernanceService, {
      explainAccess: (req) => {
        state.explained?.push({ permission: req.permission });
        return { decision: "deny" };
      },
    });
    service(NotificationService, {
      listInbox: (req) => {
        const items = (state.inbox ?? []).filter((i) => !req.unreadOnly || !i.read);
        return {
          items: items.map((i) => create(InboxItemSchema, i)),
          unread: BigInt(items.filter((i) => !i.read).length),
        };
      },
    });
    service(PluginService, {
      listPlugins: () => ({
        plugins: Object.keys(state.pluginDocuments ?? {}).map((id) => create(PluginSchema, { id, key: id })),
      }),
    });
    service(DocumentService, {
      listDocuments: (req) => ({
        documents: (req.pluginId === ""
          ? (state.appDocuments ?? state.documents ?? [])
          : (state.pluginDocuments?.[req.pluginId] ?? state.documents ?? [])
        ).map((d) =>
          create(DocumentSchema, {
            kind: d.kind,
            path: d.path,
            content: new TextEncoder().encode(JSON.stringify(d.json)),
          }),
        ),
      }),
    });
    service(FederationService, { listSignInProviders: () => ({ providers: [] }) });
  });
}

/** Renders the whole app at `path` against the in-memory upstream. */
export function renderApp(path: string, state: Upstream = {}) {
  configureZod();
  const session = createSession("/api", createUpstream(state));
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  const history = createMemoryHistory({ initialEntries: [path] });
  const router = createAppRouter(
    {
      session,
      ...(state.surface !== undefined && { createSurface: state.surface }),
      createValidator:
        state.validator ?? (() => ({ load: async () => [], validatePage: async () => [], dispose: () => undefined })),
    },
    history,
  );
  const utils = render(
    <I18nProvider studio={studio}>
      <App session={session} router={router} />
    </I18nProvider>,
  );
  return { ...utils, router, session };
}

/** The English source catalogue, parsed from the extracted PO file, in Lingui's compiled form. */
export function loadSourceCatalogue(): Messages {
  const text = readFileSync(new URL("./locales/en/messages.po", import.meta.url), "utf-8");
  const out: Messages = {};
  for (const m of text.matchAll(/msgid "([^"]+)"\nmsgstr "([^"]*)"/g)) {
    const id = m[1];
    const message = m[2];
    if (id !== undefined && message !== undefined) {
      const parts = message.split(/\{(\w+)\}/);
      out[id] =
        parts.length === 1
          ? message
          : (parts.map((part, i) => (i % 2 === 1 ? [part] : part)).filter((p) => p !== "") as Messages[string]);
    }
  }
  return out;
}
