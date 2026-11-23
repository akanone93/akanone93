// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// How the Dev app trusts dev bundles (DEV-001, DEV-003, ADR-0109). It is
/// built with no key. The phone pins its organisation's development root
/// when it is connected — the root's public key rides in the sign-in QR code
/// Studio shows after its person's step-up — and each app opens with the
/// development key that root certifies for that app and environment, checked
/// on the phone before anything is trusted. The Dev app trusts development
/// keys only and holds no production root: one binary never trusts both sets
/// (SEC-056), and the viewer owns the production set.
library;

/// The host build the Dev app reports to the runtime.
const String devHostBuild = 'plux-dev';

/// The environment whose dev bundles the Dev app applies.
const String devEnvironment = 'development';
