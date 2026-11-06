// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The interaction state the inspector edits on the selected widgets (PC-21): picking Pressed on a button's background
 * draws the selection pressed — its pressed values and Material's state layer — until Default is picked again or the
 * selection changes. Only the drawn copy carries it (`x-studio.state`, through the live surface's values).
 */

import { useCallback, useMemo, useState } from "react";
import type { EditedState, StatePreview } from "../panels/state-editors.tsx";
import type { LiveState } from "./live.ts";

/** The live values with `state` previewed on `ids`; `live` itself for the default state. */
export function withStates(live: LiveState, ids: readonly string[], state: EditedState): LiveState {
  return state === "value" || ids.length === 0 ? live : { ...live, states: new Map(ids.map((id) => [id, state])) };
}

/** The previewed state of the selection `ids`, reset when the selection changes. */
export function useStatePreview(ids: readonly string[]): StatePreview {
  const key = ids.join("\u0000");
  const [picked, setPicked] = useState<{ readonly key: string; readonly state: EditedState }>({
    key: "",
    state: "value",
  });
  const show = useCallback((state: EditedState) => setPicked({ key, state }), [key]);
  return useMemo(() => ({ state: picked.key === key ? picked.state : "value", show }), [picked, key, show]);
}
