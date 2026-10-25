// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Command } from "../../../lib/commands/index.ts";

export type StructureKind = "wrap" | "unwrap" | "duplicate";

export interface CanvasCommandText {
  readonly wrap: string;
  readonly unwrap: string;
  readonly duplicate: string;
}

/**
 * The page-scoped canvas commands. Chords avoid browser-reserved ones: Ctrl/Cmd+D (bookmark), Ctrl+G (find next),
 * Ctrl+Shift+G (find previous) are taken, so all three use Ctrl/Cmd+Alt: G wraps, Shift+G unwraps, J duplicates.
 */
export function canvasCommands(text: CanvasCommandText, run: (kind: StructureKind) => void): readonly Command[] {
  const command = (id: string, title: string, shortcut: string, kind: StructureKind): Command => ({
    id,
    title,
    group: "edit",
    shortcut,
    permission: null,
    run: () => run(kind),
  });
  return [
    command("canvas.wrap", text.wrap, "mod+alt+g", "wrap"),
    command("canvas.unwrap", text.unwrap, "mod+alt+shift+g", "unwrap"),
    command("canvas.duplicate", text.duplicate, "mod+alt+j", "duplicate"),
  ];
}
