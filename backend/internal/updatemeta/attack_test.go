// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package updatemeta_test

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/updatemeta"
)

// minted describes a chain to build: the version and lifetime of each role
// and the key that signs it.
type minted struct {
	targets, snapshot, timestamp int64
	lifetime                     updatemeta.Expiry
	signers                      online
}

// mint builds a chain under root that the verifier of newChain accepts
// unless a field of m says otherwise.
func mint(t *testing.T, root updatemeta.Root, m minted) chain {
	t.Helper()
	manifest := manifestJSON(t, m.targets, at.Add(m.lifetime.Targets))
	snap := updatemeta.NewSnapshot(m.snapshot, m.targets, at.Add(m.lifetime.Snapshot))
	snapDoc, err := updatemeta.Marshal(snap, []updatemeta.Signature{m.signers.snapshot.signature(mustCanonical(t, snap))})
	if err != nil {
		t.Fatal(err)
	}
	ts := updatemeta.NewTimestamp(m.timestamp, m.snapshot, snapDoc, at.Add(m.lifetime.Timestamp))
	tsDoc, err := updatemeta.Marshal(ts, []updatemeta.Signature{m.signers.timestamp.signature(mustCanonical(t, ts))})
	if err != nil {
		t.Fatal(err)
	}
	return chain{
		root: root, timestamp: tsDoc, snapshot: snapDoc, manifest: manifest,
		sigs: []updatemeta.Signature{m.signers.targets.signature(manifest)},
	}
}

// flipSignature returns the signature with one bit of its value changed.
func flipSignature(t *testing.T, s updatemeta.Signature) updatemeta.Signature {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(s.Sig)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0x01
	s.Sig = base64.StdEncoding.EncodeToString(raw)
	return s
}

// resign returns a metadata file whose signed part is that of doc and whose
// signatures are sigs.
func resign(t *testing.T, doc []byte, sigs []updatemeta.Signature) []byte {
	t.Helper()
	d, err := updatemeta.ParseDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	out, err := updatemeta.Marshal(d.Signed, sigs)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// signaturesOf returns the signatures of a metadata file.
func signaturesOf(t *testing.T, doc []byte) []updatemeta.Signature {
	t.Helper()
	d, err := updatemeta.ParseDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	return d.Signatures
}

// Verifies: SEC-050, QA-008.
// Tamper and rollback attacks against the three online roles. Every row
// serves a chain that an attacker could assemble and asserts the device
// refuses it with the error that names the attack.
func TestOnlineRoleAttacks_SEC_050_QA_008(t *testing.T) {
	t.Parallel()
	on := newOnline(t)
	stranger := newOnline(t)
	life := updatemeta.DefaultExpiry()
	good := mint(t, newChain(t, on, 4, 9).root, minted{targets: 4, snapshot: 3, timestamp: 9, lifetime: life, signers: on})
	root := good.root
	older := mint(t, root, minted{targets: 3, snapshot: 2, timestamp: 8, lifetime: life, signers: on})
	// The old chain, frozen: valid in itself, signed by the right keys.
	if err := older.verify(older.verifier(false, nil)); err != nil {
		t.Fatalf("the older chain must be valid on its own: %v", err)
	}

	for _, tc := range []struct {
		name   string
		serve  func(c chain) chain
		floor  updatemeta.Floor
		now    time.Duration // after the epoch of the tests
		wantIs error
	}{
		// Bad signature: one flipped bit in the role's signature.
		{name: "timestamp: bad signature", wantIs: updatemeta.ErrThreshold, serve: func(c chain) chain {
			c.timestamp = resign(t, c.timestamp, []updatemeta.Signature{flipSignature(t, signaturesOf(t, c.timestamp)[0])})
			return c
		}},
		{name: "snapshot: bad signature", wantIs: updatemeta.ErrMismatch, serve: func(c chain) chain {
			// Re-signing changes the bytes the timestamp hashes: the
			// attacker cannot touch a signature without the hash link
			// breaking too.
			c.snapshot = resign(t, c.snapshot, []updatemeta.Signature{flipSignature(t, signaturesOf(t, c.snapshot)[0])})
			return c
		}},
		{name: "targets: bad signature", wantIs: updatemeta.ErrThreshold, serve: func(c chain) chain {
			c.sigs = []updatemeta.Signature{flipSignature(t, c.sigs[0])}
			return c
		}},
		// Below threshold: no signature at all.
		{name: "timestamp: unsigned", wantIs: updatemeta.ErrThreshold, serve: func(c chain) chain {
			c.timestamp = resign(t, c.timestamp, nil)
			return c
		}},
		{name: "targets: unsigned", wantIs: updatemeta.ErrThreshold, serve: func(c chain) chain {
			c.sigs = nil
			return c
		}},
		// Signed by a key the root does not list for the role.
		{name: "timestamp: signed by a stranger", wantIs: updatemeta.ErrThreshold, serve: func(c chain) chain {
			return mint(t, root, minted{targets: 4, snapshot: 3, timestamp: 9, lifetime: life, signers: online{on.targets, on.snapshot, stranger.timestamp}})
		}},
		{name: "snapshot: signed by a stranger", wantIs: updatemeta.ErrThreshold, serve: func(c chain) chain {
			return mint(t, root, minted{targets: 4, snapshot: 3, timestamp: 9, lifetime: life, signers: online{on.targets, stranger.snapshot, on.timestamp}})
		}},
		{name: "targets: signed by a stranger", wantIs: updatemeta.ErrThreshold, serve: func(c chain) chain {
			return mint(t, root, minted{targets: 4, snapshot: 3, timestamp: 9, lifetime: life, signers: online{stranger.targets, on.snapshot, on.timestamp}})
		}},
		{name: "role keys swapped: timestamp key signs the snapshot", wantIs: updatemeta.ErrThreshold, serve: func(c chain) chain {
			return mint(t, root, minted{targets: 4, snapshot: 3, timestamp: 9, lifetime: life, signers: online{on.targets, on.timestamp, on.timestamp}})
		}},
		// Expired roles.
		{name: "timestamp: expired", wantIs: updatemeta.ErrExpired, now: life.Timestamp, serve: func(c chain) chain { return c }},
		{name: "snapshot: expired, timestamp fresh", wantIs: updatemeta.ErrExpired, now: time.Hour, serve: func(c chain) chain {
			l := life
			l.Snapshot = time.Minute
			return mint(t, root, minted{targets: 4, snapshot: 3, timestamp: 9, lifetime: l, signers: on})
		}},
		{name: "targets: expired, others fresh", wantIs: updatemeta.ErrExpired, now: time.Hour, serve: func(c chain) chain {
			l := life
			l.Targets = time.Minute
			return mint(t, root, minted{targets: 4, snapshot: 3, timestamp: 9, lifetime: l, signers: on})
		}},
		// Version rollback against the versions the device already trusts.
		{name: "timestamp: version rollback", wantIs: updatemeta.ErrRollback, floor: updatemeta.Floor{Timestamp: 10}, serve: func(c chain) chain { return c }},
		{name: "snapshot: version rollback", wantIs: updatemeta.ErrRollback, floor: updatemeta.Floor{Snapshot: 4}, serve: func(c chain) chain { return c }},
		{name: "targets: version rollback", wantIs: updatemeta.ErrRollback, floor: updatemeta.Floor{Targets: 5}, serve: func(c chain) chain { return c }},
		{name: "the whole older chain replayed", wantIs: updatemeta.ErrRollback, floor: updatemeta.Floor{Timestamp: 9, Snapshot: 3, Targets: 4}, serve: func(c chain) chain { return older }},
		// Freeze: the attacker keeps serving a once-valid timestamp after
		// its 24 hours, whatever else is fresh.
		{name: "freeze: an old timestamp past its lifetime", wantIs: updatemeta.ErrExpired, now: life.Timestamp + 5*time.Minute, serve: func(c chain) chain { return older }},
		// Mix and match: files from different chains that each verify.
		{name: "mix: the older snapshot under the newer timestamp", wantIs: updatemeta.ErrMismatch, serve: func(c chain) chain {
			c.snapshot = older.snapshot
			return c
		}},
		{name: "mix: the newer snapshot under the older timestamp", wantIs: updatemeta.ErrMismatch, serve: func(c chain) chain {
			c.timestamp = older.timestamp
			return c
		}},
		{name: "mix: the older manifest under the newer snapshot", wantIs: updatemeta.ErrMismatch, serve: func(c chain) chain {
			c.manifest, c.sigs = older.manifest, older.sigs
			return c
		}},
		{name: "mix: the newer manifest under the older snapshot and timestamp", wantIs: updatemeta.ErrMismatch, serve: func(c chain) chain {
			o := older
			o.manifest, o.sigs = c.manifest, c.sigs
			return o
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tc.serve(good)
			v := c.verifier(false, nil)
			v.Floor = tc.floor
			v.Now = at.Add(time.Hour + tc.now)
			if err := c.verify(v); !errors.Is(err, tc.wantIs) {
				t.Errorf("got %v, want %v", err, tc.wantIs)
			}
		})
	}
}

// Verifies: SEC-050, SEC-051, QA-008.
// Tamper and rollback attacks against the root role, offline and trusted
// by pin: a flipped signature, a missing signature, an expired root and a
// replayed older root.
func TestRootRoleAttacks_SEC_051_QA_008(t *testing.T) {
	t.Parallel()
	on := newOnline(t)
	keys := offline(t)
	year := time.Hour * 24 * 365
	v1 := rootSigned(1, at.Add(year), keys, on)
	v2 := rootSigned(2, at.Add(year), keys, on)
	two := func(t *testing.T, r updatemeta.Root) []updatemeta.Signature {
		return signaturesOf(t, signRoot(t, r, keys[0], keys[2]))
	}

	t.Run("first root: a flipped signature", func(t *testing.T) {
		t.Parallel()
		sigs := two(t, v1)
		sigs[0] = flipSignature(t, sigs[0])
		if _, _, err := updatemeta.FirstRoot(resign(t, signRoot(t, v1, keys[0], keys[2]), sigs)); !errors.Is(err, updatemeta.ErrThreshold) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("rotation: a flipped signature of the previous keys", func(t *testing.T) {
		t.Parallel()
		sigs := two(t, v2)
		sigs[1] = flipSignature(t, sigs[1])
		if _, _, err := updatemeta.NextRoot(v1, resign(t, signRoot(t, v2, keys[0], keys[2]), sigs)); !errors.Is(err, updatemeta.ErrThreshold) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("rotation: a signed part altered after signing", func(t *testing.T) {
		t.Parallel()
		doc := signRoot(t, v2, keys[0], keys[2])
		altered := v2
		altered.Expires = updatemeta.FormatTime(at.Add(10 * year))
		forged := resign(t, signRoot(t, altered, keys[0]), signaturesOf(t, doc))
		if _, _, err := updatemeta.NextRoot(v1, forged); !errors.Is(err, updatemeta.ErrThreshold) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("rotation: unsigned", func(t *testing.T) {
		t.Parallel()
		if _, _, err := updatemeta.NextRoot(v1, signRoot(t, v2)); !errors.Is(err, updatemeta.ErrThreshold) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("chain: an expired newest root", func(t *testing.T) {
		t.Parallel()
		d2 := signRoot(t, v2, keys[0], keys[2])
		if _, err := updatemeta.RootChain(v1, [][]byte{d2}, at.Add(year)); !errors.Is(err, updatemeta.ErrExpired) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("chain: an older root replayed after the newer one is trusted", func(t *testing.T) {
		t.Parallel()
		if _, err := updatemeta.RootChain(v2, [][]byte{signRoot(t, v2, keys[0], keys[2])}, at); !errors.Is(err, updatemeta.ErrRollback) {
			t.Errorf("version 2 again over trusted 2: %v", err)
		}
		if _, err := updatemeta.RootChain(v2, [][]byte{signRoot(t, v1, keys[0], keys[2])}, at); !errors.Is(err, updatemeta.ErrRollback) {
			t.Errorf("version 1 over trusted 2: %v", err)
		}
	})
}
