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

| Path | Contents |
|---|---|
| `lib/main.dart` | the controller, the link receiver, then the app |
| `lib/src/link.dart` | `ViewerLink`: parses `plux-viewer://reconstruction/<id>?server=<url>` |
| `lib/src/api.dart` | `ViewerApi`: the ConnectRPC JSON calls (sign-in, lists, `GetReconstruction`, `GetRootKeys`, bundle download) |
| `lib/src/device_sign_in.dart` | the device authorization grant (RFC 8628) |
| `lib/src/controller.dart` | the session (in memory) and the link waiting to be opened |
| `lib/src/sign_in_screen.dart`, `list_screen.dart`, `reconstruction_screen.dart` | the three screens |
| `lib/src/opener.dart` | `ReconstructionOpener`: release, keys, bundles, `Plux.initialize`, `Plux.dispose` |
| `lib/src/viewer_app.dart` | the app, and `ViewerLinks`, which hands platform links to the controller |

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
2. `GetRootKeys` returns the environment's keys; the viewer keeps those of role `targets`. (It
   reads the app and environment out of the still unverified manifest to know which to ask for,
   and nothing else.)
3. The bundles are downloaded, each stopped at the size the release declares and at the registry's
   bound for a bundle (`bundle.pluginSize`).
4. `Plux.initialize(PluxConfig(…, reconstruction: …))` verifies the manifest (canonical form,
   signature under a `targets` key, the app, environment and `audit` channel) and then each bundle
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
