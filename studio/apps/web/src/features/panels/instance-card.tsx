// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Icon } from "@plux/ui";
import { createContext, type ReactElement, useContext } from "react";
import type { InstanceMark } from "../plugin-canvas/component-instances.ts";
import { instanceCardMessages } from "./instance-card.messages.ts";

/**
 * The selected component instance (PC-37, STU-084, STU-085): its component, the version it uses and whether a newer
 * one exists, how many props and slots it overrides, isolating it on the canvas and opening its component.
 */

export interface InstancePreview {
  readonly markOf: (id: string) => InstanceMark | undefined;
  readonly isolated: string | undefined;
  readonly isolate: (id: string | undefined) => void;
  readonly open: (component: string) => void;
}
export const InstancePreviewContext = createContext<InstancePreview | null>(null);

export function InstanceCard({ id }: { readonly id: string }): ReactElement | null {
  const m = instanceCardMessages(useLingui().i18n);
  const preview = useContext(InstancePreviewContext);
  const mark = preview?.markOf(id);
  if (preview === null || mark === undefined) {
    return null;
  }
  const isolated = preview.isolated === id;
  return (
    <section className="lay-card" aria-label={m.title(mark.name)}>
      <h3 className="lay-title">
        <Icon name="component" size="pill" />
        {m.title(mark.name)}
      </h3>
      {!mark.known && (
        <p className="cond-worked" data-result="false">
          {m.missing}
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        {m.version(mark.version)} · {m.overrides(mark.overrides)}
      </p>
      {mark.latest > mark.version && (
        <p className="cond-worked" data-result="false">
          <Icon name="arrow-up-right" size="pill" />
          {m.newer(mark.latest)}
        </p>
      )}
      <div className="flex flex-wrap gap-1.5">
        <Button
          size="sm"
          variant={isolated ? "primary" : "secondary"}
          onClick={() => preview.isolate(isolated ? undefined : id)}
        >
          <Icon name="crosshair" size="pill" />
          {isolated ? m.leave : m.isolate}
        </Button>
        {mark.known && (
          <Button size="sm" variant="secondary" onClick={() => preview.open(mark.component)}>
            <Icon name="arrow-up-right" size="pill" />
            {m.open}
          </Button>
        )}
      </div>
    </section>
  );
}
