// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package builder

// tryLock never holds the lock where the standard library has no file lock:
// builds there run from scratch.
func tryLock(string) (unlock func(), held bool) { return nil, false }
