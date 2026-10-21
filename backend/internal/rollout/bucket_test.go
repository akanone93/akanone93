// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"fmt"
	"testing"
)

// Verifies: REL-050.
func TestBucket_REL_050_StableAndInRange(t *testing.T) {
	salt := []byte("salt-one")
	first := Bucket(salt, Subject("", "device-1"))
	for range 5 {
		if got := Bucket(salt, Subject("", "device-1")); got != first {
			t.Fatalf("bucket changed from %d to %d", first, got)
		}
	}
	for i := range 2000 {
		if b := Bucket(salt, Subject("", fmt.Sprint("d", i))); b < 0 || b >= Buckets {
			t.Fatalf("bucket %d is outside [0, %d)", b, Buckets)
		}
	}
}

// A fixed vector, computed independently with Python's hashlib, pins the function: a change of the hash or of the byte
// order would move every device in every running rollout.
//
// Verifies: REL-050.
func TestBucket_REL_050_Vector(t *testing.T) {
	if got, want := Bucket([]byte("salt"), "device:abc"), 2792; got != want {
		t.Errorf("Bucket = %d, want %d", got, want)
	}
}

// Verifies: REL-050.
func TestBucket_REL_050_MonotonicAcrossShares(t *testing.T) {
	salt := []byte("monotonic")
	shares := []int{100, 1000, 2500, 5000, 9999, 10000}
	for i := range 3000 {
		b := Bucket(salt, Subject("", fmt.Sprint("device-", i)))
		in := false
		for _, share := range shares {
			now := Admitted(b, share)
			if in && !now {
				t.Fatalf("device %d was in at a smaller share and out at %d", i, share)
			}
			in = now
		}
		if !in {
			t.Fatalf("device %d is out at 10000 basis points", i)
		}
	}
}

// Verifies: REL-050.
func TestBucket_REL_050_ShareApproximatesPercentage(t *testing.T) {
	salt := []byte("distribution")
	const n = 20000
	in := 0
	for i := range n {
		if Admitted(Bucket(salt, Subject("", fmt.Sprint("device-", i))), 1000) {
			in++
		}
	}
	if in < n*8/100 || in > n*12/100 {
		t.Errorf("%d of %d devices are in at 10%%", in, n)
	}
}

// Verifies: REL-050.
func TestBucket_REL_050_SaltsAreIndependent(t *testing.T) {
	differ := 0
	for i := range 200 {
		s := Subject("", fmt.Sprint("device-", i))
		if Bucket([]byte("a"), s) != Bucket([]byte("b"), s) {
			differ++
		}
	}
	if differ < 150 {
		t.Errorf("only %d of 200 devices moved with another salt", differ)
	}
}

// The user, not the device, places a person: two devices of one user share a
// bucket, and a user's hash never collides with a device identifier.
//
// Verifies: REL-050.
func TestSubject_REL_050_UserOverDevice(t *testing.T) {
	if Subject("u", "d1") != Subject("u", "d2") {
		t.Error("two devices of one user have different subjects")
	}
	if Subject("", "x") == Subject("x", "") {
		t.Error("a device identifier and a user hash share a namespace")
	}
	if Subject("", "d") != "device:d" {
		t.Errorf("subject of a device = %q", Subject("", "d"))
	}
}
