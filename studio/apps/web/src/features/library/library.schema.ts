// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** O4 search: `?kind&q&visibility&owner&category&tag`. Bad values fall back to "no filter". */
export const librarySearchSchema = z.object({
  kind: z.enum(["component", "template"]).optional().catch(undefined),
  q: z.string().optional().catch(undefined),
  visibility: z.enum(["private", "organization", "public"]).optional().catch(undefined),
  owner: z.string().optional().catch(undefined),
  category: z.string().optional().catch(undefined),
  tag: z.string().optional().catch(undefined),
});

export type LibrarySearch = z.infer<typeof librarySearchSchema>;
