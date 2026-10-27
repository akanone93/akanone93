// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useState } from "react";
import { notifyMessages } from "../shell/notify.messages.ts";
import { isMutable, knownTypes, readMuted, writeMuted } from "./notification-kinds.ts";

/** F16: per-type mute in the profile; approval and security types are listed but locked on. */
export function MuteSettings() {
  const m = notifyMessages(useLingui().i18n);
  const [muted, setMuted] = useState<ReadonlySet<string>>(readMuted);
  const toggle = (type: string, on: boolean) => {
    const next = new Set(muted);
    if (on) {
      next.add(type);
    } else {
      next.delete(type);
    }
    writeMuted(next);
    setMuted(next);
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
