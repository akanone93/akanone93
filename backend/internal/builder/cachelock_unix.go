// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package builder

import (
	"os"
	"syscall"
)

// tryLock takes an exclusive lock on the file at path without waiting. held
// is false when another build holds it or the lock cannot be taken; unlock
// releases it.
func tryLock(path string) (unlock func(), held bool) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304: the builder's own cache directory.
	if err != nil {
		return nil, false
	}
	fd := int(f.Fd()) //nolint:gosec // G115: a file descriptor fits an int.
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false
	}
	return func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = f.Close()
	}, true
}
