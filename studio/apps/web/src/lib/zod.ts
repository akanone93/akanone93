// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** Zod runs without code generation: no `new Function` under the strict CSP (rule book §11.2). */
export function configureZod(): void {
  z.config({ jitless: true });
}
