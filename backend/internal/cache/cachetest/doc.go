// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package cachetest runs a throw-away Valkey (or Redis) server for the
// tests of everything that talks to the shared cache over the wire. It
// belongs to tests only: it starts the server binary found on PATH, on a
// free loopback port, and stops it with the test. A test can stop and
// restart it to simulate an outage.
package cachetest
