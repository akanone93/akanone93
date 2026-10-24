<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# plux_host_kit

What the Plux host apps share (P10 D-6): the viewer (`apps/plux_viewer`) today, the Dev app next. Unpublished.

| API | Purpose |
|---|---|
| `ViewerApi`, `DeviceSignIn` | the ConnectRPC JSON calls, and sign-in by the device authorization grant (RFC 8628) or a pasted access token |
| `ViewerController`, `ViewerSession` | the in-memory session and the link waiting to be opened; a token is sent only to the server that issued it |
| `ViewerLink`, `ViewerLinks` | the link grammar (untrusted input) and the receiver of platform links |
| `parseRootKeys` | reads a `keys.json` for the keys a host pins |
| `ReconstructionOpener` | opens a reconstruction release in the runtime's reconstruction mode |

**The kit never chooses a trust anchor.** `ReconstructionOpener` takes the root keys as a constructor parameter: the viewer passes its production roots, the Dev app will pass development roots, and one binary never trusts both (`SEC-051`, `SEC-052`).

```bash
flutter test
```
