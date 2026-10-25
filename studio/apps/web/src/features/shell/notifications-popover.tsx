// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Count, Popover } from "@plux/ui";
import { useParams, useRouter } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { useInbox } from "../notifications/use-inbox.ts";
import { notifyMessages } from "./notify.messages.ts";

const latest = 5;

/** E9: the latest notifications with a link to O14; the unread count follows the events stream. */
export function NotificationsPopover(): ReactElement {
  const m = notifyMessages(useLingui().i18n);
  const router = useRouter();
  const org = (useParams({ strict: false }) as { org?: string }).org;
  const inbox = useInbox({ unreadOnly: false, limit: latest });
  const unread = Number(inbox.data?.unread ?? 0n);
  const items = inbox.data?.items ?? [];
  return (
    <Popover
      title={m.bell}
      trigger={
        <button type="button" className="inline-flex items-center gap-1 text-body">
          <span>{m.bell}</span>
          {unread > 0 && <Count value={unread} tone="warning" label={m.unread(unread)} />}
        </button>
      }
    >
      {items.length === 0 && <p className="text-body text-secondary">{m.empty}</p>}
      <ul className="flex flex-col gap-1">
        {items.map((item) => (
          <li key={item.id} className="text-body">
            <a href={item.link} className={item.read ? "text-secondary" : "font-medium"}>
              {item.title}
            </a>
          </li>
        ))}
      </ul>
      {org !== undefined && (
        <button
          type="button"
          className="text-body underline"
          onClick={() => router.history.push(`/o/${org}/notifications`)}
        >
          {m.openInbox}
        </button>
      )}
    </Popover>
  );
}
