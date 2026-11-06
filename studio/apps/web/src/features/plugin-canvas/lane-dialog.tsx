// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { NavigateEdge } from "@plux/canvas";
import { Button, Dialog, Select } from "@plux/ui";
import { Input } from "@plux/ui/components/input";
import { type ReactElement, useMemo, useState } from "react";
import "./lane-dialog.css";
import { pageSettingsMessages } from "../panels/page-settings.messages.ts";
import { transitions } from "../panels/page-settings-model.ts";
import { TargetPicker } from "../safety/index.ts";
import { type NavigateSite, navigateSite, type RouteOption, routeOptions } from "./links.ts";
import { navigationMessages } from "./navigation.messages.ts";
import { type DeclaredParam, missingParams, paramText, paramValue } from "./navigation.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";

/** What the lane's link opens (PC-28): the page's declared parameters, what the link passes, its transition. */
export interface LaneTarget {
  readonly declared: readonly DeclaredParam[];
  readonly passed: Readonly<Record<string, unknown>>;
  readonly transition: string;
  readonly onParam: (name: string, value: unknown) => void;
  readonly onTransition: (transition: string) => void;
}

/** The parameters a link passes, each typed field committing on blur; the required ones it misses are named. */
function LaneParams({ target, canEdit }: { readonly target: LaneTarget; readonly canEdit: boolean }) {
  const { i18n } = useLingui();
  const m = navigationMessages(i18n);
  const ps = pageSettingsMessages(i18n);
  const missing = missingParams(target.declared, target.passed);
  return (
    <div className="lane-target">
      <h3 className="text-label font-medium">{m.params}</h3>
      {target.declared.length === 0 && <p className="text-body text-muted-foreground">{m.noParams}</p>}
      {target.declared.map((d) => (
        <div key={d.name} className="lane-param">
          <span className="lane-param-name">
            {d.name}
            <span className="lane-param-type">{d.type}</span>
            {d.required && <span className="lane-param-required">{m.required}</span>}
          </span>
          <Input
            key={paramText(target.passed[d.name])}
            aria-label={d.name}
            placeholder={m.paramHint}
            defaultValue={paramText(target.passed[d.name])}
            readOnly={!canEdit}
            onBlur={(e) => {
              if (e.target.value !== paramText(target.passed[d.name])) {
                target.onParam(d.name, paramValue(d.type, e.target.value));
              }
            }}
          />
        </div>
      ))}
      {missing.length > 0 && (
        <p className="lane-missing" role="alert">
          {m.missing(missing.join(", "))}
        </p>
      )}
      <div className="lane-param">
        <span className="lane-param-name">{m.opensWith}</span>
        <Select
          label={m.opensWith}
          value={target.transition}
          disabled={!canEdit}
          options={transitions
            .filter((t) => t !== "custom" || target.transition === "custom")
            .map((t) => ({ value: t, label: ps.transitionName(t) }))}
          onValueChange={target.onTransition}
        />
      </div>
    </div>
  );
}

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
  target,
}: {
  readonly title: string;
  readonly site: NavigateSite | undefined;
  readonly routes: readonly RouteOption[];
  readonly canEdit: boolean;
  readonly onRedirect: (route: string) => void;
  readonly onRemove: () => void;
  readonly onClose: () => void;
  /** The opened page's parameters and transition, when the link opens a page of this plugin (PC-28). */
  readonly target?: LaneTarget | undefined;
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
        {site === undefined && <p className="text-body text-muted-foreground">{m.laneUnavailable}</p>}
        {site !== undefined && !canEdit && <p className="text-body text-muted-foreground">{m.laneReadOnly}</p>}
        {site !== undefined && target !== undefined && <LaneParams target={target} canEdit={canEdit} />}
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
