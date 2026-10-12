# Replay-Cache Outage Runbook

The shared cache (Valkey) that remembers DPoP proof IDs is unavailable (`SEC-023`,
[ADR-0012](../adr/0012-dpop-hardware-keys-and-attestation.md)). The same Valkey holds
registration challenges, revocation flags, rate-limit counters and the DPoP-adjacent short
caches, so the effects reach beyond replay detection.

## How the server behaves

Every device request records its proof ID (`jti`, scoped by the key thumbprint) with
`SET NX` for twice the `iat` window plus 60 s. If Valkey fails:

| Policy | Behaviour | Profile |
|---|---|---|
| `memory` | Each replica records proofs in a bounded in-memory store (`dpop.replayCacheEntries`, 1,000,000 by default), applies the narrower window `dpopIatWindowFallback` (15 s), logs a warning at most every 10 s, and raises the gauge below. Requests are never accepted unchecked | `standard`, `strict` |
| `failClosed` | The request is refused with `PLX-6015` | `maximum` |

**As built (P6).** The policy is the environment's: each device call takes it from the
effective `replayCacheFallback` of the environment its token names, so a `maximum`
environment fails closed while a `standard` one beside it keeps working on its own replica.
The `standard` default (`memory`) applies to calls that carry no token (registration and
refresh), which name no environment yet. `/readyz` also reports `cache: unavailable`, so a
load balancer that honours readiness stops sending traffic to the replicas. Revocation checks
fall back to the database, so a revoked device stays refused.

The fallback is weaker in one way: a proof captured while the shared cache is down can be
replayed once to **each other replica** inside the 15-second window, because replicas do not
share their stores. The window is narrow and the proof still needs a valid token bound to
the same key and a fresh server nonce. When the cache returns, proofs held by a replica are
still checked before a proof is accepted.

## Signals

| Signal | Meaning |
|---|---|
| `plux_dpop_replay_cache_degraded` | Gauge: 1 while a replica is deciding alone. Alert at 1 for more than 2 minutes |
| `rate(plux_dpop_replay_cache_fallback_total[5m])` | Checks the per-replica store decided; the size of the exposure |
| `/readyz` reports `cache: unavailable` | Valkey does not answer; replicas drop out of rotation |
| Log: "the DPoP replay cache is unavailable; this replica remembers proofs itself and applies the narrower issued-at window (SEC-023)" | Degraded mode began on that replica |
| `plux_rpc_requests_total{code!="ok"}` rising with `PLX-6015` in clients | Fail-closed refusals |
| `plux_rpc_requests_total{method="CreateRegistrationChallenge"}` failures | The challenge store is the same cache |
| `plux_dpop_rejections_total{reason}` | Catalogued in Appendix G.1; not yet exported |

## Diagnosis

1. Is Valkey up? `valkey-cli -h <host> -p 6379 --tls ping` (the Compose stack's service
   name is in `deploy/compose`).
2. With Sentinel (`cache.valkeyURL: "valkeys+sentinel://s1:26379,s2:26379/plux"`): ask a
   Sentinel for the master, `valkey-cli -p 26379 sentinel get-master-addr-by-name plux`,
   and check that a quorum of Sentinels agrees. The server's client follows the promoted
   master; it never reads from replicas, because the replay cache and rate limits need
   the master's view.
3. A failover takes seconds. Degraded mode for longer means no master was promoted, the
   Sentinels cannot reach a quorum, TLS or passwords on the URL changed, or the network
   between the `api` replicas and Valkey is broken.
4. Memory pressure: a Valkey at `maxmemory` with `noeviction` refuses writes. The replay keys
   are small and expire in minutes.
5. Check the URL: errors never echo it (it can hold passwords). Look at the replica's
   config, not its logs.

## Mitigation

- Restore Valkey or force a Sentinel failover (`sentinel failover plux`).
- If Valkey cannot come back soon and the installation must not run degraded, take the
  `api` replicas out of rotation. Do not switch `cache.backend` to `memory` on a multi-replica
  installation: each replica would then keep its own state for everything, including
  rate limits and revocations.
- Do not loosen `dpopIatWindow` to "help" clients. Clock-skew failures during an outage are
  a different problem; the fallback window is deliberately narrow.

## Recovery

When Valkey answers again, the gauge returns to 0 on each replica by itself. No restart is
needed. Then check:

1. Registration challenges issued during the outage are gone if Valkey lost data; devices that
   fail with `PLX-6005` (challenge invalid) simply retry.
2. Revocation flags (`device:revoked:<jkt>`) kept only in Valkey are lost if it restarted
   without persistence. The database is authoritative and `IsRevoked` falls back to it, but
   re-publish if in doubt: calling `RevokeDevice` on a revoked device publishes the flag again.
3. A Valkey that restarted empty has forgotten proofs seen before the outage. A proof is
   replayable only inside its own window (60 s, 30 s under `maximum`) and still needs a
   current server nonce (rotated every 300 s, 120 s under `maximum`).

## Prevention

- Run Valkey with Sentinel (three Sentinels, quorum 2) and persistence, TLS on, in
  production (`server.production: true` refuses unencrypted links).
- Alert on the gauge, not only on `/readyz`.
- Size `dpop.replayCacheEntries` for your peak: requests per second times the window times 2,
  per replica.
- Rehearse a failover in staging each quarter. The Sentinel failover tests in CI prove the
  client; they do not prove your topology.
