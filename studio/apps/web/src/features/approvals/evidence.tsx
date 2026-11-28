// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { GetApprovalReviewResponse, ReviewImage } from "@plux/api";
import type { ReactElement } from "react";
import { PageEmpty } from "../../components/page.tsx";
import { Table } from "../security/parts.tsx";
import { approvalsMessages } from "./approvals.messages.ts";

const decoder = new TextDecoder();

/** A PNG as a `data:` URI, which the Studio's content security policy allows for images. */
export function pngSource(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return `data:image/png;base64,${btoa(binary)}`;
}

function Heading({ children }: { readonly children: string }): ReactElement {
  return <h2 className="text-section">{children}</h2>;
}

/** One changed document of a promotion's diff, as the server encodes a changelog entry. */
export interface DiffChange {
  readonly Kind: string;
  readonly Change: string;
  readonly PluginKey: string;
  readonly Name: string;
}

/** One part of a release request's diff: a plugin, or the app bundle under an empty key. */
export interface DiffPart {
  readonly pluginKey: string;
  readonly version: number;
  readonly bundleSha256: string;
  readonly warnings?: readonly string[];
}

/** A diff as Studio shows it: a promotion's changes, a release request's parts, nothing, or text of another shape. */
export type Diff =
  | { readonly kind: "changes"; readonly changes: readonly DiffChange[] }
  | { readonly kind: "parts"; readonly parts: readonly DiffPart[] }
  | { readonly kind: "empty" }
  | { readonly kind: "text"; readonly text: string };

/** A JSON value's fields of the given names, unknown until checked; null when it is no object. */
function fieldsOf<K extends string>(v: unknown): Partial<Record<K, unknown>> | null {
  return typeof v === "object" && v !== null && !Array.isArray(v) ? (v as Partial<Record<K, unknown>>) : null;
}
const isChange = (v: unknown): v is DiffChange => {
  const f = fieldsOf<keyof DiffChange>(v);
  return typeof f?.Change === "string" && typeof f.Kind === "string" && typeof f.Name === "string";
};
const isPart = (v: unknown): v is DiffPart => {
  const f = fieldsOf<keyof DiffPart>(v);
  return typeof f?.pluginKey === "string" && typeof f.version === "number" && typeof f.bundleSha256 === "string";
};

/** Reads a review's diff: the two shapes the release reviewer writes become tables, anything else stays text. */
export function readDiff(bytes: Uint8Array): Diff {
  const text = decoder.decode(bytes);
  if (text.trim() === "") {
    return { kind: "empty" };
  }
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    return { kind: "text", text };
  }
  if (value === null || (Array.isArray(value) && value.length === 0)) {
    return { kind: "empty" };
  }
  if (Array.isArray(value) && value.every(isChange)) {
    return { kind: "changes", changes: value };
  }
  if (Array.isArray(value) && value.every(isPart)) {
    return { kind: "parts", parts: value };
  }
  return { kind: "text", text };
}

function DiffBody({ diff }: { readonly diff: Diff }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const changes: Readonly<Record<string, string>> = {
    added: m.changeAdded,
    removed: m.changeRemoved,
    changed: m.changeChanged,
  };
  const kinds: Readonly<Record<string, string>> = {
    page: m.kindPage,
    action: m.kindAction,
    translation: m.kindTranslation,
  };
  switch (diff.kind) {
    case "empty":
      return <PageEmpty icon="check-circle" text={m.noDiff} />;
    case "changes":
      return (
        <Table
          caption={m.diffHeading}
          heads={[m.colChange, m.colKind, m.colPlugin, m.colName]}
          rows={diff.changes.map((c) => ({
            key: `${c.Kind}:${c.PluginKey}:${c.Name}`,
            cells: [changes[c.Change] ?? c.Change, kinds[c.Kind] ?? c.Kind, c.PluginKey, c.Name],
          }))}
        />
      );
    case "parts":
      return (
        <Table
          caption={m.diffHeading}
          heads={[m.colPart, m.colVersion, m.colBundle, m.colWarnings]}
          rows={diff.parts.map((p) => ({
            key: p.pluginKey,
            cells: [
              p.pluginKey === "" ? m.appBundle : p.pluginKey,
              String(p.version),
              <span key="sha" className="font-mono text-tag">
                {p.bundleSha256}
              </span>,
              (p.warnings ?? []).join("; "),
            ],
          }))}
        />
      );
    case "text":
      return <pre className="overflow-auto rounded-sm bg-raised p-3 font-mono text-tag">{diff.text}</pre>;
  }
}

/** The diff of the change (GOV-023): a table of what a promotion changes or of the parts a release request built. */
export function DiffBlock({ diff }: { readonly diff: Uint8Array }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-2">
      <Heading>{m.diffHeading}</Heading>
      <DiffBody diff={readDiff(diff)} />
    </section>
  );
}

function GoldenPair({ image }: { readonly image: ReviewImage }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <figure className="flex gap-4">
      <div className="flex flex-col gap-1">
        <figcaption className="text-caption text-muted-foreground">{m.baseline}</figcaption>
        {image.baseline.length === 0 ? (
          <span className="text-body text-muted-foreground">{m.noBaseline}</span>
        ) : (
          <img src={pngSource(image.baseline)} alt={`${m.baseline}: ${image.pluginKey} ${image.caseId}`} />
        )}
      </div>
      <div className="flex flex-col gap-1">
        <figcaption className="text-caption text-muted-foreground">{m.actual}</figcaption>
        <img src={pngSource(image.actual)} alt={`${m.actual}: ${image.pluginKey} ${image.caseId}`} />
      </div>
    </figure>
  );
}

/** Reports, golden-image differences, install base and changelog: what an approver reads before deciding. */
export function EvidenceBlocks({ review }: { readonly review: GetApprovalReviewResponse }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <>
      {review.reports.length > 0 && (
        <section className="flex flex-col gap-2">
          <Heading>{m.reportsHeading}</Heading>
          {review.reports.map((r) => (
            <details key={r.kind}>
              <summary className="font-mono text-tag">{r.kind}</summary>
              <pre className="overflow-auto rounded-sm bg-raised p-3 font-mono text-tag">
                {decoder.decode(r.content)}
              </pre>
            </details>
          ))}
        </section>
      )}
      {review.goldenDiffs.length > 0 && (
        <section className="flex flex-col gap-2">
          <Heading>{m.goldenHeading}</Heading>
          {review.goldenDiffs.map((g) => (
            <GoldenPair key={`${g.pluginKey}/${g.caseId}`} image={g} />
          ))}
        </section>
      )}
      {review.installBase.length > 0 && (
        <section className="flex flex-col gap-2">
          <Heading>{m.installBaseHeading}</Heading>
          <Table
            caption={m.installBaseHeading}
            heads={[m.colChannel, m.colRelease, m.colBuild, m.colDevices]}
            rows={review.installBase.map((e) => ({
              key: `${e.channel}/${e.releaseSequence}/${e.hostBuild}`,
              cells: [e.channel, String(e.releaseSequence), e.hostBuild, String(e.devices)],
            }))}
          />
        </section>
      )}
      {review.changelog.length > 0 && (
        <section className="flex flex-col gap-2">
          <Heading>{m.changelogHeading}</Heading>
          <ul className="list-disc ps-5 text-body">
            {review.changelog.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

/** The audit records the decisions were entered in; the audit log (O27) searches them by sequence. */
export function AuditRecords({ sequences }: { readonly sequences: readonly bigint[] }): ReactElement | null {
  const m = approvalsMessages(useLingui().i18n);
  if (sequences.length === 0) {
    return null;
  }
  return (
    <section className="flex flex-col gap-2">
      <Heading>{m.auditHeading}</Heading>
      <ul className="flex flex-col gap-1 text-body">
        {sequences.map((s) => (
          <li key={String(s)} className="font-mono">
            {m.auditRecord({ sequence: String(s) })}
          </li>
        ))}
      </ul>
    </section>
  );
}
