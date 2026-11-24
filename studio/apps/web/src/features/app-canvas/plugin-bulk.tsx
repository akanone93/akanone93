// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type Plugin, PluginService } from "@plux/api";
import { AlertDialog, Icon } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { type ReactNode, useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { TypeToConfirmDialog, useProductionRelease } from "../safety/index.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import { pluginKeyFrom } from "./app-canvas.model.ts";

/**
 * The plugins of a selection, acted on together (#264): archive them, restore them, or duplicate each. Archiving asks
 * first — for an app with a production release, by typing the app's key, as one plugin asks for its own (STU-074) —
 * and the calls go one after another, so one refused stops the rest and says so. Deleting stays one plugin at a time:
 * each delete has its own where-used check and fixes (STU-070, STU-071).
 */

/** The copies of `plugins`: a numbered name each, and a key free among `taken` and the copies before it. */
export function copiesOf(
  plugins: readonly Pick<Plugin, "id" | "name">[],
  taken: ReadonlySet<string>,
): { readonly id: string; readonly name: string; readonly key: string }[] {
  const used = new Set(taken);
  return plugins.map((p) => {
    const name = `${p.name} 2`;
    const key = pluginKeyFrom(name, used);
    used.add(key);
    return { id: p.id, name, key };
  });
}

/** Runs the calls in order; resolves false at the first that fails, leaving the rest undone. */
export async function inOrder(calls: readonly (() => Promise<unknown>)[]): Promise<boolean> {
  for (const call of calls) {
    try {
      await call();
    } catch {
      return false;
    }
  }
  return true;
}

export function PluginBulk(props: {
  readonly appId: string;
  readonly appKey: string;
  /** The selected plugins, two or more. */
  readonly plugins: readonly Plugin[];
  /** Every plugin key of the app, for the copies' keys. */
  readonly keys: ReadonlySet<string>;
  /** Hidden while the pointer shows one plugin's details. */
  readonly hidden?: boolean;
  readonly onDone: () => void;
  readonly onFailed: (message: string) => void;
}): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  const { session } = useApp();
  const client = useMemo(() => createClient(PluginService, session.transport), [session.transport]);
  const production = useProductionRelease(props.appId);
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const active = props.plugins.filter((p) => !p.archived);
  const archived = props.plugins.filter((p) => p.archived);

  const run = (calls: readonly (() => Promise<unknown>)[]) => {
    setAsking(false);
    setBusy(true);
    void inOrder(calls).then((ok) => {
      setBusy(false);
      if (ok) {
        props.onDone();
      } else {
        props.onFailed(m.bulkFailed);
        props.onDone();
      }
    });
  };
  const archive = () => run(active.map((p) => () => client.archivePlugin({ id: p.id, archived: true })));
  const restore = () => run(archived.map((p) => () => client.archivePlugin({ id: p.id, archived: false })));
  const duplicate = () =>
    run(
      copiesOf(props.plugins, props.keys).map(
        (c) => () => client.duplicatePlugin({ id: c.id, name: c.name, key: c.key, session: uuidv7() }),
      ),
    );
  const title = m.archiveSeveral(active.length);
  const description = m.archiveSeveralConfirm(active.length);
  if (props.plugins.length < 2 || props.hidden === true) {
    return null;
  }

  return (
    <aside
      aria-label={m.bulkSelected(props.plugins.length)}
      className="canvas-panel canvas-side canvas-inspector glass flex w-80 flex-col gap-3 p-4"
    >
      <h2 className="font-semibold">{m.bulkSelected(props.plugins.length)}</h2>
      <ul className="flex flex-col gap-1 text-sm text-muted-foreground">
        {props.plugins.map((p) => (
          <li key={p.id} className="truncate">
            {p.name}
          </li>
        ))}
      </ul>
      {/* biome-ignore lint/a11y/useSemanticElements: a labelled group of buttons, not a form's fieldset */}
      <div role="group" aria-label={m.pluginActions} className="flex flex-col gap-2 border-t pt-3">
        {active.length > 0 && (
          <Button variant="outline" size="sm" className="justify-start" disabled={busy} onClick={() => setAsking(true)}>
            <Icon name="folder" data-icon="inline-start" />
            {m.archiveSeveral(active.length)}
          </Button>
        )}
        {archived.length > 0 && (
          <Button variant="outline" size="sm" className="justify-start" disabled={busy} onClick={restore}>
            <Icon name="arrow-counter-clockwise" data-icon="inline-start" />
            {m.restoreSeveral(archived.length)}
          </Button>
        )}
        <Button variant="outline" size="sm" className="justify-start" disabled={busy} onClick={duplicate}>
          <Icon name="copy" data-icon="inline-start" />
          {m.duplicateSeveral(props.plugins.length)}
        </Button>
      </div>
      {asking && !production && (
        <AlertDialog
          open
          onOpenChange={(o) => !o && setAsking(false)}
          title={title}
          description={description}
          labels={{ cancel: m.cancel, confirm: title }}
          onConfirm={archive}
        />
      )}
      {asking && production && (
        <TypeToConfirmDialog
          open
          title={title}
          description={`${description} ${m.productionNoteSeveral}`}
          name={props.appKey}
          action={title}
          onConfirm={archive}
          onOpenChange={(o) => !o && setAsking(false)}
        />
      )}
    </aside>
  );
}
