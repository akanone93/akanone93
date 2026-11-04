// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useParams } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { AreaTabs } from "../../components/page.tsx";
import { experimentsMessages } from "../experiments/experiments.messages.ts";
import { releasesMessages } from "./releases.messages.ts";

function useBase(area: string): string {
  const { org, app } = useParams({ strict: false }) as { org: string; app: string };
  return `/o/${org}/apps/${app}/${area}`;
}

/** The pages of the releases area as the header's tab strip: the timeline and the kill switch. */
export function ReleasesTabs(): ReactElement {
  const m = releasesMessages(useLingui().i18n);
  const base = useBase("releases");
  return (
    <AreaTabs
      label={m.areaNav}
      tabs={[
        { href: base, label: m.timelineTitle, icon: "rocket-launch", exact: true },
        { href: `${base}/kill-switch`, label: m.killSwitchLink, icon: "warning-circle" },
      ]}
    />
  );
}

/** The pages of the experiments area as the header's tab strip: the experiments and the feature flags. */
export function ExperimentsTabs(): ReactElement {
  const m = experimentsMessages(useLingui().i18n);
  const base = useBase("experiments");
  return (
    <AreaTabs
      label={m.areaNav}
      tabs={[
        { href: base, label: m.listTitle, icon: "funnel", exact: true },
        { href: `${base}/flags`, label: m.flagsLink, icon: "toggle-right" },
      ]}
    />
  );
}
