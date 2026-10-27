// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { GetApprovalReviewResponse, ReviewImage } from "@plux/api";
import type { ReactElement } from "react";
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

/** The JSON diff of the change (GOV-023). */
export function DiffBlock({ diff }: { readonly diff: Uint8Array }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-2">
      <Heading>{m.diffHeading}</Heading>
      {diff.length === 0 ? (
        <p className="text-body text-secondary">{m.noDiff}</p>
      ) : (
        <pre className="overflow-auto rounded-sm bg-raised p-3 font-mono text-tag">{decoder.decode(diff)}</pre>
      )}
    </section>
  );
}

function GoldenPair({ image }: { readonly image: ReviewImage }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <figure className="flex gap-4">
      <div className="flex flex-col gap-1">
        <figcaption className="text-caption text-secondary">{m.baseline}</figcaption>
        {image.baseline.length === 0 ? (
          <span className="text-body text-secondary">{m.noBaseline}</span>
        ) : (
          <img src={pngSource(image.baseline)} alt={`${m.baseline}: ${image.pluginKey} ${image.caseId}`} />
        )}
      </div>
      <div className="flex flex-col gap-1">
        <figcaption className="text-caption text-secondary">{m.actual}</figcaption>
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
