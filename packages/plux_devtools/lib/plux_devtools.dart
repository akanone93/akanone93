// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// A debug overlay for host apps that use the Plux runtime (RT-060).
///
/// Wrap the app with [PluxDevtools] to see the sync status, the active
/// release and the problems the runtime reported. It renders nothing in
/// release builds. A [PluxDevSession] pairs the host's own debug build with a
/// dev session (DEV-040), so it gets the Dev app's capabilities.
library;

export 'src/dev_session.dart'
    show
        PluxDevError,
        PluxDevEvents,
        PluxDevIdle,
        PluxDevJoined,
        PluxDevPaired,
        PluxDevPairer,
        PluxDevPairingNow,
        PluxDevSession,
        PluxDevSessionState;
export 'src/devtools_overlay.dart' show PluxDevtools, describeSyncEvent;
