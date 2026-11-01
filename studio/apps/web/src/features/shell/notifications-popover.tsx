// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Button, buttonVariants } from "@plux/ui/components/button";
import { Popover, PopoverContent, PopoverTrigger } from "@plux/ui/components/popover";
import { Separator } from "@plux/ui/components/separator";
import { Link } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { useInbox } from "../notifications/use-inbox.ts";
import { notifyMessages } from "./notify.messages.ts";

const latest = 5;

/** "5 minutes ago", in the reader's language. */
function ago(locale: string, date: Date, now: number): string {
  const seconds = Math.round((date.getTime() - now) / 1000);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  for (const [unit, size] of [
    ["day", 86_400],
    ["hour", 3600],
    ["minute", 60],
  ] as const) {
    if (Math.abs(seconds) >= size) {
      return rtf.format(Math.round(seconds / size), unit);
    }
  }
  return rtf.format(seconds, "second");
}

/**
 * E9: the bell of the header with the latest notifications and the way to all of them (O14). The unread count follows
 * the events stream; a notification that links into Studio opens there.
 */
export function NotificationsPopover(): ReactElement {
  const { i18n } = useLingui();
  const m = notifyMessages(i18n);
  const { session } = useApp();
  const org = session.home();
  const [open, setOpen] = useState(false);
  const inbox = useInbox({ unreadOnly: false, limit: latest });
  const unread = Number(inbox.data?.unread ?? 0n);
  const items = inbox.data?.items ?? [];
  const now = Date.now();
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button variant="ghost" size="icon-sm" className="relative" aria-label={m.bell}>
            <Icon name="bell" />
            {unread > 0 && (
              <span className="absolute -top-0.5 -end-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-xs font-medium text-primary-foreground">
                <span className="sr-only">{m.unread(unread)}</span>
                <span aria-hidden="true">{unread > 9 ? "9+" : unread}</span>
              </span>
            )}
          </Button>
        }
      />
      <PopoverContent align="end" className="w-80 gap-0 p-0">
        <div className="flex items-center justify-between gap-2 px-4 py-3">
          <h2 className="text-sm font-semibold">{m.bell}</h2>
          {unread > 0 && <Badge variant="secondary">{m.unread(unread)}</Badge>}
        </div>
        <Separator />
        {items.length === 0 ? (
          <div className="flex flex-col items-center gap-2 px-4 py-8 text-center text-sm text-muted-foreground">
            <Icon name="inbox" />
            {m.empty}
          </div>
        ) : (
          <ul className="flex max-h-96 flex-col overflow-y-auto py-1">
            {items.map((item) => {
              const inside = item.link.startsWith("/");
              const body = (
                <>
                  <span
                    className={`mt-1.5 size-2 shrink-0 rounded-full ${item.read ? "bg-transparent" : "bg-primary"}`}
                    aria-hidden="true"
                  />
                  <span className="flex min-w-0 flex-col gap-0.5">
                    <span className={`text-sm ${item.read ? "text-muted-foreground" : "font-medium"}`}>
                      {item.title}
                    </span>
                    {item.createdAt !== undefined && (
                      <span className="text-xs text-muted-foreground">
                        {ago(i18n.locale, timestampDate(item.createdAt), now)}
                      </span>
                    )}
                  </span>
                </>
              );
              const row = "flex gap-3 px-4 py-2.5 hover:bg-muted focus-visible:bg-muted focus-visible:outline-none";
              return (
                <li key={item.id}>
                  {inside ? (
                    <Link to={item.link} className={row} onClick={() => setOpen(false)}>
                      {body}
                    </Link>
                  ) : (
                    <a href={item.link} className={row}>
                      {body}
                    </a>
                  )}
                </li>
              );
            })}
          </ul>
        )}
        {org !== undefined && (
          <>
            <Separator />
            <div className="p-2">
              <Link
                to="/o/$org/notifications"
                params={{ org }}
                className={buttonVariants({ variant: "ghost", size: "sm", className: "w-full" })}
                onClick={() => setOpen(false)}
              >
                {m.openInbox}
              </Link>
            </div>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
}
