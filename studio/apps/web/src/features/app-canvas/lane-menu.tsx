// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type LaneAction, laneMenu } from "@plux/canvas";
import { Button, Dialog } from "@plux/ui";
import { type ReactNode, useState } from "react";
import { type PageInfo, TargetPicker } from "../safety/index.ts";
import type { appCanvasMessages } from "./app-canvas.messages.ts";
import type { AppLink } from "./app-canvas.scene.ts";

type Messages = ReturnType<typeof appCanvasMessages>;

export interface LaneMenuProps {
  readonly link: AppLink | undefined;
  readonly pages: ReadonlyMap<string, PageInfo>;
  readonly messages: Messages;
  readonly onOpenScreen: (plugin: string, page: string) => void;
  readonly onRedirect: (link: AppLink, route: string) => void;
  readonly onRemove: (link: AppLink) => void;
  readonly onClose: () => void;
}

/**
 * The lane menu of one link picked on the app canvas (STU-046), in the order `laneMenu` gives: open
 * either end, change the trigger in its plugin, redirect or remove the one step. Redirect and remove
 * run the S3.5 change sets, so each is one atomic change across plugins.
 */
export function LaneMenu(props: LaneMenuProps): ReactNode {
  const { link, messages: m } = props;
  const [choosing, setChoosing] = useState(false);
  if (link === undefined) {
    return null;
  }
  const labels: Record<LaneAction, string> = {
    "open-source": m.laneOpenSource,
    "open-target": m.laneOpenTarget,
    "change-trigger": m.laneChangeTrigger,
    redirect: m.laneRedirect,
    remove: m.laneRemove,
  };
  const run = (action: LaneAction) => {
    if (action === "open-source" || action === "change-trigger") {
      props.onOpenScreen(link.sourcePlugin, link.sourcePage);
    } else if (action === "open-target" && link.targetPlugin !== undefined && link.targetPage !== undefined) {
      props.onOpenScreen(link.targetPlugin, link.targetPage);
    } else if (action === "redirect") {
      setChoosing(true);
    } else if (action === "remove") {
      props.onRemove(link);
    }
  };
  return (
    <>
      <Dialog
        open={!choosing}
        onOpenChange={(o) => !o && props.onClose()}
        title={m.laneMenu(link.edge.title)}
        size="sm"
        labels={{ close: m.close }}
      >
        <p className="text-body text-secondary">{link.edge.trigger}</p>
        <div className="mt-4 flex flex-col items-stretch gap-2">
          {laneMenu(link.edge).map((item) => (
            <Button
              key={item.action}
              variant={item.action === "remove" ? "danger" : "secondary"}
              disabled={item.action === "open-target" && link.targetPage === undefined}
              onClick={() => run(item.action)}
            >
              {labels[item.action]}
            </Button>
          ))}
        </div>
      </Dialog>
      <TargetPicker
        open={choosing}
        title={m.redirectTitle}
        label={m.redirectTarget}
        options={[...props.pages.values()]
          .filter((p) => p.route !== link.route)
          .map((p) => ({ value: p.route, label: p.title }))}
        action={m.redirect}
        onConfirm={(route) => props.onRedirect(link, route)}
        onOpenChange={(o) => !o && props.onClose()}
      />
    </>
  );
}
