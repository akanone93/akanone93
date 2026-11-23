<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Apply Register — recreate the built app on your machine

This brings the **finished** Apply Register (17 plugins, 207 documents, 13 functions) onto a local Plux stack, so
you can open every plugin in Studio and experiment. Everything a person does in Studio is done in Studio; the
project itself is imported, not rebuilt. Commands are only used where Studio has no page (noted each time).
To build the app from nothing instead, follow the build sheet, [README.md](README.md).

Time: about an hour, most of it waiting for the first stack start and the function builds.

## 1. What you need

- Docker (Compose v2), Git, OpenSSL; Go (the version on the `toolchain` line of `backend/go.mod`), Bun 1.3.11,
  Flutter 3.47.5, Rust 1.97.0 with `rustup target add wasm32-wasip1`.
- A clone of this repository on the branch you were given, then `make setup` (pinned tools, git hooks).
- The app kit, kept on the branch `claude/handoff-temp` (the project, the assets, the host fonts and the `ar-api`
  test service). From the repository root:

  ```bash
  git fetch origin claude/handoff-temp
  mkdir -p ../ar-kit && git archive origin/claude/handoff-temp handoff/apply-register | tar -x -C ../ar-kit
  ```

  You now have `../ar-kit/handoff/apply-register/` with `project/`, `assets/`, `fonts/`, `ar-api/` and
  `db-inventory.md` (every setting of the original installation, secrets by name only). Below, `KIT` means that
  folder.

## 2. Start the stack

1. `COMPOSE_PROFILES=functions make dev` — the server, Postgres, the object store, the function builder and runner,
   egress, the render worker of visual diffs and Studio. The first start builds the images (10–20 minutes).
2. **One fix before Studio works** (build log #179, open): the development CA's IP name constraints make Studio's
   backend refuse the server's certificate (every page shows "Studio can't reach the Plux Server"). Issue the server
   a certificate for its names only, from the same CA, and restart the server:

   ```bash
   cd deploy/compose/.secrets
   printf '[req]\ndistinguished_name=dn\nprompt=no\n[dn]\nCN=localhost\n[ext]\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:localhost,DNS:plux-server\n' > /tmp/leaf.cnf
   openssl req -new -key tls/server.key -config /tmp/leaf.cnf -out /tmp/leaf.csr
   openssl x509 -req -sha256 -days 30 -in /tmp/leaf.csr -CA ca/ca.crt -CAkey ca/ca.key -CAcreateserial -extfile /tmp/leaf.cnf -extensions ext -out tls/server.crt
   cd ../../.. && docker compose -f deploy/compose/compose.yaml -f deploy/compose/compose.dev.yaml restart plux-server
   ```

   A phone or emulator then reaches the server as `localhost` through `adb reverse` (which `make dev` keeps on),
   not as `10.0.2.2`.
3. **Let the server reach `ar-api` on your machine.** On Docker Desktop (macOS, Windows) `host.docker.internal`
   works as is. On Linux add, under the `plux-server` service of `deploy/compose/compose.dev.yaml`,
   `extra_hosts: ["host.docker.internal:host-gateway"]`, and run `make dev` again.
4. Start the test service in a second terminal: `cd KIT/ar-api && go run .` (listens on `:8090`; demo sign-in
   `+15555550100`, code `000000`; staff `+15555550101`). Keep the kit outside the checkout, as §1 does: inside it,
   the repository's `go.work` takes over and `go run .` fails (run `GOWORK=off go run .` there).
5. Open Studio at <http://localhost:8081>. Sign in as `dev@plux.localhost`; the password is
   `PLUX_DEV_PASSWORD` in `deploy/compose/.secrets/dev.env` (open the file; do not paste it anywhere).

## 3. In Studio: you, the organisation, the team

1. **Profile › Security keys › Add security key.** Secrets, the gateway, restrictions, roles and publishing ask
   for it ("Confirm it's you").
2. **New organisation** (organisation menu): name `Apply Register`, key `apply-register`. The seeded `acme`
   works too; the original lived there.
3. **Administration › Teams:** `platform` (Platform), `orgs` (Orgs), `qa` (QA).
4. **Administration › Members:** invite the people who will work with you (the invitation is shown once; send it
   to them directly). Roles of the original: `release-manager`, `translator`, `auditor`.
5. **Administration › Restrictions:** subject team **Orgs**, permissions `release.publish, release.promote`,
   **In app** Apply Register, **In environment** production, reason "Designers may not publish to production".

## 4. In Studio: the app and its settings

1. **Apps › Create app › Blank:** name `Apply Register`, key `apply-register`, description "Find an organisation,
   register, follow your registration.", locales `en` (default), `de-DE`, `ar`, `am-ET` (search "Amharic" or
   "Ethiopia"). Studio creates the environments `development`, `staging` and `production`.
2. **Administration › Environments and secrets**, for each environment: `AR_GATEWAY_KEY` (any value; `ar-api`
   does not check it), the key the gateway upstreams of §6 send. `AR_PARTNER_KEY` does not go here — nothing reads
   it from this page: the functions read their `secrets` from the sensitive server-bound configuration values of §6.
3. **App canvas › Assets:** upload every file of `KIT/assets/` and `KIT/fonts/` (52 files; GIF, SVG, WebP, Lottie,
   Rive and fonts are all accepted).
4. **Design › Typography** (the app's areas in the side menu; also **Settings › Host fonts**), section **Host app
   fonts:** Inter (`fonts/inter-variable.ttf`), Noto Naskh Arabic, Noto Sans
   Ethiopic, IBM Plex Sans, Source Serif 4, JetBrains Mono — each with its uploaded file.

## 5. Import the project (CLI; Studio's import takes a `.plux` archive, the kit holds the project folder)

```bash
cd backend && go build -o ~/bin/plux ./cmd/plux && cd ..
export SSL_CERT_FILE="$PWD/deploy/compose/.secrets/ca/ca.crt" PLUX_SDK="$PWD/sdk/fn/plux-fn"
plux login --server https://localhost:8080      # approve the code in Studio with your security key
```

Edit `KIT/project/plux.json`: `server` `https://localhost:8080`, `organization` and `app` the IDs in Studio's
address bar (`/o/<organization>/apps/<app>`), `environment` `development`. Then:

```bash
plux import -C KIT/project        # replaces the app's drafts with the 207 files of the project
```

Open the app canvas: 17 plugins and the native screens `licenses` and `sign-in`, all **Valid**.

## 6. What Studio has no page for yet

**Configuration values** (CLI; they need a recent step-up, so run `plux login` again first — it lasts five
minutes). From `KIT/project`, for `--env development`, then `staging` and `production` (the last two without the
two sensitive values):

```bash
plux config set feeTable --tier live --value '{"standard":{"base":"2.50","perPage":"0.10"},"premium":{"base":"5.00","perPage":"0.05"}}' --env development
plux config set supportPhone --tier live --value '+1 555 555 0199' --env development
plux config set ratesEURUSD --tier live --type decimal --strict --value 1.0825 --env development
plux config set payeeIbanRelief --tier server-bound --value DE89370400440532013000 --env development
printf 'dev-partner-key' | plux config set AR_PARTNER_KEY --tier server-bound --sensitive --value-stdin --env development
openssl rand -base64 32 | tr -d '\n' | plux config set PASS_SIGNING_KEY --tier server-bound --sensitive --value-stdin --env development
```

Functions read their `secrets` (`AR_PARTNER_KEY`, `PASS_SIGNING_KEY`) from these sensitive server-bound values.

**Gateway upstreams, environment variables and the user identity provider** have neither a page nor a command
(build log #199). Paste this into the browser console of a Studio tab inside the organisation (any page under
`/o/…`); it asks for your security key once, then creates the 25 upstreams in each environment, the three
development variables and the identity provider of staging and production, and prints a summary:

```js
(async () => {
  const org = location.pathname.split("/")[2];
  let csrf = "";
  const call = async (m, b) => {
    const r = await fetch(`/api/plux.v1.${m}`, { method: "POST", credentials: "same-origin", body: JSON.stringify(b),
      headers: { "Content-Type": "application/json", "Connect-Protocol-Version": "1", "x-plux-organization": org, ...(csrf && { "x-csrf-token": csrf }) } });
    const j = await r.json();
    if (!r.ok) throw new Error(`${m}: ${j.message}`);
    return j;
  };
  csrf = (await call("IdentityService/GetCurrentUser", {})).csrfToken;
  const stepUp = async () => {
    const b = await call("IdentityService/BeginStepUp", {});
    const c = await navigator.credentials.get({ publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(JSON.parse(b.optionsJson)) });
    const b64 = (x) => btoa(String.fromCharCode(...new Uint8Array(x)));
    await call("IdentityService/CompleteStepUp", { requestId: b.requestId, credentialId: b64(c.rawId),
      clientDataJson: b64(c.response.clientDataJSON), authenticatorData: b64(c.response.authenticatorData), signature: b64(c.response.signature) });
  };
  const app = (await call("AppService/ListApps", { organizationId: org })).apps.find((a) => a.key === "apply-register");
  const envs = Object.fromEntries((await call("AppService/ListEnvironments", { appId: app.id })).environments.map((e) => [e.key, e.id]));
  const all = ["GET", "POST", "PATCH", "DELETE"];
  const rest = ["accounts", "api", "cacheFirst", "cacheNF", "cacheNet", "cacheSWR", "campaign", "donations", "eligibility",
    "featured", "listings", "me", "orgs", "partner", "products", "remote", "stats", "subs", "verify"];
  const ops = {
    payments: [{ name: "charge", methods: ["POST"], pathPrefix: "/payments", transaction: { payee: "/payee", amount: "/amount", currency: "/currency" } }],
    transfers: [{ name: "deposit", methods: ["POST"], pathPrefix: "/transfers", transaction: { payee: "/to/iban", amount: "/amount", summary: "/reference", currency: "/currency" } }],
    reliefTransfer: [{ name: "donate", methods: ["POST"], pathPrefix: "/transfers/relief", transaction: { payee: "/payee", amount: "/amount", summary: "/reference", currency: "/currency" },
      bindings: [{ key: "payeeIbanRelief", jsonPointer: "/payee", display: true, label: "Recipient" }] }],
  };
  const upstreams = [...rest.map((n) => [n, "/v1", all, "AL0"]), ["payments", "/v1", all, "AL2"], ["transfers", "/v1", all, "AL2"],
    ["reliefTransfer", "/v1", all, "AL2"], ["agenda", "", ["POST"], "AL0"], ["speakers", "", ["POST"], "AL0"], ["workshops", "", ["POST"], "AL0"]];
  await stepUp();
  let n = 0;
  for (const [env, base, priv] of [["development", "http://host.docker.internal:8090", true],
    ["staging", "https://api.apply-register.example", false], ["production", "https://api.apply-register.example", false]]) {
    for (const [name, path, methods, assurance] of upstreams) {
      if (++n % 25 === 0) await stepUp();
      await call("SecurityAdminService/CreateUpstream", { upstream: { appId: app.id, environmentId: envs[env], name, baseUrl: base + path,
        apiKey: { header: "X-AR-Gateway", secretRef: "AR_GATEWAY_KEY" }, allowPrivateAddresses: priv, allowedMethods: methods,
        requiresAssurance: assurance, operations: ops[name] ?? [] } });
      await new Promise((r) => setTimeout(r, 250));
    }
  }
  await stepUp();
  const vars = { AR_BASE: "https://api.dev.applyregister.app/v1", PROMO_EARLY_CLOSES: "1803855600",
    FEE_TABLE: '{"campus":{"bachelor":"45.00","master":"60.00","phd":"0.00"},"cityrun":{"5k":"25.00","10k":"35.00","half":"49.00","full":"69.00"},"devconf":{"standard":"199.00","student":"49.00","vip":"399.00","workshop":"79.00"},"arena":{"standing":"39.00","seated":"59.00"}}' };
  for (const [key, value] of Object.entries(vars)) await call("AppService/SetVariable", { environmentId: envs.development, key, value });
  for (const env of ["staging", "production"]) {
    await call("SecurityAdminService/SetUserIdentityProvider", { provider: { appId: app.id, environmentId: envs[env],
      issuer: "https://api.apply-register.example/auth", jwksUrl: "https://api.apply-register.example/auth/.well-known/jwks.json",
      audiences: ["apply-register"], forwardClaims: ["role", "org", "region", "segment"] } });
  }
  console.log(`done: ${n} upstreams, ${Object.keys(vars).length} variables, 2 identity providers`);
})();
```

## 7. Functions (CLI; Studio shows and tests them, deploying is the CLI's)

From `KIT/project`, with `PLUX_SDK` set as in §5 and the builder running (`COMPOSE_PROFILES=functions`):

```bash
for f in $(ls functions); do plux fn deploy "$f" --env development --alias stable; done
plux fn triggers create --function reminders.daily --alias stable --cron @daily --tz Europe/Berlin --input '{"limit": 100}'
```

Each deploy builds in the sandboxed builder (8–20 s once Cargo's cache is warm) and runs the stored tests. The
13 module hashes equal those in `db-inventory.md`.

## 8. Check, then experiment

- The app canvas: 17 plugins, all **Valid**; a plugin canvas opens with its screens; Problems shows notes only.
- **Functions** lists 13 functions with alias `stable`; **Tests** › Run on one passes.
- **Administration › Tokens:** create a CI token if you script the CLI (shown once).
- Publishing (§J1 of the sheet) works up to the release; see the open rows below first.

## 9. Open problems you may meet (build log)

| # | What you see | What to do |
|---|---|---|
| 179 | Studio can't reach the server | §2.2 above |
| 194 | Pages fail with `PLX-8040` when you click through many pages quickly | Wait a minute: every Studio user shares the backend's address and one allowance of 300 calls a minute |
| 190 | After a reload, the publish build step shows only **Freeze and build** | Press it: it re-attaches to the builds; **Freeze again** then rebuilds |
| 197 | Reports: the app bundle's size is "not reported" | Harmless |
| 178 | An exported `.plux` archive lacks the settings of §3–§7 | Recreate them as here |

`partner.submit` and `reminders.daily` call `AR_BASE` (`api.dev.applyregister.app`), which does not exist locally:
their calls fail on your machine by design, and their stored tests do not need it.
