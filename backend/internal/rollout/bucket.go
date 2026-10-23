// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"crypto/sha256"
	"encoding/binary"
)

// Buckets is the number of buckets a rollout divides devices into; a share
// is a number of them, so it is in basis points (REL-050).
const Buckets = 10000

// Bucket places a subject in one of Buckets buckets: the first eight bytes
// of sha256(salt || subject) modulo Buckets. The salt is the rollout's own,
// so a device that is early in one rollout is not early in the next.
func Bucket(salt []byte, subject string) int {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(subject))
	sum := h.Sum(nil)
	return int(binary.BigEndian.Uint64(sum[:8]) % Buckets)
}

// Subject is what a device is bucketed by: the hashed user identifier when
// the host app provided one, so that a person keeps their place across
// devices, and the device identifier otherwise. The prefix keeps the two
// namespaces apart.
func Subject(userHash, deviceID string) string {
	if userHash != "" {
		return "user:" + userHash
	}
	return "device:" + deviceID
}

// Admitted reports whether a bucket lies inside a share of basisPoints. It is
// monotonic: whatever is admitted at a share is admitted at a larger one.
func Admitted(bucket, basisPoints int) bool { return bucket < basisPoints }
