// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { PluginSummary } from "@plux/api";
import { Button, Dialog, Field, Input, Select, Textarea } from "@plux/ui";
import { type ReactNode, useEffect, useState } from "react";
import type { appCanvasMessages } from "./app-canvas.messages.ts";
import { type PlacedBox, pluginKeyFrom } from "./app-canvas.model.ts";
import type { TeamOption } from "./use-teams.ts";

const NO_OWNER = "none";

export type Dialogue = { kind: "new" | "settings" | "duplicate" | "entry" | "frame" } | null;

export interface DialogsProps {
  readonly dialogue: Dialogue;
  readonly messages: ReturnType<typeof appCanvasMessages>;
  readonly summary: PluginSummary | undefined;
  readonly taken: ReadonlySet<string>;
  readonly selectedBoxes: readonly PlacedBox[];
  readonly teams: readonly TeamOption[];
  readonly close: () => void;
  readonly onCreate: (name: string, key: string) => void;
  readonly onSettings: (id: string, name: string, description: string, tags: string[], owningTeamId: string) => void;
  readonly onDuplicate: (id: string, name: string, key: string) => void;
  readonly onEntry: (key: string) => void;
  readonly onFrame: (label: string) => void;
}

/** The name a dialog opens with: the plugin's own for settings, a numbered copy for a duplicate. */
function initialName(kind: NonNullable<Dialogue>["kind"], current: string): string {
  if (kind === "settings") {
    return current;
  }
  return kind === "duplicate" ? `${current} 2` : "";
}

interface Fields {
  readonly name: string;
  readonly description: string;
  readonly tags: string[];
  readonly owner: string;
}

const splitTags = (text: string): string[] =>
  text
    .split(",")
    .map((t) => t.trim())
    .filter((t) => t !== "");

function dialogTitle(kind: NonNullable<Dialogue>["kind"], m: ReturnType<typeof appCanvasMessages>): string {
  const titles = {
    new: m.newPlugin,
    settings: m.settings,
    duplicate: m.duplicate,
    entry: m.setEntry,
    frame: m.newFrame,
  };
  return titles[kind];
}

/** Runs the dialog's action; an empty name does nothing, as the entry dialog has no name. */
function submitDialogue(
  kind: NonNullable<Dialogue>["kind"],
  f: Fields,
  p: PluginSummary["plugin"],
  on: DialogsProps,
): void {
  if (kind === "entry") {
    if (p !== undefined) {
      on.onEntry(p.key);
    }
  } else if (f.name === "") {
    return;
  } else if (kind === "new") {
    on.onCreate(f.name, pluginKeyFrom(f.name, on.taken));
  } else if (kind === "frame") {
    on.onFrame(f.name);
  } else if (p !== undefined && kind === "settings") {
    on.onSettings(p.id, f.name, f.description, f.tags, f.owner);
  } else if (p !== undefined) {
    on.onDuplicate(p.id, f.name, pluginKeyFrom(f.name, on.taken));
  }
}

/** D16 new plugin, D17 settings, D18 duplicate, D25 entry plugin, and the frame label (D20). */
export function PluginDialogs(props: DialogsProps): ReactNode {
  const { dialogue, messages: m, summary: s } = props;
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [tags, setTags] = useState("");
  const [owner, setOwner] = useState("");
  useEffect(() => {
    if (dialogue === null) {
      return;
    }
    const p = s?.plugin;
    setName(initialName(dialogue.kind, p?.name ?? ""));
    setDescription(p?.description ?? "");
    setTags((p?.tags ?? []).join(", "));
    setOwner(p?.owningTeamId ?? "");
  }, [dialogue, s]);
  if (dialogue === null) {
    return null;
  }
  const submit = () =>
    submitDialogue(dialogue.kind, { name: name.trim(), description, tags: splitTags(tags), owner }, s?.plugin, props);
  const title = dialogTitle(dialogue.kind, m);
  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o) {
          props.close();
        }
      }}
      title={title}
      size={dialogue.kind === "settings" ? "md" : "sm"}
      labels={{ close: m.close }}
    >
      {dialogue.kind === "entry" ? (
        <p>{s?.plugin?.name}</p>
      ) : (
        <Field label={dialogue.kind === "frame" ? m.frameLabel : m.name}>
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
      )}
      {dialogue.kind === "settings" ? (
        <>
          <Field label={m.description}>
            <Textarea value={description} onChange={(e) => setDescription(e.target.value)} />
          </Field>
          <Field label={m.tags}>
            <Input value={tags} onChange={(e) => setTags(e.target.value)} />
          </Field>
          <Field label={m.owner}>
            <Select
              label={m.owner}
              value={owner === "" ? NO_OWNER : owner}
              options={[
                { value: NO_OWNER, label: m.noOwner },
                ...props.teams.map((t) => ({ value: t.id, label: t.name })),
              ]}
              onValueChange={(v) => setOwner(v === NO_OWNER ? "" : v)}
            />
          </Field>
        </>
      ) : null}
      <div className="flex justify-end gap-2">
        <Button onClick={props.close}>{m.cancel}</Button>
        <Button onClick={submit}>{m.save}</Button>
      </div>
    </Dialog>
  );
}
