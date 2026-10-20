// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package reconstruct answers "what did the user see" (SEC-142, ADR-0082,
// plan p9 section 5.9, Q14).
//
// The server has no renderer. Given an app, a device or a pseudonymous user
// and a moment, it resolves the state the device was in, packages it as a
// signed, read-only reconstruction release and leaves the rendering to the
// runtime: an auditor opens the release in the viewer host, which runs the
// runtime in reconstruction mode.
//
// # What is resolved
//
// The device's history (the device package's device_installs rows, one per
// change of what it reported) gives the release it ran, the bundles it held,
// the channel it followed, and the locale, device class, theme and text scale
// it last reported. The release's bundles are the ones the device reported,
// or, when it reported none, the release's own. The control document in force
// is the newest signed version of the channel issued by the moment; the
// experiment assignments in force are the empty list, since no experiment
// engine records assignments yet. The route is the page the device last
// reported viewing in that release; parameters are not recorded and are not
// part of the manifest.
//
// # Retention
//
// A moment older than the limit reconstruction.retentionDays, a moment with
// no history at or before it, and a moment whose release bundles are no
// longer stored are all PLX-8502. The maintenance sweep deletes history and
// reconstruction releases past the limit (Service.Purge).
//
// # The reconstruction release
//
// A release is a row of the reconstructions table on the environment's
// "audit" channel, which is created on first use and never has a manifest of
// its own: the device manifest endpoint refuses the channel (release.GetManifest)
// and tenancy refuses to create one by that key. Its manifest is a JSON
// document (schema/json/reconstruction.schema.json) of type "reconstruction",
// specVersion 1, naming the release, the moment, the subject, the release
// sequence and channel, every bundle as shipped (key, version, "sha256:<hex>"
// hash, size), the control document, the experiments, the presentation and the
// route. It holds no personal data beyond the device identifier and the
// pseudonymous user reference the history retained.
//
// The manifest is canonicalised (RFC 8785) and signed with the environment's
// manifest key through release.Service.SignDocument, the path manifests are
// signed by. Only the worker role holds a signer, so the API role records the
// release as pending and enqueues a SignJob; GetReconstruction returns the
// signed bytes, the signatures and a download URL per bundle once the job has
// run. The viewer verifies the signatures with the environment's root keys
// (GetRootKeys), then each bundle against its hash, before it parses
// anything (SEC-052).
//
// # Invariants
//
//   - Creating and viewing a reconstruction both need the permission
//     reconstruction.view on the app and both write an audit entry; the
//     entry names the reconstruction and the moment, never the subject.
//   - A reconstruction release is never served to a production device.
//   - The limit comes from the registry (LIM-001); the clock is injected.
package reconstruct
