// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { configureZod } from "./zod.ts";

// Zod probes `new Function` when a schema is first built, so jitless must be set before any
// module that builds schemas is evaluated: main.tsx imports this file first (rule book §11.2).
configureZod();
