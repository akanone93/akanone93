// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Select } from "@plux/ui";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { notifyMessages } from "../shell/notify.messages.ts";
import { useInbox, useMarkRead } from "./use-inbox.ts";

const allTypes = "";

/** O14: the full inbox, filtered by event type and unread state held in the URL. */
export function NotificationsPage() {
  const m = notifyMessages(useLingui().i18n);
  const search = useSearch({ strict: false }) as { type?: string; unread?: boolean };
  const navigate = useNavigate();
  const unreadOnly = search.unread === true;
  const inbox = useInbox({ unreadOnly, limit: 200 });
  const markRead = useMarkRead();
  const items = inbox.data?.items ?? [];
  const types = [...new Set(items.map((i) => i.eventType))].sort();
  const shown = search.type === undefined ? items : items.filter((i) => i.eventType === search.type);
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
      {inbox.isError && (
        <p role="alert" className="text-body text-danger">
          {m.loadFailed}
        </p>
      )}
      {inbox.data !== undefined && shown.length === 0 && <p className="text-body text-secondary">{m.empty}</p>}
      <ul className="flex flex-col gap-2">
        {shown.map((item) => (
          <li key={item.id} className="flex items-start gap-3 rounded-lg bg-raised p-3">
            <div className="flex flex-1 flex-col">
              <a href={item.link} className="text-body font-medium">
                {item.title}
              </a>
              <span className="text-body text-secondary">{item.body}</span>
              <span className="font-mono text-tag text-tertiary">{item.eventType}</span>
            </div>
            {!item.read && (
              <Button variant="ghost" size="sm" onClick={() => void markRead([item.id])}>
                {m.markRead}
              </Button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
