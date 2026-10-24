// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Writes the Tailwind stylesheet generated from the design tokens. Run with `bun run gen:theme`. */

import { themeCss } from "../packages/brand/src/theme-css.ts";

const target = new URL("../apps/web/src/styles/theme.gen.css", import.meta.url);
await Bun.write(target, themeCss());
