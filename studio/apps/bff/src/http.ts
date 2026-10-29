// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export const text = { "Content-Type": "text/plain; charset=UTF-8" };

export function cookieValue(req: Request, name: string): string {
  for (const part of (req.headers.get("cookie") ?? "").split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === name) {
      return v.join("=");
    }
  }
  return "";
}
