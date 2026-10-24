// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";
import type { authMessages } from "./auth.messages.ts";

type Messages = ReturnType<typeof authMessages>;

/** Form schemas; the messages come from the catalogue, so the schemas are built per render. */
export const signInSchema = (m: Messages) =>
  z.object({
    email: z.string().check(z.email(m.emailInvalid)),
    password: z.string().min(1, m.passwordRequired),
  });

export const codeSchema = (m: Messages) => z.object({ code: z.string().regex(/^[0-9]{6}$/, m.codeInvalid) });

export const inviteSchema = (m: Messages) =>
  z.object({
    displayName: z.string().trim().min(1, m.nameRequired),
    password: z.string().min(12, m.passwordShort),
  });

/** A `next` search param is kept only when it is a same-origin path. */
export const signInSearchSchema = z.object({ next: z.string().optional() });
