// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactNode } from "react";
import { pluginCanvasMessages } from "../plugin-canvas/plugin-canvas.messages.ts";
import { buildWhereUsed, type ChangeSet, type LoadedDoc, moveScreen, TargetPicker } from "../safety/index.ts";
import type { RouteScreen } from "./app-routes.tsx";

/**
 * Moves a screen to another plugin from the app canvas (#287), as a plugin canvas's "Move to plugin" does (D23,
 * STU-055): one change set across both plugins, the screen keeping its ID, key and route, so every link into and out
 * of it keeps working. What the move refuses — the plugin's entry screen, a key the target has, a component private
 * to the source — is said, and nothing changes.
 */
export function MoveScreenDialog(props: {
  readonly screen: RouteScreen | undefined;
  readonly docs: readonly LoadedDoc[];
  readonly plugins: readonly { readonly id: string; readonly name: string }[];
  readonly apply: (changeSet: ChangeSet) => void;
  readonly onRefused: (message: string) => void;
  readonly onClose: () => void;
}): ReactNode {
  const pm = pluginCanvasMessages(useLingui().i18n);
  const { screen } = props;
  const options = props.plugins.filter((p) => p.id !== screen?.pluginId).map((p) => ({ value: p.id, label: p.name }));
  return (
    <TargetPicker
      open={screen !== undefined}
      title={pm.moveScreen}
      label={pm.moveTarget}
      options={options}
      action={pm.move}
      onConfirm={(target) => {
        if (screen === undefined) {
          return;
        }
        const moved = moveScreen(props.docs, buildWhereUsed(props.docs), screen.pageId, target);
        if (moved.ok) {
          props.apply(moved.changeSet);
        } else {
          props.onRefused(pm.moveRefused(pm.moveReason(moved.reason)));
        }
        props.onClose();
      }}
      onOpenChange={(o) => !o && props.onClose()}
    />
  );
}
