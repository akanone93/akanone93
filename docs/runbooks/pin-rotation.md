# Pin Rotation Runbook

Certificate pinning fails closed: a device that cannot match the server's key to one of its
pins refuses to talk (`PLX-6020`, `SEC-041`). Rotating the server's TLS key without
rotating the pins strands every device. This runbook rotates safely, and recovers when
that went wrong.

## How pins work

- A **pin** is the base64 SHA-256 of a certificate's SubjectPublicKeyInfo (RFC 7469): it
  names a *public key*, not a certificate. Renewing a certificate on the same key keeps the pin.
- The app embeds at least two distinct pins in `PluxConfig.pins`: the key in use and a
  **backup key** not yet in use. A release build with an `https` endpoint and no pins
  refuses to start.
- On Android and iOS a connection is accepted when any certificate in the server's chain has
  a pinned key, so an intermediate can be pinned. On `dart:io` platforms (desktop
  development) only the leaf is visible.
- Pins change afterwards only through the signed `root` metadata: a `pins` entry maps a host
  to 2 to 8 pins. The device replaces its set only from a verified root whose version is newer
  than the one it holds, and the new set must again hold at least two pins.
  Older metadata cannot bring back a retired pin.
- Pins reach the root through the offline ceremony (2 of 3 holders), so every pin change is
  a root change.

## Signals

| Signal | Meaning |
|---|---|
| `pin_failure` telemetry event (host, pin set version) | Devices reject the server's key |
| `PLX-6020` in the host app's `onError` | Same, on the device |
| `plux_manifest_requests_total{result="updated"}` and `…{result="not_modified"}` falling together, with `plux_rpc_requests_total` falling | Devices cannot connect at all. The server itself sees nothing: the device refuses the handshake |
| `openssl s_client` from outside | Compare the served key's pin with the pins in the root |

Compute the pin of what the server serves now:

```bash
openssl s_client -connect plux.acme.example:443 -servername plux.acme.example </dev/null 2>/dev/null \
  | openssl x509 -pubkey -noout | openssl pkey -pubin -outform der \
  | openssl dgst -sha256 -binary | base64
```

For an intermediate, extract that certificate and use the same pipeline.

## Routine rotation

Plan this at least a quarter before the certificate's key must change. Keep the **backup key's
private half offline** until it is needed.

1. **State.** App ships pins `{A (in use), B (backup)}`; the root's `pins` entry holds the same
   or a superset.
2. **Prepare the next backup.** Generate key C offline, compute its pin.
3. **Rotate the server to B.** Issue a certificate for key B and deploy it at the edge
   (the proxy or `server.tls`). Devices accept it: B is already pinned.
4. **Publish the new pin set before retiring A.** Create a new root version with pins
   `{B, C}` (the [key ceremony](key-ceremony.md) rotation steps, with `-pins pins.json`):

   ```json
   {"plux.acme.example": ["<pin of B>", "<pin of C>"]}
   ```

   ```bash
   plux-server metadata root-new -type production -version 3 -expires 8760h -threshold 2 \
     -key h1.pem -key h2.pem -key h3.pem -online-keys online-keys.txt -pins pins.json -out root-v3.json
   ```

   Two holders sign it with their keys and two holders of the previous root sign with
   `-previous root-v2.json`; check it with `root-verify -in root-v3.json -previous root-v2.json`;
   then `upload-root`. Omitting `-pins` keeps the pins the app embeds, so only include it when the
   set changes.
5. **Wait.** Devices fetch roots when they sync, so devices that sync daily have the new set
   within a day. Devices that stay offline longer keep the old set and still work with
   certificates for A or B until they sync. Keep the certificate for A servable (or keep B in
   use) for as long as you want to support the longest-offline device; a week is typical.
6. **Retire A.** After step 5 you may stop serving certificates for A and destroy its key.
7. **Next time.** B is in use, C is the backup, and a later root will add D.

Ship the next app release with `{B, C}` in `PluxConfig.pins`, so fresh installs start from the
current set.

## Emergency: the key in use is compromised

1. Rotate the server to the backup key (step 3 above) immediately. Devices accept it.
2. Run step 4 at once with the new set, without waiting for the routine schedule, and remove
   the compromised key's pin so it cannot be used again.
3. If TLS interception was possible for a time, treat tokens as exposed: DPoP binds them to
   device keys, so they cannot be replayed from elsewhere, but see the
   [key compromise runbook](key-compromise.md) for the broader review.

## Recovery: devices fail with `PLX-6020`

- **Both pinned keys are gone** (no usable backup). Devices cannot fetch a root over the
  pinned connection either, so the root cannot repair this. Ship an app release with new pins
  and ask users to update. This is why the backup key matters: store it, offline.
- **Wrong certificate deployed** (the key is not in the pin set, for example a CDN or proxy
  swapped certificates). Redeploy a certificate with a pinned key. Devices recover
  on their next request.
- **A corporate proxy or interception** on some networks: the failure is correct. Do not
  loosen pins.
- **A root with wrong pins was uploaded** and devices already took it: devices accept only a
  newer root, with at least two pins. Publish a corrected, newer root. Devices that still
  reach the server (the served key is in the wrong set only if you changed the server key too)
  fetch it at their next sync. Check every new root against the live chain before `upload-root`.

## Prevention

- Pin the intermediate for a CA you trust to renew on the same key, in addition to a leaf and
  a backup.
- Verify the new root's pins against the live certificate chain before `upload-root`.
- Put a calendar reminder on the certificate expiry and the root's expiry (at most one year).
- Test the rotation in staging with a build pinned to staging's keys.
- A host-supplied `httpClient` and WebSocket streams to customer domains are not pinned by
  Plux; pin those in your own client or with `capabilities.networkPins` for direct domains.
