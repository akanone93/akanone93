// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { DocumentService } from "@plux/api";
import { Button, Field, Input, Textarea } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useState } from "react";
import { PageEmpty } from "../../components/page.tsx";
import { useApp } from "../../lib/use-app.tsx";
import { lockTransport } from "../editor/lock-transport.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { Frame } from "../releases/frame.tsx";
import { ExperimentsTabs } from "../releases/tabs.tsx";
import { Table } from "../security/parts.tsx";
import { usePermissions, useScope } from "../security/use-security.ts";
import { type ExperimentsMessages, experimentsMessages } from "./experiments.messages.ts";
import {
  readSegment,
  ruleOf,
  ruleText,
  type SegmentDoc,
  type SegmentForm,
  segmentDocument,
  segmentKeyPattern,
  segmentPath,
} from "./segments.ts";

const blank: SegmentForm = { key: "", name: "", description: "", conditions: "", existing: undefined };

/** The segment documents of the app's draft, every page of the listing followed. */
function useSegments(app: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["segments", app],
    queryFn: async () => {
      const documents = createClient(DocumentService, session.transport);
      const out: SegmentDoc[] = [];
      let pageToken = "";
      do {
        const r = await documents.listDocuments({
          appId: app,
          pluginId: "",
          contentIncluded: true,
          page: { pageSize: 100, pageToken },
        });
        for (const d of r.documents) {
          let json: unknown;
          try {
            json = JSON.parse(new TextDecoder().decode(d.content));
          } catch {
            continue;
          }
          const seg = readSegment(d.path, d.revision, json);
          if (seg !== undefined) {
            out.push(seg);
          }
        }
        pageToken = r.page?.nextPageToken ?? "";
      } while (pageToken !== "");
      return out.sort((a, b) => a.key.localeCompare(b.key));
    },
  });
}

/**
 * Writes or deletes one segment document under the app's editing lock, taken for this one change and released after
 * it, as the server refuses a draft write without a lock session.
 */
function useSegmentActions(app: string) {
  const { session } = useApp();
  const locked = async (
    change: (documents: ReturnType<typeof createClient<typeof DocumentService>>, lock: string) => Promise<unknown>,
  ) => {
    const lock = lockTransport(session.transport, { appId: app, pluginId: "" });
    const editing = crypto.randomUUID();
    await lock.acquire(editing, false);
    try {
      await change(createClient(DocumentService, session.transport), editing);
    } finally {
      await lock.release(editing, { keepalive: false }).catch(() => undefined);
    }
    await session.queryClient.invalidateQueries({ queryKey: ["segments", app] });
  };
  return {
    save: (doc: Record<string, unknown>, existing: SegmentDoc | undefined) =>
      locked((documents, lock) =>
        documents.putDocument({
          appId: app,
          pluginId: "",
          session: lock,
          path: segmentPath(String(doc["key"])),
          content: new TextEncoder().encode(JSON.stringify(doc, null, 2)),
          ifRevision: existing?.revision ?? 0n,
        }),
      ),
    remove: (seg: SegmentDoc) =>
      locked((documents, lock) =>
        documents.deleteDocument({ appId: app, pluginId: "", session: lock, path: seg.path, ifRevision: seg.revision }),
      ),
  };
}

function SegmentFields({
  m,
  form,
  setForm,
}: {
  readonly m: ExperimentsMessages;
  readonly form: SegmentForm;
  readonly setForm: (f: SegmentForm) => void;
}): ReactElement {
  return (
    <>
      <Field label={m.segmentKeyField}>
        <Input
          disabled={form.existing !== undefined}
          value={form.key}
          onChange={(e) => setForm({ ...form, key: e.target.value })}
        />
      </Field>
      {form.key !== "" && !segmentKeyPattern.test(form.key) && (
        <p role="alert" className="text-body text-danger">
          {m.badSegmentKey}
        </p>
      )}
      <Field label={m.segmentNameField}>
        <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
      </Field>
      <Field label={m.segmentDescriptionField}>
        <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
      </Field>
      <Field label={m.segmentConditionsField}>
        <Textarea rows={4} value={form.conditions} onChange={(e) => setForm({ ...form, conditions: e.target.value })} />
        <span className="text-caption text-muted-foreground">{m.segmentConditionsHelp}</span>
      </Field>
      {form.conditions.trim() !== "" && ruleOf(form.conditions) === undefined && (
        <p role="alert" className="text-body text-danger">
          {m.badSegmentConditions}
        </p>
      )}
    </>
  );
}

/** The segments of the app (REL-051, ABT-006): the audiences flags, experiments and rollouts name by key. */
export function SegmentsPage(): ReactElement {
  const m = experimentsMessages(useLingui().i18n);
  const { app } = useScope();
  const segments = useSegments(app);
  const actions = useSegmentActions(app);
  const allowed = usePermissions().has("app.manage");
  const [form, setForm] = useState<SegmentForm>(blank);
  const [failed, setFailed] = useState(false);
  const doc = segmentDocument(form, () => uuidv7());
  const run = (change: Promise<unknown>, after?: () => void) => {
    setFailed(false);
    change.then(
      () => after?.(),
      () => setFailed(true),
    );
  };
  return (
    <Frame
      nav={<ExperimentsTabs />}
      id="segments-title"
      title={m.segmentsTitle}
      caption={m.segmentsCaption}
      loading={segments.isPending}
      failed={segments.isError}
      error={segments.error}
    >
      {(segments.data ?? []).length === 0 ? (
        <PageEmpty icon="users" text={m.noSegments} />
      ) : (
        <Table
          caption={m.segmentsTitle}
          heads={[m.segmentKeyField, m.segmentNameField, m.segmentRuleHead, ""]}
          rows={(segments.data ?? []).map((s) => {
            const text = ruleText(s.rule);
            return {
              key: s.path,
              cells: [
                <span key="k" className="font-mono">
                  {s.key}
                </span>,
                s.name,
                <span key="r" className="whitespace-pre-line font-mono text-caption">
                  {text ?? m.segmentRuleComplex}
                </span>,
                <span key="a" className="flex gap-2">
                  <Button
                    variant="secondary"
                    disabled={!allowed || text === undefined}
                    aria-label={m.editNamed(s.key)}
                    onClick={() =>
                      setForm({
                        key: s.key,
                        name: s.name,
                        description: s.description,
                        conditions: text ?? "",
                        existing: s,
                      })
                    }
                  >
                    {m.editFlag}
                  </Button>
                  <Button
                    variant="secondary"
                    disabled={!allowed}
                    aria-label={m.deleteNamed(s.key)}
                    onClick={() => run(actions.remove(s), () => form.existing?.path === s.path && setForm(blank))}
                  >
                    {m.deleteFlag}
                  </Button>
                </span>,
              ],
            };
          })}
        />
      )}
      <h2 className="text-heading">
        {form.existing === undefined ? m.newSegmentHeading : m.editSegmentHeading(form.key)}
      </h2>
      <SegmentFields m={m} form={form} setForm={setForm} />
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.actionFailed}
        </p>
      )}
      <span className="flex gap-2">
        <Button
          disabled={!allowed || doc === undefined}
          onClick={() => doc !== undefined && run(actions.save(doc, form.existing), () => setForm(blank))}
        >
          {m.saveSegment}
        </Button>
        {form.existing !== undefined && (
          <Button variant="secondary" onClick={() => setForm(blank)}>
            {m.cancelEdit}
          </Button>
        )}
      </span>
    </Frame>
  );
}
