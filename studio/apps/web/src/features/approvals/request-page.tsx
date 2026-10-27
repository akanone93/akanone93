// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { ApprovalRequest } from "@plux/api";
import { Button, Field, Textarea } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { fmt } from "../security/parts.tsx";
import { approvalsMessages } from "./approvals.messages.ts";
import { AuditRecords, DiffBlock, EvidenceBlocks } from "./evidence.tsx";
import { Frame, Status } from "./parts.tsx";
import { useStepUpGate } from "./step-up-gate.tsx";
import { useApproval, useDecide, useMe, useScope } from "./use-approvals.ts";

/** The decision form: the comment, then the step-up D3, then the decision (D47). The server's refusal is shown as worded. */
function DecideForm({ request }: { readonly request: ApprovalRequest }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const decide = useDecide();
  const can = useMe().permissions.has("approvals.decide") && request.status === "open";
  const [comment, setComment] = useState("");
  const [decision, setDecision] = useState<"approve" | "reject">("approve");
  const gate = useStepUpGate(() => decide(request.appId, request.id, decision, comment), m.decideFailed);
  const start = (chosen: "approve" | "reject") => {
    setDecision(chosen);
    gate.begin();
  };
  return (
    <section className="flex flex-col gap-3" aria-labelledby="decide-title">
      <h2 id="decide-title" className="text-section">
        {m.decideHeading}
      </h2>
      <Field label={m.comment}>
        <Textarea value={comment} onChange={(e) => setComment(e.target.value)} />
      </Field>
      <div className="flex items-center gap-3">
        <Button disabled={!can} onClick={() => start("approve")}>
          {m.approve}
        </Button>
        <Button variant="secondary" disabled={!can} onClick={() => start("reject")}>
          {m.reject}
        </Button>
        {!can && <span className="text-caption text-secondary">{m.decideNeedsPermission}</span>}
      </div>
      {gate.problem !== undefined && (
        <p role="alert" className="text-body text-danger">
          {gate.problem}
        </p>
      )}
      {gate.dialog}
    </section>
  );
}

function Decisions({ request }: { readonly request: ApprovalRequest }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-section">{m.decisionsHeading}</h2>
      {request.decisions.length === 0 ? (
        <p className="text-body text-secondary">{m.noDecisions}</p>
      ) : (
        <ul className="flex flex-col gap-2 text-body">
          {request.decisions.map((d) => (
            <li key={d.id}>
              <span className="font-medium">{d.decider}</span>{" "}
              {d.decision === "approve" ? m.decisionApprove : m.decisionReject}{" "}
              <span className="text-secondary">
                {fmt(d.decidedAt === undefined ? undefined : timestampDate(d.decidedAt), m.never)}
              </span>
              {d.comment !== "" && <p className="text-secondary">{d.comment}</p>}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** O8: one request with all its evidence, and the decision (STU-102, GOV-023). */
export function RequestPage(): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const { app } = useScope();
  const id = (useParams({ strict: false }) as { requestId?: string }).requestId ?? "";
  const review = useApproval(app, id);
  const data = review.data;
  const request = data?.approval;
  const risks: Record<string, string> = { low: m.riskLow, medium: m.riskMedium, high: m.riskHigh };
  return (
    <Frame
      id="request-title"
      title={m.requestTitle}
      {...(request !== undefined && { caption: request.summary })}
      loading={review.isPending}
      failed={review.isError || (review.isSuccess && request === undefined)}
    >
      {request !== undefined && data !== undefined && (
        <>
          <p className="flex items-center gap-3 text-body">
            <Status status={request.status} />
            <span className="font-mono">{request.subject}</span>
            <span>{request.environment}</span>
            <span className="text-secondary">{request.requestedBy}</span>
          </p>
          {request.policyKeys.length > 0 && (
            <p className="text-body">
              {m.policiesInForce}: <span className="font-mono">{request.policyKeys.join(", ")}</span>
            </p>
          )}
          <p className="text-body">
            {m.riskHeading}: <span className="font-medium">{risks[data.risk?.level ?? ""] ?? data.risk?.level}</span>{" "}
            {data.risk?.reasons.join("; ")}
          </p>
          <DiffBlock diff={data.diff} />
          <EvidenceBlocks review={data} />
          <Decisions request={request} />
          <AuditRecords sequences={request.decisions.map((d) => d.auditSequence)} />
          <DecideForm request={request} />
        </>
      )}
    </Frame>
  );
}
