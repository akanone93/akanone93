// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package updatemeta_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/updatemeta"
)

// controlRig is a root that delegates the control role to a key held by
// the file signing backend, as the worker signs.
type controlRig struct {
	backend *signing.File
	root    updatemeta.Root
	scope   updatemeta.ControlScope
}

func newControlRig(t testing.TB, delegate bool) controlRig {
	t.Helper()
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pub, _, err := backend.PublicKey(context.Background(), "control-env")
	if err != nil {
		t.Fatal(err)
	}
	off := offline(t)
	on := newOnline(t)
	spec := updatemeta.RootSpec{
		Version: 1, Expires: at.Add(updatemeta.DefaultExpiry().Root),
		Root:      updatemeta.RoleSpec{Threshold: 2, Keys: []updatemeta.Key{off[0].listed(), off[1].listed(), off[2].listed()}},
		Targets:   updatemeta.RoleSpec{Threshold: 1, Keys: []updatemeta.Key{on.targets.listed()}},
		Snapshot:  updatemeta.RoleSpec{Threshold: 1, Keys: []updatemeta.Key{on.snapshot.listed()}},
		Timestamp: updatemeta.RoleSpec{Threshold: 1, Keys: []updatemeta.Key{on.timestamp.listed()}},
	}
	if delegate {
		spec.Control = &updatemeta.RoleSpec{Threshold: 1, Keys: []updatemeta.Key{{Alg: updatemeta.AlgEd25519, Public: base64.StdEncoding.EncodeToString(pub)}}}
	}
	root, err := updatemeta.NewRoot(spec)
	if err != nil {
		t.Fatal(err)
	}
	return controlRig{backend: backend, root: root, scope: updatemeta.ControlScope{App: "app-1", Environment: "production", Channel: "production"}}
}

func (r controlRig) document(version uint64) controldoc.Document {
	return controldoc.Document{
		Version: version, AppID: r.scope.App, Environment: r.scope.Environment, Channel: r.scope.Channel,
		IssuedAt: at, ExpiresAt: at.Add(24 * time.Hour),
		KillSwitches:    []controldoc.KillSwitch{{Scope: controldoc.ScopePlugin, Target: "checkout"}},
		MandatoryUpdate: &controldoc.MandatoryUpdate{ReleaseID: "rel-4", ReleaseSequence: 4},
	}
}

// sign encodes and signs a document under the control key.
func (r controlRig) sign(t testing.TB, d controldoc.Document) []byte {
	t.Helper()
	encoded, err := controldoc.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	file, _, err := updatemeta.Sign(context.Background(), r.backend, "control-env", updatemeta.NewControl(d, encoded))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func (r controlRig) verifier(floor int64, now time.Time) updatemeta.Verifier {
	return updatemeta.Verifier{Root: r.root, Floor: updatemeta.Floor{Control: floor}, Now: now}
}

// TestControlSignedAndVerifiedFromRoot_SYN_060 signs a control document
// with the online control key and verifies it from the root that delegates
// to that key, with the document read only afterwards.
//
// Verifies: SYN-060, REL-060, REL-070, SEC-052.
func TestControlSignedAndVerifiedFromRoot_SYN_060(t *testing.T) {
	t.Parallel()
	rig := newControlRig(t, true)
	file := rig.sign(t, rig.document(3))
	got, err := rig.verifier(2, at.Add(time.Hour)).Control(file, rig.scope)
	if err != nil {
		t.Fatalf("Control: %v", err)
	}
	if got.Version != 3 || len(got.KillSwitches) != 1 || got.MandatoryUpdate == nil || got.MandatoryUpdate.ReleaseSequence != 4 {
		t.Fatalf("document = %+v", got)
	}
	again := rig.sign(t, rig.document(3))
	if !bytes.Equal(file, again) {
		t.Fatalf("the same document signed twice gave different files")
	}
}

// TestControlRefusals_SYN_060 checks each way a device refuses a control
// document, with the error that maps to its code.
//
// Verifies: SYN-060, SEC-052.
func TestControlRefusals_SYN_060(t *testing.T) {
	t.Parallel()
	rig := newControlRig(t, true)
	file := rig.sign(t, rig.document(5))
	now := at.Add(time.Hour)

	flipped := bytes.Replace(file, []byte(`"version":5`), []byte(`"version":6`), 1)
	if bytes.Equal(flipped, file) {
		t.Fatal("the test did not change the file")
	}
	otherKey := newControlRig(t, true)
	cases := []struct {
		name  string
		file  []byte
		check updatemeta.Verifier
		scope updatemeta.ControlScope
		want  error
	}{
		{"tampered header", flipped, rig.verifier(1, now), rig.scope, updatemeta.ErrThreshold},
		{"signed by another key", otherKey.sign(t, otherKey.document(5)), rig.verifier(1, now), rig.scope, updatemeta.ErrThreshold},
		{"expired", file, rig.verifier(1, at.Add(25*time.Hour)), rig.scope, updatemeta.ErrExpired},
		{"older version", file, rig.verifier(6, now), rig.scope, updatemeta.ErrRollback},
		{"another channel", file, rig.verifier(1, now), updatemeta.ControlScope{App: "app-1", Environment: "production", Channel: "beta"}, updatemeta.ErrMismatch},
		{"not a document", []byte(`{"signed":{}}`), rig.verifier(1, now), rig.scope, updatemeta.ErrFormat},
	}
	for _, tc := range cases {
		if _, err := tc.check.Control(tc.file, tc.scope); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := rig.verifier(5, now).Control(file, rig.scope); err != nil {
		t.Errorf("the same version again must be accepted (an unchanged document): %v", err)
	}
}

// TestControlRootWithoutDelegation_SYN_060 checks that a root that
// delegates no control role accepts no control document, whoever signed.
//
// Verifies: SYN-060.
func TestControlRootWithoutDelegation_SYN_060(t *testing.T) {
	t.Parallel()
	rig := newControlRig(t, false)
	if _, ok := rig.root.Roles.Named(updatemeta.RoleControl); ok {
		t.Fatal("the root delegates a control role")
	}
	file := rig.sign(t, rig.document(1))
	_, err := rig.verifier(0, at.Add(time.Hour)).Control(file, rig.scope)
	if !errors.Is(err, updatemeta.ErrThreshold) || !strings.Contains(err.Error(), "delegates no control role") {
		t.Fatalf("err = %v", err)
	}
}

// TestControlDocumentMustMatchItsHeader_SYN_060 checks that a document
// that says something other than the signed header is refused even when
// the key is right.
//
// Verifies: SYN-060, SEC-052.
func TestControlDocumentMustMatchItsHeader_SYN_060(t *testing.T) {
	t.Parallel()
	rig := newControlRig(t, true)
	inner := rig.document(2)
	encoded, err := controldoc.Encode(inner)
	if err != nil {
		t.Fatal(err)
	}
	header := inner
	header.Version = 9
	file, _, err := updatemeta.Sign(context.Background(), rig.backend, "control-env", updatemeta.NewControl(header, encoded))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rig.verifier(0, at.Add(time.Hour)).Control(file, rig.scope); !errors.Is(err, updatemeta.ErrMismatch) {
		t.Fatalf("err = %v, want ErrMismatch", err)
	}
}

// TestControlMatchesTheSchemas_SYN_060 checks that the root with a control
// role and a control document validate against their JSON Schemas.
//
// Verifies: SYN-060, SEC-122.
func TestControlMatchesTheSchemas_SYN_060(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", "..", "schema", "update")
	commonRaw, err := os.ReadFile(filepath.Join(dir, "common.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	common, err := jsonschema.UnmarshalJSON(bytes.NewReader(commonRaw))
	if err != nil {
		t.Fatal(err)
	}
	rig := newControlRig(t, true)
	docs := map[string][]byte{
		"root":    signRoot(t, rig.root, offline(t)[0]),
		"control": rig.sign(t, rig.document(1)),
	}
	for kind, doc := range docs {
		abs, err := filepath.Abs(filepath.Join(dir, kind+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		comp := jsonschema.NewCompiler()
		comp.AssertFormat()
		if err := comp.AddResource("https://plux.dev/schema/v1/update/common.schema.json", common); err != nil {
			t.Fatal(err)
		}
		s, err := comp.Compile(abs)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Validate(inst); err != nil {
			t.Errorf("%s does not match its schema: %v\n%s", kind, err, doc)
		}
	}
}
