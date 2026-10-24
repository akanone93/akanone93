<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Plux Viewer

The viewer host for reconstructions (`SEC-142`, [ADR-0082](../../docs/adr/0082-audit-export-reconstruction-and-egress.md)):
an auditor signs in, lists the reconstruction releases of their apps, and opens one in the
runtime's **reconstruction mode**, which shows the page a user saw at a past moment, as the
device presented it, under a banner that names the moment. P10 grows it into the Dev app.

A reconstruction release is made by the server (`AuditService/Reconstruct`): the bundles a
device held, exactly as shipped, with a signed manifest of the moment. The server renders
nothing. The viewer downloads the release and hands it to `Plux.initialize` through
`PluxConfig.reconstruction`; the runtime verifies it before it loads anything (`SEC-052`).

The sign-in, API client, link, controller, root-key reader and release opener are in
[`packages/plux_host_kit`](../../packages/plux_host_kit/README.md), shared with the Dev app
(P10 D-6). The viewer keeps the screens and its production root keys.

| Path | Contents |
|---|---|
| `lib/main.dart` | the controller, the link receiver, then the app |
| `lib/src/production_roots.dart` | the production root keys the viewer is built with (`PLUX_VIEWER_ROOT_KEYS`), passed to the kit |
| `lib/src/sign_in_screen.dart`, `list_screen.dart`, `reconstruction_screen.dart` | the three screens |
| `lib/src/viewer_app.dart` | the app |
| kit: `api.dart` | `ViewerApi`: the ConnectRPC JSON calls (sign-in, lists, `GetReconstruction`, `GetRootKeys` for the signed root chain, bundle download) |
| kit: `link.dart`, `links.dart` | `ViewerLink`: parses `plux-viewer://reconstruction/<id>?server=<url>`; `ViewerLinks` hands platform links to the controller |
| kit: `device_sign_in.dart`, `controller.dart` | the device authorization grant (RFC 8628); the session (in memory) and the link waiting to be opened |
| kit: `opener.dart`, `root_keys.dart` | `ReconstructionOpener`: release, root chain, bundles, `Plux.initialize`, `Plux.dispose`, with the root keys the host passes; `parseRootKeys` |

## The trust anchor: root keys pinned at build time

A reconstruction is only as trustworthy as the keys it is verified with. The viewer does not take
them from the server it talks to (a server, or anyone in the TLS path, could then sign a forged
reconstruction and list the key that verifies it). It is built with the **root keys** of the
installation, as a host app is (`SEC-051`): the contents of the installation's `keys.json`, as
`plux pull` writes it, a JSON list of `{keyId, algorithm, role, publicKey}` with the public key in
hex.

```bash
flutter build apk \
  --dart-define=PLUX_VIEWER_ROOT_KEYS="$(cat path/to/keys.json)"
```

| Define | Meaning |
|---|---|
| `PLUX_VIEWER_ROOT_KEYS` | the installation's `keys.json`; without it the viewer opens nothing and says why |

The runtime makes a root of these keys and walks the signed root files `GetRootKeys` returns
after it, each signed by the threshold of its predecessor's keys and its own, to the `targets`
keys it verifies the manifest with: the walk of a normal sync, not a second implementation. A
reconstruction signed by a key that chain does not list is refused (`PLX-3001`), as is a chain the
pinned keys did not sign or that has expired (`PLX-6022`; `PLX-3003` for a chain that skips a version). Keys that cannot be read stop the app at
start, so that a mistake in a build does not look like a viewer that opens nothing.

## Signing in

The default is the **device authorization grant**, the flow `plux login` uses
([ADR-0028](../../docs/adr/0028-cli-credential-storage.md), `CLI-002`): the viewer asks the server
for a code, shows it with an address, and the auditor approves it in a browser, on any device,
where they are already signed in with whatever the organisation requires (SSO, a second factor).
The viewer never sees the auditor's password. It asks for the two permissions it uses and no more:
`reconstruction.view` and `app.read`. The alternative is a pasted **access token** the auditor
created for themselves (`IdentityService/CreateAccessToken`), for a device where approving in a
browser is awkward.

The token is kept **in memory only**. It is not written to disk, so a lost or shared device holds
no credential; the cost is signing in again after the app is closed. It is sent to the server it
was issued by and to nothing else: a link naming another server ends the session, and the auditor
signs in to that server instead. Bundles are downloaded without it.

## The link

The server gives each reconstruction an `openUri`, `plux-viewer://reconstruction/<id>?server=<url>`.
Android (`BROWSABLE` intent filter) and iOS (`CFBundleURLTypes`) hand it to the app; `ViewerLinks`
claims every link of the scheme, so the framework's navigator never tries to open it as a route. A
link is untrusted input: the ID is one path segment of safe characters, and the server is an
`https` origin without credentials, query or fragment (`http` is accepted for the development
machine only: `localhost`, `127.0.0.1`, `10.0.2.2`). A link that fails these is refused with the
reason. When the auditor is not signed in to the link's server, the link waits and opens as soon
as they are.

## Opening a reconstruction

1. `GetReconstruction` returns the canonical manifest, its signatures and a download URL per
   bundle. The server audits the view.
2. `GetRootKeys` returns the environment's signed root chain after the pinned root; the viewer keeps
   the root files and nothing else it lists. (It reads the app and environment out of the still
   unverified manifest to know which chain to ask for, and nothing else.)
3. The bundles are downloaded, each stopped at the size the release declares and at the registry's
   bound for a bundle (`bundle.pluginSize`).
4. `Plux.initialize(PluxConfig(rootKeys: <pinned>, …, reconstruction: …))` walks the chain from the
   pinned keys to the `targets` keys, then verifies the manifest (canonical form,
   signature under one of them, the app, environment and `audit` channel) and then each bundle
   against the hash the manifest names, before anything is stored or parsed.
5. `PluxReconstructionView` shows the recorded route in the recorded locale and theme, at the
   recorded text scale and device class, under the banner. Leaving the screen stops the runtime,
   which deletes the reconstruction's store.

Reconstruction mode syncs nothing, records no telemetry, has no data layer, makes no request to a
host other than the server, and fails every action that reaches beyond the page with `PLX-4010`
(data sources, functions, the host, the device, links out of the app). The viewer does not pin the
server's certificate: it opens releases of servers whose keys it cannot know in advance, and the
runtime makes no request of its own in this mode.

## What is not shown

The manifest records the route but not its parameters, and no application state or data: a page
that needs parameters shows its parameter error, and a page's data sources are not reached, so it
shows what it shows without data. Assets (images, fonts) are not part of the release yet.

## Tests

```bash
flutter test          # link parsing, the API, sign-in, the list screen, the opener, link handling
```

No test here needs a device or an emulator. The runtime's side — verification, the banner, the
recorded presentation, the refusal of side effects — is tested in
`packages/plux_flutter/test/reconstruction/`.
