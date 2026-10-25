// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** O14 search: `?type&unread`. Bad values fall back to "no filter" rather than failing the page. */
export const inboxSearchSchema = z.object({
  type: z.string().optional().catch(undefined),
  unread: z.boolean().optional().catch(undefined),
});

export type InboxSearch = z.infer<typeof inboxSearchSchema>;
