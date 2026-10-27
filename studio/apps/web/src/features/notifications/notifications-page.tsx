// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Select } from "@plux/ui";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { notifyMessages } from "../shell/notify.messages.ts";
import {
  describe,
  groupItems,
  isMutable,
  kindOf,
  kindOrder,
  type NotificationKind,
  readMuted,
  retentionDays,
} from "./notification-kinds.ts";
import { useInbox, useMarkRead } from "./use-inbox.ts";

const allTypes = "";

/** O14: the full inbox, filtered by event type and unread state held in the URL. */
export function NotificationsPage() {
  const { i18n } = useLingui();
  const m = notifyMessages(i18n);
  const search = useSearch({ strict: false }) as { type?: string; unread?: boolean };
  const navigate = useNavigate();
  const unreadOnly = search.unread === true;
  const inbox = useInbox({ unreadOnly, limit: 200 });
  const markRead = useMarkRead();
  const muted = readMuted();
  const items = (inbox.data?.items ?? []).filter((i) => !muted.has(i.eventType) || !isMutable(i.eventType));
  const types = [...new Set(items.map((i) => i.eventType))].sort();
  const shown = search.type === undefined ? items : items.filter((i) => i.eventType === search.type);
  const groups = groupItems(shown);
  const kindTitle: Record<NotificationKind, string> = {
    work: m.kindWork,
    approvals: m.kindApprovals,
    operations: m.kindOperations,
    security: m.kindSecurity,
  };
  const setSearch = (next: { type?: string | undefined; unread?: boolean | undefined }) =>
    void navigate({ to: ".", search: (prev: object) => ({ ...prev, ...next }) });
  return (
    <section className="flex flex-col gap-4" aria-labelledby="inbox-title">
      <h1 id="inbox-title" className="text-screen">
        {m.inboxTitle}
      </h1>
      <div className="flex items-end gap-4">
        <Select
          label={m.filterType}
          value={search.type ?? allTypes}
          placeholder={m.filterAll}
          options={[{ value: allTypes, label: m.filterAll }, ...types.map((t) => ({ value: t, label: t }))]}
          onValueChange={(value) => setSearch({ type: value === allTypes ? undefined : value })}
        />
        <label className="flex items-center gap-2 text-body">
          <input
            type="checkbox"
            checked={unreadOnly}
            onChange={(e) => setSearch({ unread: e.target.checked ? true : undefined })}
          />
          {m.filterUnread}
        </label>
      </div>
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="sm"
          disabled={!items.some((i) => !i.read)}
          onClick={() => void markRead(items.filter((i) => !i.read).map((i) => i.id))}
        >
          {m.markAllRead}
        </Button>
        <span className="text-caption text-secondary">{m.retention(retentionDays())}</span>
      </div>
      {inbox.isError && (
        <p role="alert" className="text-body text-danger">
          {m.loadFailed}
        </p>
      )}
      {inbox.data !== undefined && shown.length === 0 && <p className="text-body text-secondary">{m.empty}</p>}
      {kindOrder.map((kind) => {
        const ofKind = groups.filter((g) => kindOf(g.eventType) === kind);
        if (ofKind.length === 0) {
          return null;
        }
        return (
          <section key={kind} aria-label={kindTitle[kind]} className="flex flex-col gap-2">
            <h2 className="text-body font-medium">{kindTitle[kind]}</h2>
            <ul className="flex flex-col gap-2">
              {ofKind.map((group) => {
                const first = group.items[0];
                if (first === undefined) {
                  return null;
                }
                const audited = kind === "approvals" || kind === "security";
                return (
                  <li key={group.key} className="flex items-start gap-3 rounded-lg bg-raised p-3">
                    <div className="flex flex-1 flex-col">
                      <span className={`text-body ${group.unread ? "font-medium" : ""}`}>
                        {describe(i18n, first, group.count)}
                      </span>
                      {audited && first.link !== "" && (
                        <a href={first.link} className="text-caption">
                          {m.auditRecord}
                        </a>
                      )}
                      <span className="font-mono text-tag text-tertiary">{group.eventType}</span>
                    </div>
                    {group.unread && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => void markRead(group.items.filter((i) => !i.read).map((i) => i.id))}
                      >
                        {m.markRead}
                      </Button>
                    )}
                  </li>
                );
              })}
            </ul>
          </section>
        );
      })}
    </section>
  );
}
