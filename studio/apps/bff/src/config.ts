// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limitDefault } from "@plux/schema";

export type BffConfig = {
  publicBaseUrl: string;
  apiUpstream: string;
  host: string;
  port: number;
  drainTimeoutMs: number;
  telemetryEnabled: boolean;
  telemetrySampleRate: number;
  maxStreamConnections: number;
  requestsPerMinute: number;
  streamsPerSession: number;
  telemetryEventsPerMinute: number;
};

export type LoadedConfig = { config: BffConfig; warnings: string[] };

export class ConfigError extends Error {}

type Env = Record<string, string | undefined>;

const localHosts = new Set(["localhost", "127.0.0.1", "[::1]"]);

function parseUrl(name: string, value: string): URL {
  try {
    const url = new URL(value);
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      throw new ConfigError(`${name} must be an http(s) URL`);
    }
    return url;
  } catch (e) {
    if (e instanceof ConfigError) {
      throw e;
    }
    throw new ConfigError(`${name} is not a valid URL`);
  }
}

function parseDuration(name: string, value: string): number {
  const m = /^(\d+)(ms|s|m)$/.exec(value);
  if (m === null) {
    throw new ConfigError(`${name} must look like 500ms, 30s or 2m`);
  }
  const n = Number(m[1]);
  return m[2] === "ms" ? n : m[2] === "s" ? n * 1000 : n * 60000;
}

function positiveInt(name: string, value: unknown): number {
  const n = typeof value === "string" ? Number(value) : value;
  if (typeof n !== "number" || !Number.isInteger(n) || n <= 0) {
    throw new ConfigError(`${name} must be a positive integer`);
  }
  return n;
}

function pick(file: Record<string, unknown>, env: Env, key: string, envName: string): unknown {
  return env[envName] ?? file[key];
}

function readFile(fileText: string | undefined): Record<string, unknown> {
  if (fileText === undefined) {
    return {};
  }
  try {
    const parsed: unknown = JSON.parse(fileText);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      throw new Error("not an object");
    }
    return parsed as Record<string, unknown>;
  } catch {
    throw new ConfigError("the configuration file is not a JSON object");
  }
}

function readUrls(file: Record<string, unknown>, env: Env, warnings: string[]): { pub: URL; up: URL } {
  const publicRaw = pick(file, env, "publicBaseUrl", "PLUX_BFF_PUBLIC_BASE_URL");
  const upstreamRaw = pick(file, env, "apiUpstream", "PLUX_BFF_API_UPSTREAM");
  if (typeof publicRaw !== "string") {
    throw new ConfigError("PLUX_BFF_PUBLIC_BASE_URL is required");
  }
  if (typeof upstreamRaw !== "string") {
    throw new ConfigError("PLUX_BFF_API_UPSTREAM is required");
  }
  const pub = parseUrl("publicBaseUrl", publicRaw);
  const up = parseUrl("apiUpstream", upstreamRaw);
  if (pub.protocol === "http:" && !localHosts.has(pub.hostname)) {
    warnings.push("UNSAFE: publicBaseUrl is plain http outside localhost; session cookies need https");
  }
  if (up.username !== "" || up.password !== "") {
    warnings.push("UNSAFE: apiUpstream carries credentials in the URL; they are ignored, use a network policy");
    up.username = "";
    up.password = "";
  }
  return { pub, up };
}

/** Reads the configuration from PLUX_BFF_* variables over an optional JSON file; fails fast. */
export function loadConfig(env: Env, fileText?: string): LoadedConfig {
  const file = readFile(fileText);
  const warnings: string[] = [];
  const { pub, up } = readUrls(file, env, warnings);
  const drainRaw = pick(file, env, "drainTimeout", "PLUX_BFF_DRAIN_TIMEOUT") ?? "30s";
  const sampleRaw = pick(file, env, "telemetrySampleRate", "PLUX_BFF_TELEMETRY_SAMPLE_RATE") ?? 1;
  const sample = Number(sampleRaw);
  if (!Number.isFinite(sample) || sample < 0 || sample > 1) {
    throw new ConfigError("telemetrySampleRate must be between 0 and 1");
  }
  const enabledRaw = pick(file, env, "telemetryEnabled", "PLUX_BFF_TELEMETRY_ENABLED") ?? true;
  const port = positiveInt("port", pick(file, env, "port", "PLUX_BFF_PORT") ?? 8081);
  if (port > 65535) {
    throw new ConfigError("port must be at most 65535");
  }
  const config: BffConfig = {
    publicBaseUrl: pub.origin,
    apiUpstream: up.origin,
    host: String(pick(file, env, "host", "PLUX_BFF_HOST") ?? "127.0.0.1"),
    port,
    drainTimeoutMs: parseDuration("drainTimeout", String(drainRaw)),
    telemetryEnabled: enabledRaw === true || enabledRaw === "true",
    telemetrySampleRate: sample,
    maxStreamConnections: positiveInt(
      "maxStreamConnections",
      pick(file, env, "maxStreamConnections", "PLUX_BFF_MAX_STREAM_CONNECTIONS") ?? 1000,
    ),
    requestsPerMinute: positiveInt(
      "requestsPerMinute",
      pick(file, env, "requestsPerMinute", "PLUX_BFF_REQUESTS_PER_MINUTE") ?? limitDefault("bff.requestsPerMinute"),
    ),
    streamsPerSession: positiveInt(
      "streamsPerSession",
      pick(file, env, "streamsPerSession", "PLUX_BFF_STREAMS_PER_SESSION") ?? limitDefault("bff.streamsPerSession"),
    ),
    telemetryEventsPerMinute: limitDefault("bff.telemetryEventsPerMinute"),
  };
  return { config, warnings };
}
