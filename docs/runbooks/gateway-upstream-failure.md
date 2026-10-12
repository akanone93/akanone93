# Gateway Upstream Failure Runbook

Calls through the gateway fail because the customer's API (the *upstream*) is down, slow,
rejecting the gateway, or unreachable (`SEC-030`, `SEC-031`,
[ADR-0056](../adr/0056-secure-gateway-and-sca.md)). Background: the [gateway
guide](../guides/gateway.md) and [reference](../reference/gateway.md).

## Symptoms

- Devices report `PLX-6062` (`GATEWAY_UPSTREAM_FAILED`): the upstream did not answer in time or
  answered with a failure the gateway could not pass on.
- Pages bound to a gateway data source show their error state; the outbox keeps mutations and
  retries them through the gateway with the same idempotency keys.
- Transactions fail. They are never queued; the user retries.
- A burst of `PLX-6064` (`GATEWAY_RATE_LIMITED`) or `PLX-6061` (`GATEWAY_DESTINATION_REFUSED`)
  after a change.

## Signals

A finished gateway call is one RPC of service `plux.gateway`: `method` is the upstream's
name, `code` the HTTP status.

| Signal | Query |
|---|---|
| 5xx share per upstream | `sum by (method)(rate(plux_rpc_requests_total{service="plux.gateway",code=~"5.."}[5m])) / sum by (method)(rate(plux_rpc_requests_total{service="plux.gateway"}[5m]))` |
| Latency per upstream | `histogram_quantile(0.95, sum by (le, method)(rate(plux_rpc_duration_seconds_bucket{service="plux.gateway"}[5m])))` |
| Refusals before forwarding | `plux_rpc_requests_total{service="plux.v1.GatewayService", code!="ok"}` by `method` and `code` |
| Added latency budget | `NFR-025`: 5 ms p95 at 1,000 req/s per replica, excluding the upstream |

Alert when an upstream's 5xx share exceeds 5% for 5 minutes, or its p95 exceeds
`gateway.timeout` (10 s by default).

## Diagnosis

Work from the cheapest check.

1. **Is the upstream itself healthy?** Call its health endpoint from a host on the gateway's
   network. If it is down, the fix is the customer's.
2. **Which status?** `502/504` or timeouts: reachability, DNS or a slow upstream. `401/403`: the
   gateway's credentials. `400` on everything: a changed contract. `429`: the upstream limits
   the gateway as one client, since every device appears as the gateway.
3. **Credentials.** For OAuth 2.0 client credentials the gateway caches the access token until
   expiry; a rotated client secret fails at the next fetch. Check the environment secret the
   upstream's `…Ref` names (`ListSecrets`) and the token endpoint's response. For mTLS check the
   client certificate's expiry. For an API key check the header and the secret.
4. **Destination.** `PLX-6061` after nothing changed in your registry usually means DNS now
   resolves to a private, loopback, link-local or metadata address, or the base URL gained a
   redirect (redirects are not followed). Private addresses need `allowPrivateAddresses`
   on the upstream; link-local and metadata addresses are never allowed.
5. **The upstream rejects the assertion.** Check its verification: `typ`
   `plux-assertion+jwt`, `aud` equal to the registered audience (the base URL when none),
   `exp` 60 s (clock skew between the upstream and the Plux server), and the key set at
   `/.well-known/plux/gateway-jwks/{production,development}`. After a key rotation an
   upstream that cached the old key set must refetch on an unknown `kid`.
6. **Limits.** `PLX-6064` is `gateway.requestsPerMinutePerDevice` (600). A body over
   `gateway.requestBytes` (1 MiB) or a response over `gateway.responseBytes` (8 MiB) fails; so
   does a call longer than the upstream's `timeoutMs` or `gateway.timeout`.
7. **After a release.** Creating a release checks the documents against the registry. A
   release that passed the check cannot name an unknown upstream, but a later `UpdateUpstream` or
   `DeleteUpstream` can break running apps. Check the audit log for upstream changes.

## Mitigation

- **Upstream down:** nothing to fix on the Plux side. Tell users; mutations queued in the
  outbox replay when it returns. Do not mark the source `direct` as a workaround: a new
  release is needed, `maximum` refuses it, and a transaction can never be `direct`.
- **Credential expired or rotated:** set the environment secret again (`AppService.SetSecret`
  with the same key). A replica keeps the connections and tokens it built for up to 5 minutes, so the
  change reaches every replica within 5 minutes; `UpdateUpstream` (any change) makes a new
  version and forces new connections at once. No restart is needed.
- **Slow upstream:** raise `timeoutMs` on that upstream (up to the registry maximum, 60 s), or
  tighten the operation's scope. Do not raise limits globally.
- **Wrong destination:** correct `baseUrl`, or set `allowPrivateAddresses` for an upstream on a
  private network. Review that choice: it lets the gateway reach the server's network.
- **Rate limited by the upstream:** agree a higher limit for the gateway's identity, or add
  gateway replicas only if the limit is per connection.
- **Assertion rejected after a key rotation:** the upstream refetches the key set; if it cannot,
  restore the old key version during the overlap (see [key compromise](key-compromise.md) for
  a rotation under attack, which is the opposite case).
- **The gateway itself is overloaded:** scale `server.roles: [gateway]` replicas apart from
  `api`.

## Recovery

1. Watch the 5xx share and p95 fall.
2. Devices retry on their own; the outbox replays queued mutations in order per plugin, each
   with its idempotency key, so a double submission cannot happen at the upstream if it
   honours `Idempotency-Key`.
3. Transactions that failed during the outage were not charged: nothing reached the
   upstream without a verified signature, and a challenge is single use. Users repeat the
   confirmation.
4. If you changed an upstream, record why in the change ticket; the registry change is already
   in the audit log.

## Prevention

- Monitor the upstream independently of the gateway.
- Track expiry dates of OAuth client secrets and mTLS certificates; rotate before expiry.
- Keep the assertion check simple in the upstream and test it against the development key
  set (`…/gateway-jwks/development`) in staging.
- Make upstream operations idempotent.
- Run the `gateway` role apart from `api` if gateway load is high; the added latency is
  gated in CI by the `NFR-025` benchmark.
