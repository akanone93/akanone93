# 0061. Server-held configuration

- **Status:** Accepted (maintainer, 2026-10-11, option B: all three tiers as a follow-on milestone of P6)
- **Date:** 2026-10-11
- **Requirements:** `DAT-003`, `DAT-090`–`DAT-097`, `SEC-028`, `SEC-031`, `SEC-092`, `SEC-106`, `NFR-025`

## Context and problem

Financial apps need per-plugin values that are never stale: the pool or credit account of a
service provider, a settlement account, a fee rate. A device can run an old bundle, be
offline, or have a wrong clock, so a value that travels in a bundle or is cached on the device
can be out of date at the moment the user acts on it. "Never stale" can only be enforced where
the value takes effect, and that is the server, at the moment of use. The user must never
notice the mechanism.

Environment variables (`GOV-010`) are compiled into bundles; they are right for values that
may lag, and wrong for these.

## Decision drivers

- A value used in a request to the customer's API must be the current one when the request is
  made, whatever the device holds (security and correctness come first, spec §1.4).
- A plugin must not be able to place a value into a field the upstream echoes back and so read
  it, or choose which value is sent.
- Sensitive values are sealed at rest and never leave the server (`SEC-106`, `SEC-092`).
- The check costs a cache-store read per call, within the gateway's 5 ms p95 (`NFR-025`).
- A transaction's confirmation must describe the value that is sent (`SEC-028`).

## Considered options

1. **Device placeholders** (`{{x}}` in a data source, replaced by the gateway). Rejected: a
   plugin could put a placeholder into a field the upstream echoes back and read the value,
   and a device would choose which value is sent.
2. **Declarative binding in the operation definition, resolved by the gateway per request,
   plus a live tier for values the device itself needs.** Chosen.
3. **Live fetch without server validation.** Rejected: the device would use whatever it last
   fetched, so a stale result is accepted.
4. **Bundled values only.** Rejected for financial values: they lag by a release and a sync.

## Decision

Chosen option: **2**, with three tiers the developer chooses per value:

- **`server-bound`.** Used in API calls through the gateway. The operation's definition
  declares where a value goes (JSON Pointer into a JSON body, header, path parameter, query
  parameter); the gateway resolves it for each request and injects it. The device never
  sends, chooses or sees it, and a request that sets a bound location is refused
  (`PLX-6065`). Operations without bindings keep the zero-copy streaming path.
- **`live`.** Values the device needs and that must be current. Fetched over the DPoP-bound
  channel (`ConfigService.GetLiveConfig`), held in memory only, and version-checked by the
  server on every gateway call: a value marked `strict` that changed after the version the
  call carries refuses it (`PLX-6067`), and the runtime refreshes and repeats the call once
  with the same idempotency key. Offline, a read of a `strict` value fails closed
  (`PLX-6068`).
- **`bundled`.** Values that may lag: the environment variables of `GOV-010`. Nothing is
  built; Studio and the CLI label them "may lag".

Mechanics:

- **Storage.** One row per value and environment, with a version that rises on each change,
  and one row per environment with its **epoch**, which rises on every change, and the epoch
  of the last change to a strict live value. A sensitive value is sealed with the envelope
  encryption of `SEC-106`, bound to its environment and key, and only a `server-bound`
  value may be sensitive. Every change is audited with the key, never the value.
- **Freshness.** Each replica caches the values of an environment with their epoch. A change
  writes the new epoch to the cache store before its transaction commits; every call that
  needs configuration reads that one key and reloads from the database when the epoch differs
  from the cached one. A missing key is re-seeded from the database. If the key cannot be read
  the call is refused (`PLX-6069`): the gateway fails closed.
- **Transactions.** A binding may be marked `display`. At challenge creation the gateway
  resolves it, merges it into the summary the user signs, returns it for the confirmation
  sheet, and keeps it with the challenge. At execution the current value is compared with the
  kept one; a difference refuses the call (`PLX-6066`) and the runtime re-challenges
  transparently.
- **Epoch.** Every gateway response carries `Plux-Config-Epoch`; a device that sees a newer
  one refreshes its live values in the background.

## Consequences

- **Positive:** a bound value is never stale where it takes effect; plugins cannot read or
  choose bound values; the confirmation shows what is sent; the bundle format is unchanged.
- **Negative:** one extra cache-store read per gateway call (budget: at most 0.2 ms p95 added,
  part of the 5 ms of `NFR-025`); the gateway cannot serve a call that needs configuration
  while the cache store is down; a path parameter is inserted after the operation's path
  prefix, which is less flexible than a template.
- **Follow-up:** the runtime's live configuration client, Studio's tier labels and the
  confirmation sheet (S13 runtime slice).

## Options in detail

### Option 1: device placeholders

Simple to author, and wrong in principle: the device decides what is sent, and a value that
the upstream echoes can be read back by the plugin. Rejected.

### Option 2: declarative binding and a live tier

The operation, which an administrator registers on the server, is the only place that names a
location, so the plugin has no say. The version check makes freshness a property of the
request rather than of any cache. Change notification: the epoch key in the cache store is
the notification. A publish-subscribe channel was considered to evict entries eagerly; it
would add a long-lived connection to every replica and gain nothing, because every use is
gated by the epoch check anyway.

### Option 3: live fetch without validation

Gives the device current values at fetch time only. A device that fetched, went offline and
came back would use a value that changed meanwhile, and nothing would notice. Rejected.

### Option 4: bundled only

Correct for values that may lag, which is why it stays as the third tier. For financial values
the lag is a release, a publish and a sync: unacceptable.
