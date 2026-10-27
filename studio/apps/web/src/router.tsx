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
import type { EditSession } from "./features/editor/edit-session.ts";
import { librarySearchSchema } from "./features/library/library.schema.ts";
import { inboxSearchSchema } from "./features/notifications/notifications.schema.ts";
import { canvasSearchSchema } from "./features/plugin-canvas/plugin-canvas.schema.ts";
import type { SurfaceFactory } from "./features/plugin-canvas/surface.ts";
import type { ValidatorFactory } from "./features/problems/validator-client.ts";
import { publishSearchSchema } from "./features/publish/publish.schema.ts";
import { releaseEnvSearchSchema, releasesSearchSchema } from "./features/releases/releases.schema.ts";
import { devicesSearchSchema, envSearchSchema } from "./features/security/security.schema.ts";
import { ForbiddenPage, LicencePage, NotFoundPage } from "./features/shell/error-pages.tsx";
import { CanvasLayout, PublicLayout } from "./features/shell/layouts.tsx";
import { ShellLayout } from "./features/shell/shell-layout.tsx";
import { AccessError, loadUser, requireMembership, requirePermission, requireUser, safeNext } from "./lib/guards.ts";
import type { Session } from "./lib/session.ts";

export interface RouterContext {
  readonly session: Session;
  /** False when the browser lacks WebGL2 (P8); unset means supported. */
  readonly webgl2?: boolean;
  /** Starts the canvas surface of C2; unset means the browser worker (tests inject a stub). */
  readonly createSurface?: SurfaceFactory;
  /** Starts the validator of C2's Problems tab; unset means the browser worker (tests inject a stub). */
  readonly createValidator?: ValidatorFactory;
  /** Tests observe the plugin canvas's edit pipeline here. */
  readonly observeEditor?: (edit: EditSession) => void;
}

function RouteError({ error }: ErrorComponentProps) {
  if (error instanceof AccessError) {
    return error.kind === "licence" ? <LicencePage /> : <ForbiddenPage permission={error.permission} />;
  }
  throw error;
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  beforeLoad: ({ context, location }) => {
    if (context.webgl2 === false && location.pathname !== "/unsupported") {
      throw redirect({ to: "/unsupported" });
    }
  },
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

const unsupportedRoute = createRoute({
  getParentRoute: () => publicLayout,
  path: "/unsupported",
  component: lazyRouteComponent(() => import("./features/shell/unsupported-page.tsx"), "UnsupportedPage"),
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

const notificationsRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/notifications",
  validateSearch: (search) => inboxSearchSchema.parse(search),
  component: lazyRouteComponent(() => import("./features/notifications/index.ts"), "NotificationsPage"),
});

/** A31-A36: the Devices and security area of one app (STU-115); every page needs `app.read` (F14). */
function securityRoute(path: string, component: string, search: typeof envSearchSchema | typeof devicesSearchSchema) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/${path}`,
    validateSearch: (raw) => search.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/security/index.ts"), component as "DevicesPage"),
  });
}

const devicesRoute = securityRoute("devices", "DevicesPage", devicesSearchSchema);
const deviceRoute = securityRoute("devices/$deviceId", "DevicePage", envSearchSchema);
const securityProfileRoute = securityRoute("security", "SecurityProfilePage", envSearchSchema);
const keysRoute = securityRoute("security/keys", "KeysPage", envSearchSchema);
const pinsRoute = securityRoute("security/pins", "PinsPage", envSearchSchema);
const capabilitiesRoute = securityRoute("security/capabilities", "CapabilitiesPage", envSearchSchema);

/** A1: opens or resumes the release request; A2: the publish flow of one request (STU-100). */
const publishStartRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/apps/$app/publish",
  validateSearch: (raw) => envSearchSchema.parse(raw),
  beforeLoad: ({ context }) => requirePermission(context.user, "release.publish"),
  component: lazyRouteComponent(() => import("./features/publish/index.ts"), "PublishStartPage"),
});

const publishRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/apps/$app/publish/$requestId",
  validateSearch: (raw) => publishSearchSchema.parse(raw),
  beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
  component: lazyRouteComponent(() => import("./features/publish/index.ts"), "PublishPage"),
});
/** A3-A5: the Releases area of one app (STU-101); the pages need `app.read`, their actions are gated by rollout.manage and release.promote (F14). */
function releasesRoute(
  path: string,
  component: "ReleasesPage" | "ReleasePage" | "KillSwitchPage",
  search: typeof releasesSearchSchema | typeof releaseEnvSearchSchema,
) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/releases${path}`,
    validateSearch: (raw) => search.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/releases/index.ts"), component),
  });
}

const releasesTimelineRoute = releasesRoute("", "ReleasesPage", releasesSearchSchema);
const killSwitchRoute = releasesRoute("/kill-switch", "KillSwitchPage", releaseEnvSearchSchema);
const releaseRoute = releasesRoute("/$releaseId", "ReleasePage", releaseEnvSearchSchema);

const libraryRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/library",
  validateSearch: (search) => librarySearchSchema.parse(search),
  component: lazyRouteComponent(() => import("./features/library/index.ts"), "LibraryPage"),
});

const libraryComponentRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/library/components/$id",
  component: lazyRouteComponent(() => import("./features/library/index.ts"), "ComponentDetailPage"),
});

const libraryTemplateRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/library/templates/$id",
  component: lazyRouteComponent(() => import("./features/library/index.ts"), "TemplateDetailPage"),
});

const canvasLayout = createRoute({
  getParentRoute: () => rootRoute,
  id: "canvas",
  beforeLoad: async ({ context, location }) => ({ user: await requireUser(context.session, location.href) }),
  component: CanvasLayout,
});

/** C1: the app canvas (STU-030); the plugin boxes, native nodes and frames of one app. */
const appCanvasRoute = createRoute({
  getParentRoute: () => canvasLayout,
  path: "/o/$org/apps/$app",
  params: { parse: (raw) => ({ org: z.string().parse(raw.org), app: z.string().parse(raw.app) }) },
  beforeLoad: ({ context, params }) => context.session.setOrganization(requireMembership(context.user, params.org)),
  component: lazyRouteComponent(() => import("./features/app-canvas/index.ts"), "AppCanvasPage"),
});

/** C2: the plugin canvas (STU-050); search `x`, `y`, `z` replace history. */
const pluginCanvasRoute = createRoute({
  getParentRoute: () => canvasLayout,
  path: "/o/$org/apps/$app/plugins/$plugin",
  params: {
    parse: (raw) => ({
      org: z.string().parse(raw.org),
      app: z.string().parse(raw.app),
      plugin: z.string().parse(raw.plugin),
    }),
  },
  validateSearch: (search) => canvasSearchSchema.parse(search),
  beforeLoad: ({ context, params }) => context.session.setOrganization(requireMembership(context.user, params.org)),
  component: lazyRouteComponent(() => import("./features/plugin-canvas/index.ts"), "PluginCanvasPage"),
});

const routeTree = rootRoute.addChildren([
  canvasLayout.addChildren([appCanvasRoute, pluginCanvasRoute]),
  publicLayout.addChildren([signInRoute, inviteRoute, signedOutRoute, unsupportedRoute]),
  shellLayout.addChildren([
    personalHomeRoute,
    profileRoute,
    shortcutsRoute,
    orgRoute.addChildren([
      orgHomeRoute,
      notificationsRoute,
      devicesRoute,
      deviceRoute,
      releasesTimelineRoute,
      killSwitchRoute,
      releaseRoute,
      securityProfileRoute,
      publishStartRoute,
      publishRoute,
      keysRoute,
      pinsRoute,
      capabilitiesRoute,
      libraryRoute,
      libraryComponentRoute,
      libraryTemplateRoute,
    ]),
  ]),
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
