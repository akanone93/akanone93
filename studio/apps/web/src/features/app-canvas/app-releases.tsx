// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService, type Release, ReleaseService } from "@plux/api";
import { Icon } from "@plux/ui";
import { buttonVariants } from "@plux/ui/components/button";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { usePermissions } from "../security/use-security.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";

/**
 * Where the app stands, on the app canvas (#261): the newest release of each environment, and the way to publish. The
 * unpublished changes are the Overview's figure; the dev session's state is the link bar at the canvas's foot.
 */

/** An environment's newest release, by sequence; undefined while it has none. */
export function newestRelease(releases: readonly Release[]): Release | undefined {
  return releases.reduce<Release | undefined>(
    (a, r) => (a === undefined || r.sequence > a.sequence ? r : a),
    undefined,
  );
}

interface EnvironmentRelease {
  readonly id: string;
  readonly name: string;
  readonly state: "loading" | "failed" | "done";
  readonly newest?: Release | undefined;
}

/** Each environment of the app with its newest release; the same queries as the Releases page, so they share a cache. */
function useEnvironmentReleases(appId: string): {
  readonly state: "loading" | "failed" | "done";
  readonly environments: readonly EnvironmentRelease[];
} {
  const { session } = useApp();
  const environments = useQuery({
    queryKey: ["environments", appId],
    enabled: appId !== "",
    queryFn: () => createClient(AppService, session.transport).listEnvironments({ appId }),
  });
  const list = environments.data?.environments ?? [];
  const releases = useQueries({
    queries: list.map((e) => ({
      queryKey: ["releases", appId, e.id],
      queryFn: () => createClient(ReleaseService, session.transport).listReleases({ appId, environmentId: e.id }),
    })),
  });
  return {
    state: environments.isError ? "failed" : environments.data === undefined ? "loading" : "done",
    environments: list.map((e, i) => {
      const q = releases[i];
      return {
        id: e.id,
        name: e.name === "" ? e.key : e.name,
        state: q?.isError ? "failed" : q?.data === undefined ? "loading" : "done",
        newest: q?.data === undefined ? undefined : newestRelease(q.data.releases),
      };
    }),
  };
}

/** The releases section of the app canvas's Overview, and Publish for those who may. */
export function AppReleases({ org, appId }: { readonly org: string; readonly appId: string }): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const { state, environments } = useEnvironmentReleases(appId);
  const canPublish = usePermissions().has("release.publish");
  const line = (e: EnvironmentRelease): string => {
    if (e.state !== "done") {
      return e.state === "failed" ? m.releasesUnavailable : m.checkingReleases;
    }
    if (e.newest === undefined) {
      return m.noReleaseYet;
    }
    const at = e.newest.createdAt === undefined ? undefined : timestampDate(e.newest.createdAt);
    return m.releaseOf(String(e.newest.sequence), at === undefined ? "" : i18n.date(at, { dateStyle: "medium" }));
  };
  return (
    <section aria-label={m.releasesHeading} className="flex flex-col gap-2">
      <h3 className="text-label font-medium">{m.releasesHeading}</h3>
      {state !== "done" ? (
        <p className="dock-note">{state === "failed" ? m.releasesUnavailable : m.checkingReleases}</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {environments.map((e) => (
            <li key={e.id} className="overview-screen">
              <Icon name="rocket-launch" size="pill" />
              <span className="min-w-0 flex-1 truncate">{e.name}</span>
              <span className="truncate text-muted-foreground">{line(e)}</span>
            </li>
          ))}
        </ul>
      )}
      {canPublish && (
        <Link
          to="/o/$org/apps/$app/publish"
          params={{ org, app: appId } as never}
          className={buttonVariants({ size: "sm", className: "self-start" })}
        >
          <Icon name="paper-plane-tilt" data-icon="inline-start" />
          {m.publishApp}
        </Link>
      )}
    </section>
  );
}
