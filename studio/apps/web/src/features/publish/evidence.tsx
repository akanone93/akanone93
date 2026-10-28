// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { DocumentChange, PatchOp, VisualDiffScreen } from "@plux/api";
import { StatusPill } from "@plux/ui";
import type { ReactElement } from "react";
import { evidenceMessages } from "./evidence.messages.ts";
import type { PluginEvidence } from "./use-evidence.ts";
import { useDiffImage } from "./use-publish.ts";

type Messages = ReturnType<typeof evidenceMessages>;

const decoder = new TextDecoder();

function kib(bytes: bigint): string {
  return (Number(bytes) / 1024).toFixed(1);
}

/** Size and accessibility of one built plugin, from the plugin version and the publish job (STU-100, A11Y-008). */
export function PluginReport({ evidence }: { readonly evidence: PluginEvidence }): ReactElement {
  const m = evidenceMessages(useLingui().i18n);
  const { size, previousSize, accessibility } = evidence;
  return (
    <li className="flex flex-col gap-1 rounded-sm border border-hairline p-3">
      <span className="font-mono text-data">{evidence.pluginKey}</span>
      <div className="flex flex-wrap items-center gap-3 text-body">
        <span className="text-secondary">{m.size}</span>
        {size === undefined ? (
          <span className="text-secondary">{m.sizeUnknown}</span>
        ) : (
          <span className="font-mono">{m.sizeKib(kib(size))}</span>
        )}
        {size !== undefined &&
          (previousSize === undefined ? (
            <span className="text-secondary">{m.sizeFirst}</span>
          ) : (
            <span className="font-mono text-secondary">{m.sizeDelta(signed(size - previousSize))}</span>
          ))}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        {accessibility.length === 0 ? (
          <StatusPill status="success" label={m.a11yNone} />
        ) : (
          <StatusPill status="warning" label={m.a11yFindings(String(accessibility.length))} />
        )}
      </div>
      {accessibility.length > 0 && (
        <ul className="flex flex-col gap-1 text-body">
          {accessibility.map((d, i) => (
            // A plugin can report one code many times; the order is the report's.
            // biome-ignore lint/suspicious/noArrayIndexKey: findings have no identity of their own
            <li key={`${d.code}/${i}`} className="flex gap-2">
              <span className="font-mono text-secondary">{d.code}</span>
              <span>{d.message}</span>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

function signed(bytes: bigint): string {
  const value = kib(bytes < 0n ? -bytes : bytes);
  return bytes < 0n ? `-${value}` : `+${value}`;
}

function opName(m: Messages, op: string): string {
  const names: Record<string, string> = {
    add: m.opAdd,
    remove: m.opRemove,
    replace: m.opReplace,
    move: m.opMove,
    copy: m.opCopy,
    test: m.opTest,
  };
  return names[op] ?? op;
}

function changeName(m: Messages, change: string): string {
  const names: Record<string, string> = {
    added: m.changeAdded,
    removed: m.changeRemoved,
    changed: m.changeChanged,
  };
  return names[change] ?? change;
}

function Op({ op }: { readonly op: PatchOp }): ReactElement {
  const m = evidenceMessages(useLingui().i18n);
  return (
    <li className="flex flex-wrap gap-2 font-mono text-tag">
      <span className={op.op === "remove" ? "text-danger" : op.op === "add" ? "text-success" : "text-secondary"}>
        {opName(m, op.op)}
      </span>
      <span>{op.path}</span>
      {op.value.length > 0 && <span className="break-all text-secondary">{decoder.decode(op.value)}</span>}
    </li>
  );
}

function Change({ change }: { readonly change: DocumentChange }): ReactElement {
  const m = evidenceMessages(useLingui().i18n);
  return (
    <li className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-2 text-body">
        <span className="font-mono">{change.path}</span>
        <span className="text-secondary">{changeName(m, change.change)}</span>
      </div>
      {change.patch.length > 0 && (
        <ul className="flex flex-col gap-0.5 ps-4">
          {change.patch.map((op, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: patch operations are ordered and have no identity
            <Op key={`${op.op}${op.path}${i}`} op={op} />
          ))}
        </ul>
      )}
    </li>
  );
}

/** The JSON diff of the documents each plugin changed against its previous version (STU-100). */
export function JsonDiff({ evidence }: { readonly evidence: readonly PluginEvidence[] }): ReactElement {
  const m = evidenceMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-2" aria-label={m.jsonDiff}>
      <h3 className="text-label font-medium">{m.jsonDiff}</h3>
      {evidence.map((e) =>
        e.changes === undefined ? (
          <p key={e.pluginKey} className="text-body text-secondary">
            <span className="font-mono">{e.pluginKey}</span> {m.jsonDiffFirst}
          </p>
        ) : (
          <details key={e.pluginKey} open={e.changes.length > 0}>
            <summary className="text-body">{m.jsonDiffSummary(e.pluginKey, String(e.changes.length))}</summary>
            {e.changes.length === 0 ? (
              <p className="text-body text-secondary">{m.jsonDiffNone}</p>
            ) : (
              <ul className="mt-2 flex flex-col gap-2">
                {e.changes.map((c) => (
                  <Change key={c.path} change={c} />
                ))}
              </ul>
            )}
          </details>
        ),
      )}
    </section>
  );
}

function shortHash(sha: string): string {
  return sha.slice(0, 8);
}

function Tile({
  label,
  sha,
  appId,
  requestId,
}: {
  readonly label: string;
  readonly sha: string;
  readonly appId: string;
  readonly requestId: string;
}): ReactElement {
  const m = evidenceMessages(useLingui().i18n);
  const image = useDiffImage(appId, requestId, sha);
  return (
    <figure className="flex min-w-32 flex-col gap-1 rounded-sm border border-hairline bg-raised p-2">
      <figcaption className="text-caption text-secondary">{label}</figcaption>
      {image.url !== undefined && <img src={image.url} alt={m.thumbAlt(label)} className="w-40 rounded-sm" />}
      <span className="font-mono text-tag">
        {sha === "" ? m.thumbNone : image.failed ? m.thumbFailed(shortHash(sha)) : m.thumbStored(shortHash(sha))}
      </span>
    </figure>
  );
}

/**
 * Before and after of one rendered screen: the thumbnails are fetched by hash from the server's store with the
 * caller's read access, and the short hash stays beneath each as its name.
 */
export function ScreenTiles({
  screen,
  appId,
  requestId,
}: {
  readonly screen: VisualDiffScreen;
  readonly appId: string;
  readonly requestId: string;
}): ReactElement {
  const m = evidenceMessages(useLingui().i18n);
  return (
    <div className="flex flex-wrap gap-2">
      <Tile label={m.thumbBefore} sha={screen.beforeSha256} appId={appId} requestId={requestId} />
      <Tile label={m.thumbAfter} sha={screen.afterSha256} appId={appId} requestId={requestId} />
    </div>
  );
}
