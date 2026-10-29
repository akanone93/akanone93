// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package renderworker is the render worker of the visual diffs (O-9,
// ADR-0097): the process in deploy/render-worker that compiles the project
// before and after a release request, renders its pages on the standard
// devices with the golden harness (ADR-0070, flutter_tester) and returns the
// screens whose image changed.
//
// The worker role of the server calls it over mutual TLS
// (renderpb.RenderWorkerService) with both projects in the request. The
// process therefore needs no database, object-store or signing credential
// (L-4): it gets its input and returns images, and the server stores them.
// The request also carries the function declarations of the environment (the
// catalogue a release build compiles against), so that a page that calls a
// function compiles; the device modules are stubs, because a render draws
// pages and never calls a function.
// Everything in a request is untrusted until the worker has bounded it: the
// archives by the limits of the registry, the compilation by the compiler's
// own limits, and Flutter by a work directory it alone writes.
package renderworker
