# Compose stack

A complete single-node Plux installation (`DEP-002`): `plux-server` running the `api` and
`worker` roles, with PostgreSQL, SeaweedFS (the S3 store), Valkey, the OpenTelemetry
Collector, Prometheus and Grafana beside it. The roles that run tenant code are separate
containers that hold no database, storage or signing credential (`FN-013`, `FN-016`):
`plux-fnrunner` and `plux-egress` always, `plux-builder` under the `functions` profile (see
[Function roles](#function-roles)). Every image is pinned by digest. Studio joins the stack
in P11.

## Use

| Command | What it does |
|---|---|
| `make compose-up` | Generates credentials on first run, builds the server image and starts the stack; `COMPOSE_PROFILES=functions make compose-up` also builds and starts the function builder |
| `make dev` | The same with hot reload of the server (`compose watch`); on first run seeds an administrator and the loan calculator promoted to `staging` |
| `make compose-seed` | Starts the stack and seeds it once, without watching |
| `make compose-test` | Runs the Go integration and end-to-end tests against the stack's PostgreSQL, SeaweedFS and Valkey (`QA-005`) |
| `make compose-down` | Stops the stack; volumes are kept (`docker compose … down -v` deletes them) |

Addresses, all bound to `127.0.0.1`: server <https://localhost:8080> (TLS 1.3, see [Development TLS](#development-tls)), Grafana
<http://localhost:3000>, Prometheus <http://localhost:9090>. The optional profiles
`keycloak` (an OpenID Connect provider on port 8180), `ollama` and `functions` (the function
builder) start with `docker compose --profile <name> …`.

## Secrets

`init-secrets.sh` writes random credentials into `.secrets/` (mode 0600, ignored by Git)
the first time it runs and never overwrites them. The seed step adds `.secrets/dev.env`
with the administrator's password, a CLI token and the IDs of the seeded organisation and
app — use them with `PLUX_TOKEN=… plux …`. Delete `.secrets/` together with the volumes to
start again from nothing.

PostgreSQL's own `postgres` user is the administrator. `postgres-init.sh` creates the role
`plux-server` connects as when the volume is first created: it owns the schema but is
neither a superuser nor `BYPASSRLS`, so row-level security binds it, and the server refuses
to start as a role that it would not bind (`SRV-022`). A stack created before this change
connects as a superuser and no longer starts: delete `.secrets/` and the volumes
(`docker compose -f deploy/compose/compose.yaml down -v`) and run `make compose-up` again.

## Function roles

Server functions (`FN-020`) run on three more roles of the same binary, each in its own
container so that a compromise of tenant code or of a compiler reaches nothing worth
stealing (`ADR-0063`, `ADR-0064`). None has a database URL, an object-store key, a keys
volume or the Vault token; each mounts only its own configuration and its own certificate.

| Service | Role | Listens on | Image | Reaches |
|---|---|---|---|---|
| `plux-fnrunner` | `fnrunner`: runs function modules on wazero (`FN-009` to `FN-013`) | `:8445`, mutual TLS | `plux-server:local` | `plux-server:8443` (modules and key-value store), `plux-egress:8444` |
| `plux-egress` | `egress`: CONNECT-only proxy with the per-function domain allowlist and the SSRF rules (`FN-017`) | `:8444`, mutual TLS | `plux-server:local` | the internet, ports 443 and 80 |
| `plux-builder` | `builder`: compiles uploaded Rust sources (`FN-016`); profile `functions` | `:8446`, mutual TLS | `plux-builder:local` | nothing |

`plux-server` calls the runner at `functions.runnerURL` and the builder at `builder.address`
(`plux-server.yaml`) and serves `FunctionHostService` to the runner on `server.internalListen`
(`:8443`). Without the `functions` profile the builder is absent: everything else works and
builds fail as unavailable.

Networks: `functions` is an `internal` Docker network, which has no route out of the host.
`plux-fnrunner` and `plux-builder` are on it alone; `plux-server` is on it and on `default`;
only `plux-egress` is also on `outbound`, a normal network. The function runner therefore
reaches the internet through the egress proxy and nothing else, and cannot reach PostgreSQL,
SeaweedFS or Valkey. Each role's configuration is a file beside this one
(`plux-egress.yaml`, `plux-builder.yaml`, `plux-fnrunner.yaml`); the runner's is completed in
`.secrets/` with its trusted key (below). The roles log JSON to standard output; they export
no telemetry in this stack.

### Internal TLS

The roles talk over mutual TLS 1.3 (`SEC-043`): every listener demands a client certificate
and every client trusts only the internal CA. `init-secrets.sh` creates, idempotently and
like the development TLS certificate (renewed within 30 days of expiry, keys kept):

| File | What |
|---|---|
| `.secrets/ca/internal-ca.crt`, `internal-ca.key` | The internal CA, ECDSA P-256, valid 1 year. It is not the development CA above, which people install in their operating system: trusting that one must never vouch for a role. Its name constraints allow only `plux-server`, `plux-fnrunner`, `plux-egress` and `plux-builder`. The key is mounted nowhere. |
| `.secrets/internal/<service>/tls.crt`, `tls.key`, `ca.crt` | One identity per service, valid 90 days, with the service name as its only name and both `serverAuth` and `clientAuth` (every role is a server and a client). A service mounts its own directory at `/etc/plux/internal`, so it cannot read another's key. |

After a renewal restart the service (`docker compose -f deploy/compose/compose.yaml restart
plux-fnrunner`). When the CA expires, delete `.secrets/ca/internal-ca.*` and
`.secrets/internal` and run `make compose-up`; nothing needs trusting on the host.

### The function-artifact key

Only artifacts that the builder produced and the worker signed with the function-artifact
key are deployable, and `fnrunner` checks the signature against public keys from its own
configuration, never from the api role (`FN-009`). The file signing backend would create
that key inside the `plux-keys` volume on first use, so its public half could not be in
`fnrunner`'s configuration beforehand. `init-secrets.sh` therefore makes the key itself:

- `.secrets/function-artifact.ed25519` is the private key in the backend's file format (the
  32-byte seed followed by the public key). `plux-server` mounts it read-only as
  `/var/lib/plux/keys/function-artifact.ed25519`, where the backend finds it.
- `.secrets/plux-fnrunner.yaml` is `plux-fnrunner.yaml` plus `fnrunner.artifactKeys` with
  the public key and its identifier (the first 16 bytes of its SHA-256, hexadecimal), and is
  rewritten by every run of the script.

The file backend never signs for an environment marked production (`SEC-056`); a production
installation signs through Vault Transit or an HSM and lists that key's public half in
`fnrunner.artifactKeys`.

### Function builder

`plux-builder` runs every build in new user, network, mount and PID namespaces
(`builder.sandbox: namespaces`, `backend/internal/builder/sandbox_linux.go`); where the
kernel denies them it refuses to build, and the unsandboxed `none` setting is not used here.
It starts with the hardened defaults of the other services (`read_only`, `cap_drop: [ALL]`,
`no-new-privileges`, user `65532`, no credential, the `functions` network only) and has
exactly two additions:

| Addition | Why it is needed |
|---|---|
| `seccomp=./seccomp-builder.json` | The sandbox starts each compiler process with `clone(2)` and `CLONE_NEWUSER`, `CLONE_NEWNET`, `CLONE_NEWNS` and `CLONE_NEWPID`. Docker's default profile allows `clone` only without a namespace flag unless the container has `CAP_SYS_ADMIN`, so the call fails with `EPERM` and the builder refuses to build. The profile is Docker's default (`moby/profiles`, `seccomp/default.json`, SHA-256 `6416b47770785a41ac59073cdc77d9fe98517df2799dc83ef207e622de3053f6`) plus one rule that allows `clone` when `CLONE_NEWIPC`, `CLONE_NEWUTS` and `CLONE_NEWCGROUP` are clear, so those three stay denied. No capability is added: creating a user namespace needs none, and the root-like capabilities a process holds inside it apply to that namespace only. The filter is inherited by the sandboxed processes, so `mount`, `unshare` and `setns` stay denied inside it. |
| `tmpfs` `/var/lib/plux/build` with `exec` | Each build unpacks into a scratch directory under `builder.workDir`, and cargo runs build scripts and procedural macros (compiled for the build host) from there. Docker mounts a `tmpfs` `noexec` by default. The mount is memory-backed, `nosuid`, `nodev`, mode `0700`, owned by the builder's user, limited to 2 GiB, and gone with the container. |

Not granted: `privileged`, `cap_add`, `seccomp=unconfined`, `apparmor=unconfined`, the host's
PID, network or IPC namespace, the Docker socket, any volume with data, and any network but
`functions`, which has no route out. The sandbox gives builds no network inside the
container either, and cargo runs offline on vendored crates.

The host kernel must allow unprivileged user namespaces for the container's user
(`user.max_user_namespaces` above 0; on Debian `kernel.unprivileged_userns_clone=1`). Some
distributions restrict them further, for example Ubuntu 24.04 with
`kernel.apparmor_restrict_unprivileged_userns=1`; then the builder logs that its sandbox is
unavailable and refuses every build. Allow it for the container's AppArmor profile or relax
the sysctl on a development machine; do not switch the sandbox off.

The image (`backend/Dockerfile`, target `builder`) pins Rust 1.97.0 with `wasm32-wasip1`,
Bun 1.3.11 running the npm package `binaryen@132.0.0`, and the SDK crate `sdk/fn/plux-fn`
at `/opt/plux/sdk/plux-fn`, the same pins as `plux fn build` and `sdk/fn/rust-toolchain.toml`.

## Development TLS

The API terminates TLS itself, as a production installation with `server.tls` does
(`SEC-040`): TLS 1.3 only, with `Strict-Transport-Security`, on <https://localhost:8080>
(still bound to `127.0.0.1`). `init-secrets.sh` needs `openssl` and, on every run, makes
sure these exist:

| File | What |
|---|---|
| `.secrets/ca/ca.crt`, `ca.key` | A development CA, valid 1 year. Its name constraints allow only `localhost`, `plux-server`, `127.0.0.1`, `::1` and `10.0.2.2`, so trusting it cannot vouch for any other site. The key never leaves `.secrets/ca`. |
| `.secrets/tls/server.crt`, `server.key` | The server certificate for those names, ECDSA P-256, valid 90 days. Mounted read-only into `plux-server`. The key is kept when the certificate is renewed. |
| `.secrets/tls/ca.crt` | A copy of the CA certificate for clients and Prometheus (which scrapes `/metrics` over HTTPS with it). |

A certificate within 30 days of expiry is re-issued by the next `make compose-up`; restart
the server (`docker compose -f deploy/compose/compose.yaml restart plux-server`) to load
it. When the CA itself expires, delete `.secrets/ca` and run `make compose-up`, then trust
the new CA. The script refuses to run when `plux-server.yaml` sets `production: true`: the
self-signed certificate is for development only, and production configures its own.

The stack's HSTS policy lasts 10 minutes and has no `includeSubDomains`. Browsers apply HSTS
to a host on every port, so the production default (two years) would make them force HTTPS
on <http://localhost:3000> (Grafana) and <http://localhost:9090> too.

### Trusting the CA

Command-line checks need only the file: `curl --cacert deploy/compose/.secrets/tls/ca.crt
-v https://localhost:8080/readyz`. `make` runs the `plux` CLI with `SSL_CERT_FILE`, which Go
honours on Linux; on macOS and Windows trust the CA as below first. Browsers and apps need it
in the operating system's store:

- **macOS:** `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain deploy/compose/.secrets/tls/ca.crt`
- **Debian, Ubuntu:** `sudo cp deploy/compose/.secrets/tls/ca.crt /usr/local/share/ca-certificates/plux-dev-ca.crt && sudo update-ca-certificates`
- **Fedora, RHEL:** copy it to `/etc/pki/ca-trust/source/anchors/` and run `sudo update-ca-trust`
- **iOS simulator:** `xcrun simctl keychain booted add-root-cert deploy/compose/.secrets/tls/ca.crt`
- **Android emulator:** `adb push deploy/compose/.secrets/tls/ca.crt /sdcard/` and install it under Settings,
  Security, Encryption and credentials, Install a certificate, CA certificate. The starter app's debug
  build trusts user-installed CAs (its `network_security_config.xml`); release and profile builds do not.
  `make dev-app` runs `adb reverse`, so `https://localhost:8080` works; `10.0.2.2` is in the certificate too.

Remove the CA again with the platform's reverse of the command above when you delete `.secrets/`.

### The runtime's pins

Debug builds with an `https` endpoint may leave `pins` empty (`SEC-041`), which is what the dev
app does: the platform's trust in the CA above is the check. To exercise pinning, pin the
server key, which survives certificate renewals:

```sh
openssl x509 -in deploy/compose/.secrets/tls/server.crt -pubkey -noout |
  openssl pkey -pubin -outform DER | openssl dgst -sha256 -binary | openssl base64
```

The runtime wants at least two distinct pins; make the second from a spare key with
`openssl ecparam -name prime256v1 -genkey -noout | openssl pkey -pubout -outform DER | openssl dgst -sha256 -binary | openssl base64`.

## Files

| File | Purpose |
|---|---|
| `compose.yaml` | The stack |
| `plux-server.yaml` | The server's configuration inside the stack, including `server.tls`, the internal listener and the routes to the function roles |
| `plux-fnrunner.yaml`, `plux-egress.yaml`, `plux-builder.yaml` | The configuration of the function roles; the first is completed in `.secrets/` |
| `seccomp-builder.json` | The builder's seccomp profile: Docker's default plus the `clone` rule for the build sandbox |
| `init-secrets.sh` | Generates the credentials, the development CA and certificate, the internal CA and identities, and the function-artifact key |
| `compose.dev.yaml` | Overlay for `make dev`: rebuilds and restarts the server on source changes |
| `compose.test.yaml` | Overlay for `make compose-test`: publishes the stores on high local ports |
| `compose.load.yaml`, `plux-server.load.yaml` | Overlay for the load test (`NFR-020`): trusts Docker's private ranges as proxies so the generator can name each device's address. **Never use it outside a load test.** |
| `otel-collector.yaml`, `prometheus.yml`, `grafana/` | Telemetry pipeline |

## Production

The stack signs with the file backend, which refuses to sign for an environment marked
production (`SEC-056`). A production installation configures Vault Transit in
`plux-server.yaml`, a managed PostgreSQL and S3 store, and its own certificate in `server.tls` (or a TLS 1.3 proxy with `server.behindTLSProxy`);
see the [server runbook](../../docs/runbooks/server.md).
