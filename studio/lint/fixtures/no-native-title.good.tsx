// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

declare const Dialog: (p: { title: string; children?: unknown }) => null;
declare const Tooltip: (p: { content: string; children?: unknown }) => null;
declare const m: { close: string; heading: string };
export const a = (
  <Dialog title={m.heading}>
    <Tooltip content={m.close}>
      <button type="button" aria-label={m.close} />
    </Tooltip>
  </Dialog>
);
