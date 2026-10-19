// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package serverapp is the plux-server command: configuration, construction,
// run, shutdown and the operator commands (spec §11). It lives outside
// cmd/ so that two binaries share it: cmd/plux-server, the open-source core,
// and ee/cmd/plux-server-ee, which registers the enterprise licence verifier
// in Options (GOV-033). The enterprise module is a separate Go module and
// can import only packages outside internal/, which is why this package and
// package edition are public.
package serverapp
