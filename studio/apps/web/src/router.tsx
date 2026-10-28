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
import { accessSearchSchema, auditSearchSchema } from "./features/admin/admin.schema.ts";
import { analyticsSearchSchema, usageSearchSchema } from "./features/analytics/analytics.schema.ts";
import { approvalsInboxSearchSchema, appSearchSchema } from "./features/approvals/approvals.schema.ts";
import { appsSearchSchema } from "./features/apps/apps.schema.ts";
import { signInSearchSchema } from "./features/auth/auth.schema.ts";
import { dataSearchSchema } from "./features/data/data.schema.ts";
import type { EditSession } from "./features/editor/edit-session.ts";
import { experimentsSearchSchema, newExperimentSearchSchema } from "./features/experiments/experiments.schema.ts";
import { functionSearchSchema, functionsEnvSearchSchema } from "./features/functions/functions.schema.ts";
import { librarySearchSchema } from "./features/library/library.schema.ts";
import { localisationSearchSchema } from "./features/localisation/area-actions.ts";
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
import { testRunSearchSchema, testsSearchSchema } from "./features/tests/tests.schema.ts";
import {
  AccessError,
  loadUser,
  requireInstallationAdmin,
  requireMembership,
  requirePermission,
  requireUser,
  safeNext,
} from "./lib/guards.ts";
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
  validateSearch: (search) => appsSearchSchema.parse(search),
  component: lazyRouteComponent(() => import("./features/home/index.ts"), "HomePage"),
});

/** O3: the new-app wizard as a route of its own (STU-024); creating an app needs `app.manage`. */
const newAppRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/apps/new",
  beforeLoad: ({ context }) => requirePermission(context.user, "app.manage"),
  component: lazyRouteComponent(() => import("./features/new-app/index.ts"), "NewAppPage"),
});

/** A38: the native shell settings of one app and its "shell update required" comparison (GEN-004); reading needs `app.read`, saving `app.manage` (F14). */
const shellSettingsRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/apps/$app/settings/shell",
  beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
  component: lazyRouteComponent(() => import("./features/shell-settings/index.ts"), "ShellSettingsPage"),
});

/** A37 and A42: the settings of one app (STU-023, STU-025, LIM-005): name, description, limits usage, host fonts; changing them needs `app.manage`. */
const appSettingsRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/apps/$app/settings",
  beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
  component: lazyRouteComponent(() => import("./features/apps/index.ts"), "AppSettingsPage"),
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

/** A10-A12: the Functions area of one app (STU-110); the pages need `app.read`, alias changes are gated by function.deploy. */
function functionsRoute(
  path: string,
  component: "FunctionsPage" | "FunctionPage" | "FunctionEditPage",
  search: typeof functionsEnvSearchSchema | typeof functionSearchSchema,
) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/functions${path}`,
    validateSearch: (raw) => search.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/functions/index.ts"), component),
  });
}

const functionsListRoute = functionsRoute("", "FunctionsPage", functionsEnvSearchSchema);
const functionRoute = functionsRoute("/$fnId", "FunctionPage", functionSearchSchema);
const functionEditRoute = functionsRoute("/$fnId/edit", "FunctionEditPage", functionsEnvSearchSchema);
/** A20-A23: the Data area of one app (STU-113); the pages need `app.read`, and editing needs the plugin's editing lock. */
function dataRoute(path: string, component: "DataPage" | "SourcePage" | "CollectionsPage" | "CollectionPage") {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/data${path}`,
    validateSearch: (raw) => dataSearchSchema.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/data/index.ts"), component),
  });
}

const dataSourcesRoute = dataRoute("", "DataPage");
const dataSourceRoute = dataRoute("/sources/$sourceId", "SourcePage");
const dataCollectionsRoute = dataRoute("/collections", "CollectionsPage");
const dataCollectionRoute = dataRoute("/collections/$collectionId", "CollectionPage");

/** A15-A19: the Design system area of one app (STU-112); the pages need `app.read`, and editing needs the app's editing lock. */
function designRoute(
  path: string,
  component: "TokensPage" | "ThemesPage" | "TypographyPage" | "IconsPage" | "LibraryPage",
) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/design${path}`,
    validateSearch: () => ({}),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/design/index.ts"), component),
  });
}

const designTokensRoute = designRoute("", "TokensPage");
const designThemesRoute = designRoute("/themes", "ThemesPage");
const designTypographyRoute = designRoute("/typography", "TypographyPage");
const designIconsRoute = designRoute("/icons", "IconsPage");
const designLibraryRoute = designRoute("/library", "LibraryPage");

/** A13-A14: the localisation grid and the heatmap of one app (STU-111); the pages need `app.read`, editing needs `translations.edit` and the locale's lock. */
function localisationRoute(path: string, component: "LocalisationPage" | "HeatmapPage") {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/localisation${path}`,
    validateSearch: (raw) => localisationSearchSchema.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/localisation/index.ts"), component),
  });
}

const localisationGridRoute = localisationRoute("", "LocalisationPage");
const localisationHeatmapRoute = localisationRoute("/heatmap", "HeatmapPage");

/** A6-A9: experiments, the wizard, one experiment and the flags (STU-103); actions are gated by experiments.manage and flags.manage. */
function experimentsRoute(
  path: string,
  component: "ExperimentsPage" | "NewExperimentPage" | "ExperimentPage" | "FlagsPage",
  search: typeof experimentsSearchSchema | typeof newExperimentSearchSchema | typeof releaseEnvSearchSchema,
) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/experiments${path}`,
    validateSearch: (raw) => search.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/experiments/index.ts"), component),
  });
}

const experimentsListRoute = experimentsRoute("", "ExperimentsPage", experimentsSearchSchema);
const newExperimentRoute = experimentsRoute("/new", "NewExperimentPage", newExperimentSearchSchema);
const flagsRoute = experimentsRoute("/flags", "FlagsPage", releaseEnvSearchSchema);
const experimentRoute = experimentsRoute("/$expId", "ExperimentPage", releaseEnvSearchSchema);
/** O7-O12: the Approvals area (STU-102). Reading needs `app.read`; the break-glass list needs `audit.read` (F14). */
function approvalsRoute(
  path: string,
  component: string,
  permission: string,
  search: typeof appSearchSchema | typeof approvalsInboxSearchSchema = appSearchSchema,
) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/approvals${path}`,
    validateSearch: (raw) => search.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, permission),
    component: lazyRouteComponent(() => import("./features/approvals/index.ts"), component as "InboxPage"),
  });
}

const approvalsInboxRoute = approvalsRoute("", "InboxPage", "app.read", approvalsInboxSearchSchema);
const approvalsHistoryRoute = approvalsRoute("/history", "HistoryPage", "app.read");
const approvalsBreakGlassRoute = approvalsRoute("/break-glass", "BreakGlassPage", "audit.read");
const approvalsPoliciesRoute = approvalsRoute("/policies", "PoliciesPage", "app.read");
const approvalsPolicyRoute = approvalsRoute("/policies/$policyId", "PolicyPage", "app.read");
const approvalsRequestRoute = approvalsRoute("/$requestId", "RequestPage", "app.read");
/** O15-O31: the organisation's administration area (STU-116); each page needs the permission its first RPC checks (F14). */
function adminRoute<const P extends string>(
  path: P,
  component: keyof typeof import("./features/admin/index.ts"),
  permission: string,
  search?: typeof accessSearchSchema | typeof auditSearchSchema,
) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/admin${path}` as const,
    ...(search !== undefined && { validateSearch: (raw: Record<string, unknown>) => search.parse(raw) }),
    beforeLoad: ({ context }) => requirePermission(context.user, permission),
    component: lazyRouteComponent(() => import("./features/admin/index.ts"), component as "SettingsPage"),
  });
}

const adminRoutes = [
  adminRoute("", "SettingsPage", "organization.manage"),
  adminRoute("/members", "MembersPage", "members.manage"),
  adminRoute("/members/$userId", "MemberPage", "members.manage"),
  adminRoute("/teams", "TeamsPage", "teams.manage"),
  adminRoute("/teams/$teamId", "TeamPage", "teams.manage"),
  adminRoute("/roles", "RolesPage", "roles.manage"),
  adminRoute("/roles/$roleId", "RolePage", "roles.manage"),
  adminRoute("/restrictions", "RestrictionsPage", "restrictions.manage"),
  adminRoute("/access", "AccessPage", "roles.manage", accessSearchSchema),
  adminRoute("/sso", "SsoPage", "sso.manage"),
  adminRoute("/tokens", "TokensPage", "tokens.manage"),
  adminRoute("/sessions", "SessionsPage", "sessions.manage"),
  adminRoute("/audit", "AuditPage", "audit.read", auditSearchSchema),
  adminRoute("/compliance", "CompliancePage", "compliance.report"),
  adminRoute("/retention", "RetentionPage", "retention.manage"),
  adminRoute("/license", "LicensePage", "organization.manage"),
  adminRoute("/limits", "LimitsPage", "limits.manage"),
];

/** I1-I7: the installation's pages for installation administrators only (STU-116); they live outside any organisation. */
function installationRoute(path: string, component: keyof typeof import("./features/installation/index.ts")) {
  return createRoute({
    getParentRoute: () => shellLayout,
    path: `/installation${path}`,
    beforeLoad: ({ context }) => {
      requireInstallationAdmin(context.user);
      context.session.setOrganization(undefined);
    },
    component: lazyRouteComponent(() => import("./features/installation/index.ts"), component),
  });
}

const installationRoutes = [
  installationRoute("", "HealthPage"),
  installationRoute("/organisations", "OrganisationsPage"),
  installationRoute("/storage", "StoragePage"),
  installationRoute("/jobs", "JobsPage"),
  installationRoute("/backups", "BackupsPage"),
  installationRoute("/issuers", "IssuersPage"),
  installationRoute("/audit", "InstallationAuditPage"),
];

/** A45-A46: Plux Test scenarios and the runs reported for the app (TST-001, TST-002). */
function testsRoute(
  path: string,
  component: "TestsPage" | "TestRunPage",
  search: typeof testsSearchSchema | typeof testRunSearchSchema,
) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/tests${path}`,
    validateSearch: (raw) => search.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/tests/index.ts"), component),
  });
}

const testsListRoute = testsRoute("", "TestsPage", testsSearchSchema);
const testRunRoute = testsRoute("/$runId", "TestRunPage", testRunSearchSchema);

/** A24-A30 and A47: analytics of the app (STU-114, ANL-020, ANL-040); every page needs analytics.read, enforced by the server. */
type AnalyticsPage =
  | "AnalyticsOverviewPage"
  | "AnalyticsUsagePage"
  | "AnalyticsFunnelsPage"
  | "AnalyticsFunnelPage"
  | "AnalyticsErrorsPage"
  | "AnalyticsErrorPage"
  | "AnalyticsPerformancePage"
  | "AnalyticsReplaysPage"
  | "AnalyticsReplayPage";

function analyticsRoute(path: string, component: AnalyticsPage, search: typeof usageSearchSchema) {
  return createRoute({
    getParentRoute: () => orgRoute,
    path: `/apps/$app/analytics${path}`,
    validateSearch: (raw) => search.parse(raw),
    beforeLoad: ({ context }) => requirePermission(context.user, "app.read"),
    component: lazyRouteComponent(() => import("./features/analytics/index.ts"), component),
  });
}

const analyticsRoutes = [
  analyticsRoute("", "AnalyticsOverviewPage", analyticsSearchSchema as unknown as typeof usageSearchSchema),
  analyticsRoute("/usage", "AnalyticsUsagePage", usageSearchSchema),
  analyticsRoute("/funnels", "AnalyticsFunnelsPage", analyticsSearchSchema as unknown as typeof usageSearchSchema),
  analyticsRoute(
    "/funnels/$funnelId",
    "AnalyticsFunnelPage",
    analyticsSearchSchema as unknown as typeof usageSearchSchema,
  ),
  analyticsRoute("/errors", "AnalyticsErrorsPage", analyticsSearchSchema as unknown as typeof usageSearchSchema),
  analyticsRoute(
    "/errors/$fingerprint",
    "AnalyticsErrorPage",
    analyticsSearchSchema as unknown as typeof usageSearchSchema,
  ),
  analyticsRoute(
    "/performance",
    "AnalyticsPerformancePage",
    analyticsSearchSchema as unknown as typeof usageSearchSchema,
  ),
  analyticsRoute("/replays", "AnalyticsReplaysPage", analyticsSearchSchema as unknown as typeof usageSearchSchema),
  analyticsRoute(
    "/replays/$replayId",
    "AnalyticsReplayPage",
    analyticsSearchSchema as unknown as typeof usageSearchSchema,
  ),
];

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
    ...installationRoutes,
    orgRoute.addChildren([
      orgHomeRoute,
      newAppRoute,
      shellSettingsRoute,
      appSettingsRoute,
      notificationsRoute,
      approvalsInboxRoute,
      approvalsHistoryRoute,
      approvalsBreakGlassRoute,
      approvalsPoliciesRoute,
      approvalsPolicyRoute,
      approvalsRequestRoute,
      devicesRoute,
      deviceRoute,
      releasesTimelineRoute,
      functionsListRoute,
      functionRoute,
      functionEditRoute,
      killSwitchRoute,
      releaseRoute,
      dataSourcesRoute,
      dataSourceRoute,
      dataCollectionsRoute,
      dataCollectionRoute,
      designTokensRoute,
      designThemesRoute,
      designTypographyRoute,
      designIconsRoute,
      designLibraryRoute,
      localisationGridRoute,
      localisationHeatmapRoute,
      experimentsListRoute,
      testsListRoute,
      ...analyticsRoutes,
      testRunRoute,
      newExperimentRoute,
      flagsRoute,
      experimentRoute,
      securityProfileRoute,
      publishStartRoute,
      publishRoute,
      keysRoute,
      pinsRoute,
      capabilitiesRoute,
      ...adminRoutes,
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
