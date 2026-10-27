// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { PreferencesSchema, PreferencesService } from "@plux/api";
import { notifyMessages } from "../shell/notify.messages.ts";
import { isMutable, knownTypes, mutedSet } from "./notification-kinds.ts";

/** F16: per-type mute in the server preferences; approval and security types are listed but locked on. */
export function MuteSettings() {
  const m = notifyMessages(useLingui().i18n);
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  const update = useMutation(PreferencesService.method.updatePreferences);
  const muted = mutedSet(prefs.data?.preferences?.mutedNotificationTypes);
  const toggle = (type: string, on: boolean) => {
    const next = new Set(muted);
    if (on) {
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
    <section aria-label={m.muteTitle} className="flex flex-col gap-2">
      <h2 className="text-body font-medium">{m.muteTitle}</h2>
      {knownTypes.map((type) => (
        <label key={type} className="flex items-center gap-2 text-body">
          <input
            type="checkbox"
            checked={muted.has(type) && isMutable(type)}
            disabled={!isMutable(type)}
            onChange={(e) => toggle(type, e.target.checked)}
          />
          <span className="font-mono text-tag">{type}</span>
          <span className="text-caption text-secondary">{isMutable(type) ? m.muteLabel : m.muteLocked}</span>
        </label>
      ))}
    </section>
  );
}
