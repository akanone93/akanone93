// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  type ErrorComponentProps,
  lazyRouteComponent,
  Outlet,
  type RouterHistory,
  redirect,
} from "@tanstack/react-router";
import { z } from "zod";
import { signInSearchSchema } from "./features/auth/auth.schema.ts";
import { ForbiddenPage, LicencePage, NotFoundPage } from "./features/shell/error-pages.tsx";
import { PublicLayout } from "./features/shell/layouts.tsx";
import { ShellLayout } from "./features/shell/shell-layout.tsx";
import { AccessError, loadUser, requireMembership, requireUser, safeNext } from "./lib/guards.ts";
import type { Session } from "./lib/session.ts";

export interface RouterContext {
  readonly session: Session;
}

function RouteError({ error }: ErrorComponentProps) {
  if (error instanceof AccessError) {
    return error.kind === "licence" ? <LicencePage /> : <ForbiddenPage permission={error.permission} />;
  }
  throw error;
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
  notFoundComponent: NotFoundPage,
  errorComponent: RouteError,
});

/** Signed-in people leave P1-P4 for `?next` or `/` (R2.7). */
async function leaveWhenSignedIn(session: Session, next: string | undefined) {
  if ((await loadUser(session)) !== undefined) {
    throw redirect({ href: safeNext(next) });
  }
}

const publicLayout = createRoute({ getParentRoute: () => rootRoute, id: "public", component: PublicLayout });

const signInRoute = createRoute({
  getParentRoute: () => publicLayout,
  path: "/signin",
  validateSearch: (search) => signInSearchSchema.catch({}).parse(search),
  beforeLoad: ({ context, search }) => leaveWhenSignedIn(context.session, search.next),
  component: lazyRouteComponent(() => import("./features/auth/index.ts"), "SignInPage"),
});

const inviteRoute = createRoute({
  getParentRoute: () => publicLayout,
  path: "/invite/$token",
  beforeLoad: ({ context }) => leaveWhenSignedIn(context.session, undefined),
  component: lazyRouteComponent(() => import("./features/auth/index.ts"), "InvitePage"),
});

const signedOutRoute = createRoute({
  getParentRoute: () => publicLayout,
  path: "/signed-out",
  component: lazyRouteComponent(() => import("./features/shell/signed-out-page.tsx"), "SignedOutPage"),
});

const shellLayout = createRoute({
  getParentRoute: () => rootRoute,
  id: "shell",
  beforeLoad: async ({ context, location }) => ({ user: await requireUser(context.session, location.href) }),
  component: ShellLayout,
});

const personalHomeRoute = createRoute({
  getParentRoute: () => shellLayout,
  path: "/",
  beforeLoad: ({ context }) => {
    const first = context.user.memberships[0];
    if (first !== undefined) {
      throw redirect({ to: "/o/$org", params: { org: first.organizationId } });
    }
  },
  component: lazyRouteComponent(() => import("./features/shell/no-organisation-page.tsx"), "NoOrganisationPage"),
});

const profileRoute = createRoute({
  getParentRoute: () => shellLayout,
  path: "/me",
  beforeLoad: ({ context }) => context.session.setOrganization(undefined),
  component: lazyRouteComponent(() => import("./features/profile/index.ts"), "ProfilePage"),
});

const shortcutsRoute = createRoute({
  getParentRoute: () => shellLayout,
  path: "/me/shortcuts",
  beforeLoad: ({ context }) => context.session.setOrganization(undefined),
  component: lazyRouteComponent(() => import("./features/profile/index.ts"), "ShortcutsPage"),
});

const orgRoute = createRoute({
  getParentRoute: () => shellLayout,
  path: "/o/$org",
  params: { parse: (raw) => ({ org: z.string().parse(raw.org) }) },
  beforeLoad: ({ context, params }) => context.session.setOrganization(requireMembership(context.user, params.org)),
  component: Outlet,
});

const orgHomeRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/",
  component: lazyRouteComponent(() => import("./features/home/index.ts"), "HomePage"),
});

const routeTree = rootRoute.addChildren([
  publicLayout.addChildren([signInRoute, inviteRoute, signedOutRoute]),
  shellLayout.addChildren([personalHomeRoute, profileRoute, shortcutsRoute, orgRoute.addChildren([orgHomeRoute])]),
]);

/** The route tree: only pages that exist (L-7). `canvas` and `page` layouts join with their first pages. */
export function createAppRouter(context: RouterContext, history?: RouterHistory) {
  return createRouter({ routeTree, context, ...(history !== undefined && { history }), defaultPreload: "intent" });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}
