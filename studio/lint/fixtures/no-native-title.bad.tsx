// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

declare const Button: (p: { title?: string; children?: unknown }) => null;
declare const m: { close: string };
export const a = <button type="button" title={m.close} />;
export const b = <span title={m.close}>x</span>;
export const c = (
  <Button title={m.close}>
    <span />
  </Button>
);
