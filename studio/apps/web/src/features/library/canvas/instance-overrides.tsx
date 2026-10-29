// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { ComponentService } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useRef } from "react";
import { useApp } from "../../../lib/use-app.tsx";
import { setPropRecipe } from "../../panels/properties-model.ts";
import type { EditFn } from "../../plugin-canvas/select/edits.ts";
import { interfaceOf } from "../library-model.ts";
import { canvasLibraryMessages } from "./canvas.messages.ts";

interface Instance {
  readonly id?: unknown;
  readonly component?: { readonly id: string; readonly version: number };
  readonly props?: Record<string, unknown>;
  readonly slots?: Record<string, unknown>;
}

/** The recipe that removes the override of one slot of an instance. */
export function resetSlotRecipe(id: string, slot: string): (root: unknown) => void {
  return (root) => {
    const stack = [root as Instance & { children?: unknown }];
    for (let n = stack.pop(); n !== undefined; n = stack.pop()) {
      if (n.id === id) {
        delete n.slots?.[slot];
        return;
      }
      const kids = Array.isArray(n.children) ? (n.children as (Instance & { children?: unknown })[]) : [];
      stack.push(...kids);
    }
  };
}

const coerce = (type: string, text: string): unknown => {
  if (text === "") {
    return undefined;
  }
  if (type === "double" || type === "int") {
    return Number.isNaN(Number(text)) ? text : Number(text);
  }
  return type === "bool" ? text === "true" : text;
};

/**
 * Properties section of a selected component instance (STU-084): edits the overrides of the props the component
 * exposes, and resets overridden slots, through the editing core. Each edit is one undo step per burst.
 */
export function InstanceOverrides({
  node,
  doc,
  edit,
  canEdit,
}: {
  readonly node: object | undefined;
  readonly doc: string | undefined;
  readonly edit: EditFn;
  readonly canEdit: boolean;
}): ReactElement | null {
  const m = canvasLibraryMessages(useLingui().i18n);
  const { session } = useApp();
  const burst = useRef(0);
  const instance = node as Instance | undefined;
  const ref = instance?.component;
  const exposed = useQuery({
    queryKey: ["library", "instance-interface", ref?.id, ref?.version],
    enabled: ref !== undefined,
    queryFn: async () => {
      const client = createClient(ComponentService, session.transport);
      const found = (await client.listLibraryComponents({ page: { pageSize: 200 } })).items.find(
        (i) => i.documentId === ref?.id,
      );
      const versions = found === undefined ? undefined : await client.listComponentVersions({ id: found.id });
      return interfaceOf(versions?.versions.find((v) => v.version === ref?.version)?.content);
    },
  });
  if (ref === undefined || doc === undefined || typeof instance?.id !== "string") {
    return null;
  }
  const id = instance.id;
  const run = (recipe: (root: unknown) => void, name: string) =>
    edit(doc, recipe, { group: `override:${id}:${name}:${burst.current}` });
  const overriddenSlots = (exposed.data?.slots ?? []).filter((s) => instance.slots?.[s] !== undefined);
  return (
    <section className="flex flex-col gap-2" aria-label={m.overridesHeading}>
      <h3 className="text-body font-medium">{m.overridesHeading}</h3>
      {exposed.data?.props.size === 0 && exposed.data.slots.length === 0 && (
        <p className="text-caption text-secondary">{m.overridesNone}</p>
      )}
      {[...(exposed.data?.props ?? [])].map(([name, type]) => (
        <Field key={name} label={name}>
          <Input
            value={String(instance.props?.[name] ?? "")}
            disabled={!canEdit}
            onChange={(e) => run(setPropRecipe(id, "compact", name, coerce(type, e.target.value)), name)}
            onBlur={() => {
              burst.current += 1;
            }}
          />
        </Field>
      ))}
      {overriddenSlots.map((slot) => (
        <div key={slot} className="flex items-center gap-2 text-caption text-secondary">
          <span>{m.overrideSlot(slot)}</span>
          <Button variant="ghost" size="sm" disabled={!canEdit} onClick={() => run(resetSlotRecipe(id, slot), slot)}>
            {m.overrideReset(slot)}
          </Button>
        </div>
      ))}
    </section>
  );
}
