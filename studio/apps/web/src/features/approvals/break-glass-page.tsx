// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { BreakGlassInvocation } from "@plux/api";
import { Button, Dialog, Field, StatusPill, Textarea } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { fmt, Table } from "../security/parts.tsx";
import { approvalsMessages } from "./approvals.messages.ts";
import { Frame } from "./parts.tsx";
import { useStepUpGate } from "./step-up-gate.tsx";
import { useBreakGlass, useMe, useReviewBreakGlass } from "./use-approvals.ts";

/** The review of one emergency action: a comment, then the step-up D3 (SEC-103); whoever took the action cannot review it. */
function ReviewDialog({
  invocation,
  onClose,
}: {
  readonly invocation: BreakGlassInvocation;
  readonly onClose: () => void;
}): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const review = useReviewBreakGlass();
  const [comment, setComment] = useState("");
  const gate = useStepUpGate(async () => {
    await review(invocation.id, comment);
    onClose();
  }, m.reviewFailed);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
      size="sm"
      title={m.review}
      description={`${invocation.action}: ${invocation.subject}`}
      labels={{ close: m.review }}
    >
      <Field label={m.reviewComment}>
        <Textarea value={comment} onChange={(e) => setComment(e.target.value)} />
      </Field>
      {gate.problem !== undefined && (
        <p role="alert" className="text-body text-danger">
          {gate.problem}
        </p>
      )}
      <div className="flex justify-end">
        <Button onClick={gate.begin}>{m.review}</Button>
      </div>
      {gate.dialog}
    </Dialog>
  );
}

/** O10: the emergency actions and their post-hoc reviews, overdue ones flagged (GOV-024). */
export function BreakGlassPage(): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const list = useBreakGlass();
  const canReview = useMe().permissions.has("approvals.decide");
  const [reviewing, setReviewing] = useState<BreakGlassInvocation>();
  const time = (t: Parameters<typeof timestampDate>[0] | undefined) =>
    fmt(t === undefined ? undefined : timestampDate(t), m.never);
  const items = list.data?.invocations ?? [];
  return (
    <Frame
      id="break-glass-title"
      title={m.breakGlassTitle}
      caption={m.breakGlassCaption}
      loading={list.isPending}
      failed={list.isError}
    >
      {items.length === 0 ? (
        <p className="text-body text-secondary">{m.empty}</p>
      ) : (
        <Table
          caption={m.breakGlassTitle}
          heads={[
            m.colAction,
            m.colEnvironment,
            m.colJustification,
            m.colInvokedBy,
            m.colInvokedAt,
            m.colDue,
            m.colReview,
            "",
          ]}
          rows={items.map((i) => ({
            key: i.id,
            cells: [
              <span key="a" className="font-mono">{`${i.action}: ${i.subject}`}</span>,
              i.environment,
              i.justification,
              i.invokedBy,
              time(i.invokedAt),
              time(i.reviewDueAt),
              i.overdue ? (
                <StatusPill key="s" status="danger" label={m.overdue} />
              ) : (
                <StatusPill
                  key="s"
                  status={i.reviewStatus === "reviewed" ? "success" : "warning"}
                  label={i.reviewStatus === "reviewed" ? m.reviewReviewed : m.reviewOpen}
                />
              ),
              i.reviewStatus === "reviewed" ? null : (
                <Button key="r" variant="secondary" disabled={!canReview} onClick={() => setReviewing(i)}>
                  {m.review}
                </Button>
              ),
            ],
          }))}
        />
      )}
      {reviewing !== undefined && <ReviewDialog invocation={reviewing} onClose={() => setReviewing(undefined)} />}
    </Frame>
  );
}
