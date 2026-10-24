// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

const requestAllow = [
  "cookie",
  "x-csrf-token",
  "x-plux-organization",
  "content-type",
  "idempotency-key",
  "traceparent",
];
const requestPrefixes = ["connect-", "accept"];
const responseAllow = [
  "content-type",
  "content-encoding",
  "cache-control",
  "retry-after",
  "grpc-status",
  "grpc-message",
  "trailer",
];
const responsePrefixes = ["connect-", "grpc-"];
const sessionCookie = "__Host-plux_session=";

export function filterRequestHeaders(src: Headers): Headers {
  const out = new Headers();
  for (const [k, v] of src) {
    if (requestAllow.includes(k) || requestPrefixes.some((p) => k.startsWith(p))) {
      out.set(k, v);
    }
  }
  return out;
}

export function filterResponseHeaders(src: Headers): Headers {
  const out = new Headers();
  for (const [k, v] of src) {
    if (responseAllow.includes(k) || responsePrefixes.some((p) => k.startsWith(p))) {
      out.set(k, v);
    }
  }
  for (const c of src.getSetCookie()) {
    if (c.startsWith(sessionCookie)) {
      out.append("Set-Cookie", c);
    }
  }
  return out;
}

export type ProxyResult = { response: Response; release: () => void };

/**
 * Forwards a request to the upstream and streams the body both ways without buffering.
 * `release` is called once the response body ends or is cancelled.
 */
export async function proxyRequest(
  req: Request,
  upstream: string,
  path: string,
  release: () => void,
): Promise<Response> {
  const url = new URL(path, upstream);
  const hasBody = req.method !== "GET" && req.method !== "HEAD";
  const controller = new AbortController();
  req.signal.addEventListener("abort", () => controller.abort(), { once: true });
  let res: Response;
  try {
    const init: RequestInit & { duplex?: "half" } = {
      method: req.method,
      headers: filterRequestHeaders(req.headers),
      signal: controller.signal,
      redirect: "manual",
    };
    if (hasBody) {
      init.body = req.body;
      init.duplex = "half";
    }
    res = await fetch(url, init);
  } catch {
    release();
    return new Response("upstream unavailable", {
      status: 502,
      headers: { "Content-Type": "text/plain; charset=UTF-8" },
    });
  }
  if (res.body === null) {
    release();
    return new Response(null, { status: res.status, headers: filterResponseHeaders(res.headers) });
  }
  const reader = res.body.getReader();
  const body = new ReadableStream<Uint8Array>({
    async pull(c) {
      try {
        const r = await reader.read();
        if (r.done) {
          release();
          c.close();
        } else {
          c.enqueue(r.value);
        }
      } catch (e) {
        release();
        c.error(e);
      }
    },
    cancel(reason) {
      release();
      controller.abort();
      return reader.cancel(reason).catch(() => undefined);
    },
  });
  return new Response(body, { status: res.status, headers: filterResponseHeaders(res.headers) });
}
