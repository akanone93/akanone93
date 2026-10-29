// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import pkg from "../package.json" with { type: "json" };
import { createBff } from "./app.ts";
import { ConfigError, decodeStateSecret, loadConfig } from "./config.ts";
import { connectValkey } from "./valkey.ts";

const env = process.env;
let loaded: ReturnType<typeof loadConfig>;
try {
  // biome-ignore lint/complexity/useLiteralKeys: noPropertyAccessFromIndexSignature requires bracket access
  const file = env["PLUX_BFF_CONFIG"];
  loaded = loadConfig(env, file === undefined ? undefined : readFileSync(file, "utf8"));
} catch (e) {
  const msg = e instanceof ConfigError ? e.message : "cannot read the configuration";
  process.stderr.write(`plux-bff: ${msg}\n`);
  process.exit(2);
}
for (const w of loaded.warnings) {
  process.stderr.write(`plux-bff: warning: ${w}\n`);
}
const { config } = loaded;
const staticDir = fileURLToPath(new URL("../../web/dist", import.meta.url));

const stateKey = decodeStateSecret(config.stateSecret);
if (stateKey === null) {
  throw new ConfigError("stateSecret is invalid");
}

const bff = createBff({
  staticDir,
  config,
  studioVersion: pkg.version,
  stateKey,
  ...(config.valkeyUrl === ""
    ? {}
    : {
        limiter: connectValkey(config.valkeyUrl, config.requestsPerMinute),
        telemetryLimiter: connectValkey(config.valkeyUrl, config.telemetryEventsPerMinute, "tl"),
      }),
  clientIp: (req) => server.requestIP(req)?.address ?? "unknown",
});
const server = Bun.serve({
  fetch: bff.app.fetch,
  hostname: config.host,
  port: config.port,
  idleTimeout: 0,
});
process.stdout.write(`plux-bff listening on http://${server.hostname}:${server.port}\n`);

process.once("SIGTERM", () => {
  bff.startDrain();
  process.stdout.write("plux-bff draining\n");
  const deadline = Date.now() + config.drainTimeoutMs;
  const timer = setInterval(() => {
    if (bff.openStreams() === 0 || Date.now() >= deadline) {
      clearInterval(timer);
      void server.stop(true).then(() => process.exit(0));
    }
  }, 200);
});
