// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { PreferencesSchema, PreferencesService } from "@plux/api";
import { Badge } from "@plux/ui/components/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@plux/ui/components/card";
import { Switch } from "@plux/ui/components/switch";
import type { ReactElement } from "react";
import { notifyMessages } from "../shell/notify.messages.ts";
import {
  isMutable,
  kindOf,
  kindOrder,
  knownTypes,
  mutedSet,
  type NotificationKind,
  typeName,
} from "./notification-kinds.ts";

type Messages = ReturnType<typeof notifyMessages>;

const kindTitle = (m: Messages, kind: NotificationKind): string =>
  ({ work: m.kindWork, approvals: m.kindApprovals, operations: m.kindOperations, security: m.kindSecurity })[kind];

/**
 * F16: which notifications reach the person's inbox, stored per type in the server preferences. Each type has a
 * switch that is on while the type is not muted; approval and security types are listed, always on.
 */
export function MuteSettings(): ReactElement {
  const { i18n } = useLingui();
  const m = notifyMessages(i18n);
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  const update = useMutation(PreferencesService.method.updatePreferences);
  const muted = mutedSet(prefs.data?.preferences?.mutedNotificationTypes);
  const setMuted = (type: string, mute: boolean) => {
    const next = new Set(muted);
    if (mute) {
      next.add(type);
    } else {
      next.delete(type);
    }
    update.mutate(
      {
        preferences: create(PreferencesSchema, { mutedNotificationTypes: [...next] }),
        updateMask: { paths: ["muted_notification_types"] },
      },
      { onSuccess: () => void prefs.refetch() },
    );
  };
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <h2>{m.muteTitle}</h2>
        </CardTitle>
        <CardDescription>{m.muteCaption}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-6">
        {kindOrder.map((kind) => {
          const types = knownTypes.filter((t) => kindOf(t) === kind);
          if (types.length === 0) {
            return null;
          }
          return (
            <section key={kind} aria-label={kindTitle(m, kind)} className="flex flex-col gap-2">
              <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                {kindTitle(m, kind)}
              </h3>
              <ul className="divide-y rounded-lg border">
                {types.map((type) => {
                  const locked = !isMutable(type);
                  const name = typeName(i18n, type);
                  return (
                    <li key={type} className="flex items-center justify-between gap-4 px-4 py-3">
                      <span className="flex min-w-0 flex-col">
                        <span className="text-sm font-medium">{name}</span>
                        <span className="truncate font-mono text-xs text-muted-foreground">{type}</span>
                      </span>
                      <span className="flex items-center gap-3">
                        {locked && <Badge variant="secondary">{m.muteLocked}</Badge>}
                        <Switch
                          aria-label={m.notifyAbout(name)}
                          checked={locked || !muted.has(type)}
                          disabled={locked}
                          onCheckedChange={(on) => setMuted(type, !on)}
                        />
                      </span>
                    </li>
                  );
                })}
              </ul>
            </section>
          );
        })}
      </CardContent>
    </Card>
  );
}
