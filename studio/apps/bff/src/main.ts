// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { fileURLToPath } from "node:url";
import { createApp } from "./app.ts";

// biome-ignore lint/complexity/useLiteralKeys: noPropertyAccessFromIndexSignature requires bracket access
const hostname = process.env["PLUX_BFF_HOST"] ?? "127.0.0.1";
// biome-ignore lint/complexity/useLiteralKeys: noPropertyAccessFromIndexSignature requires bracket access
const port = Number(process.env["PLUX_BFF_PORT"] ?? "8081");
const staticDir = fileURLToPath(new URL("../../web/dist", import.meta.url));

const server = Bun.serve({ fetch: createApp({ staticDir }).fetch, hostname, port });
process.stdout.write(`plux-bff listening on http://${server.hostname}:${server.port}\n`);
