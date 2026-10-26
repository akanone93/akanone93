// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { NavigateEdge } from "@plux/canvas";
import { Button, Dialog } from "@plux/ui";
import { type ReactElement, useMemo, useState } from "react";
import { TargetPicker } from "../safety/index.ts";
import { type NavigateSite, navigateSite, type RouteOption, routeOptions } from "./links.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";

/**
 * The fixes of one lane picked on the arrow layer (STU-046): redirect the link to another screen or
 * remove it. Both run the S3.4 recipes through the edit pipeline, so each is one undo step.
 */
export function LaneDialog({
  title,
  site,
  routes,
  canEdit,
  onRedirect,
  onRemove,
  onClose,
}: {
  readonly title: string;
  readonly site: NavigateSite | undefined;
  readonly routes: readonly RouteOption[];
  readonly canEdit: boolean;
  readonly onRedirect: (route: string) => void;
  readonly onRemove: () => void;
  readonly onClose: () => void;
}): ReactElement {
  const m = pluginCanvasMessages(useLingui().i18n);
  const [choosing, setChoosing] = useState(false);
  const fixable = site !== undefined && canEdit;
  return (
    <>
      <Dialog
        open={!choosing}
        onOpenChange={(o) => !o && onClose()}
        title={m.laneTitle(title)}
        labels={{ close: m.close }}
      >
        {site === undefined && <p className="text-body text-secondary">{m.laneUnavailable}</p>}
        {site !== undefined && !canEdit && <p className="text-body text-secondary">{m.laneReadOnly}</p>}
        <div className="mt-4 flex flex-wrap justify-end gap-2">
          <Button variant="secondary" disabled={!fixable} onClick={() => setChoosing(true)}>
            {m.laneRedirect}
          </Button>
          <Button variant="danger" disabled={!fixable} onClick={onRemove}>
            {m.laneRemove}
          </Button>
        </div>
      </Dialog>
      <TargetPicker
        open={choosing}
        title={m.redirectTitle}
        label={m.redirectTarget}
        options={routes.filter((r) => r.route !== site?.link.route).map((r) => ({ value: r.route, label: r.title }))}
        action={m.redirect}
        onConfirm={onRedirect}
        onOpenChange={(o) => !o && onClose()}
      />
    </>
  );
}

/** The picked lane, the navigate step behind it and the screens it can be redirected to. */
export function useLane(
  screens: readonly { readonly id: string; readonly root: unknown }[],
  own: readonly { readonly route: string; readonly title: string }[],
  app: readonly { readonly route: string; readonly title: string }[],
) {
  const [lane, setLane] = useState<NavigateEdge | null>(null);
  const root = lane === null ? undefined : screens.find((s) => s.id === lane.source)?.root;
  const site = useMemo(
    () =>
      lane === null || root === undefined
        ? undefined
        : navigateSite(root, Number(lane.id.slice(lane.id.lastIndexOf("#") + 1))),
    [lane, root],
  );
  const routes = useMemo(() => routeOptions(own, app), [own, app]);
  return { lane, setLane, site, routes };
}
