// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { createContext, type ReactElement, useContext } from "react";
import { timelineMessages } from "../plugin-canvas/timelines.messages.ts";
import { addTimelineRecipe, removeTimelineRecipe, timelinesOf } from "../plugin-canvas/timelines.ts";

/**
 * The timelines of a page (PC-30, ANI-002): each with its length and tracks, opened on the canvas to scrub, play and
 * key; added and removed here.
 */

type Obj = Record<string, unknown>;
type Change = (recipe: (draft: Obj) => void, field: string) => void;

/** Opens a page's timeline on the canvas; null outside a canvas. */
export const TimelineOpenContext = createContext<((page: string, id: string) => void) | null>(null);

export function TimelinesSection(p: {
  readonly pageId: string;
  readonly page: unknown;
  readonly canEdit: boolean;
  readonly change: Change;
  readonly newId: () => string;
}): ReactElement {
  const m = timelineMessages(useLingui().i18n);
  const open = useContext(TimelineOpenContext);
  const list = timelinesOf(p.page);
  const names = list.map((t) => t.name);
  return (
    <div className="forms-section">
      {list.map((t) => (
        <div key={t.id} className="form-card timeline-card">
          <div className="form-card-head">
            <Icon name="film" size="pill" />
            <span className="timeline-name">{t.name}</span>
            <span className="timeline-meta">{m.meta((t.durationMs / 1000).toFixed(2), t.tracks.length)}</span>
            {open !== null && (
              <button
                type="button"
                className="form-icon"
                aria-label={m.open(t.name)}
                onClick={() => open(p.pageId, t.id)}
              >
                <Icon name="play" size="pill" />
              </button>
            )}
            {p.canEdit && (
              <button
                type="button"
                className="form-icon"
                aria-label={m.remove(t.name)}
                onClick={() => p.change(removeTimelineRecipe(t.id), `timeline:${t.id}:remove`)}
              >
                <Icon name="trash" size="pill" />
              </button>
            )}
          </div>
        </div>
      ))}
      {p.canEdit && (
        <button
          type="button"
          className="form-add"
          onClick={() => {
            let n = 1;
            while (names.includes(n === 1 ? "intro" : `intro${n}`)) {
              n += 1;
            }
            const id = p.newId();
            p.change(addTimelineRecipe(id, n === 1 ? "intro" : `intro${n}`), "timelines:add");
            open?.(p.pageId, id);
          }}
        >
          <Icon name="plus" size="pill" />
          {m.add}
        </button>
      )}
    </div>
  );
}
