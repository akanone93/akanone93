// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import type { Messages } from "@lingui/core";
import {
  FederationService,
  GetCurrentUserResponseSchema,
  GovernanceService,
  IdentityService,
  MemberSchema,
  OrganizationSchema,
  OrgService,
  PreferencesSchema,
  PreferencesService,
  UserSchema,
} from "@plux/api";
import { createStudioI18n } from "@plux/i18n";
import { createMemoryHistory } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { App } from "./app.tsx";
import { I18nProvider } from "./i18n-provider.tsx";
import { createSession } from "./lib/session.ts";
import { configureZod } from "./lib/zod.ts";
import { createAppRouter } from "./router.tsx";

export const orgA = "0192f000-0000-7000-8000-00000000000a";
export const orgB = "0192f000-0000-7000-8000-00000000000b";

export interface Upstream {
  signedIn?: boolean;
  permissions?: string[];
  memberships?: string[];
  stored?: { theme: string; language: string; density: string };
  explained?: { permission: string }[];
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
    service(FederationService, { listSignInProviders: () => ({ providers: [] }) });
  });
}

/** Renders the whole app at `path` against the in-memory upstream. */
export function renderApp(path: string, state: Upstream = {}) {
  configureZod();
  const session = createSession("/api", createUpstream(state));
  const studio = createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false });
  const history = createMemoryHistory({ initialEntries: [path] });
  const router = createAppRouter({ session }, history);
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
