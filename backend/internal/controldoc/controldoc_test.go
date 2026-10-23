// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package controldoc_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/controldoc/controlfbs"
)

var issued = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func sample() controldoc.Document {
	return controldoc.Document{
		Version: 7, AppID: "0190a000-0000-7000-8000-000000000001", Environment: "production", Channel: "production",
		IssuedAt: issued, ExpiresAt: issued.Add(24 * time.Hour),
		KillSwitches: []controldoc.KillSwitch{
			{Scope: controldoc.ScopeFunction, Target: "orders.total"},
			{Scope: controldoc.ScopePlugin, Target: "checkout"},
			{Scope: controldoc.ScopePage, Target: "0190a000-0000-7000-8000-0000000000aa"},
			{Scope: controldoc.ScopePlugin, Target: "browse"},
		},
		MandatoryUpdate: &controldoc.MandatoryUpdate{ReleaseID: "0190a000-0000-7000-8000-0000000000bb", ReleaseSequence: 12, MessageKey: "update.security"},
		ForcedRollback:  &controldoc.ForcedRollback{ReleaseID: "0190a000-0000-7000-8000-0000000000cc", ReleaseSequence: 9},
	}
}

// TestRoundTrip_SYN_060 round-trips a document and checks the kill
// switches come back sorted by scope and target.
//
// Verifies: SYN-060, REL-060, REL-070.
func TestRoundTrip_SYN_060(t *testing.T) {
	t.Parallel()
	buf, err := controldoc.Encode(sample())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !controlfbs.ControlDocumentBufferHasIdentifier(buf) {
		t.Fatalf("the buffer has no PLXC identifier")
	}
	got, err := controldoc.Decode(buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := sample()
	want.KillSwitches = []controldoc.KillSwitch{
		{Scope: controldoc.ScopePlugin, Target: "browse"},
		{Scope: controldoc.ScopePlugin, Target: "checkout"},
		{Scope: controldoc.ScopePage, Target: "0190a000-0000-7000-8000-0000000000aa"},
		{Scope: controldoc.ScopeFunction, Target: "orders.total"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, want)
	}
}

// TestEncodeDeterministic_SYN_060 checks that the order the kill switches
// are given in does not reach the bytes, and that the reserved vectors are
// present and empty.
//
// Verifies: SYN-060, CMP-002.
func TestEncodeDeterministic_SYN_060(t *testing.T) {
	t.Parallel()
	a := sample()
	b := sample()
	b.KillSwitches = []controldoc.KillSwitch{a.KillSwitches[3], a.KillSwitches[2], a.KillSwitches[1], a.KillSwitches[0]}
	first, err := controldoc.Encode(a)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for range 3 {
		again, err := controldoc.Encode(b)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if !bytes.Equal(first, again) {
			t.Fatalf("the same document gave different bytes")
		}
	}
	root := controlfbs.GetRootAsControlDocument(first, 0)
	if root.FlagsLength() != 0 || root.ExperimentsLength() != 0 {
		t.Fatalf("the reserved vectors hold %d flags and %d experiments", root.FlagsLength(), root.ExperimentsLength())
	}
}

// TestValidate_SYN_060 refuses documents that break the schema's rules.
//
// Verifies: SYN-060, REL-060.
func TestValidate_SYN_060(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*controldoc.Document){
		"version zero":       func(d *controldoc.Document) { d.Version = 0 },
		"no app":             func(d *controldoc.Document) { d.AppID = "" },
		"expiry not after":   func(d *controldoc.Document) { d.ExpiresAt = d.IssuedAt },
		"unknown scope":      func(d *controldoc.Document) { d.KillSwitches[0].Scope = "app" },
		"empty target":       func(d *controldoc.Document) { d.KillSwitches[0].Target = "" },
		"target with space":  func(d *controldoc.Document) { d.KillSwitches[0].Target = "a b" },
		"overlong target":    func(d *controldoc.Document) { d.KillSwitches[0].Target = strings.Repeat("a", controldoc.MaxText+1) },
		"duplicate switch":   func(d *controldoc.Document) { d.KillSwitches[1] = d.KillSwitches[0] },
		"mandatory no seq":   func(d *controldoc.Document) { d.MandatoryUpdate.ReleaseSequence = 0 },
		"mandatory bad key":  func(d *controldoc.Document) { d.MandatoryUpdate.MessageKey = "has space" },
		"rollback no id":     func(d *controldoc.Document) { d.ForcedRollback.ReleaseID = "" },
		"rollback no number": func(d *controldoc.Document) { d.ForcedRollback.ReleaseSequence = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := sample()
			mutate(&d)
			if _, err := controldoc.Encode(d); !errors.Is(err, controldoc.ErrInvalid) {
				t.Fatalf("Encode = %v, want ErrInvalid", err)
			}
		})
	}
}

// TestDecodeMalformed_SYN_060 refuses bytes that are not a control
// document, without panicking.
//
// Verifies: SYN-060, SEC-052.
func TestDecodeMalformed_SYN_060(t *testing.T) {
	t.Parallel()
	buf, err := controldoc.Encode(sample())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	cases := map[string][]byte{
		"empty":         nil,
		"short":         {1, 2, 3},
		"no identifier": append([]byte{8, 0, 0, 0}, []byte("NOPE0000000000000000")...),
	}
	// The last bytes of a buffer can be padding nothing refers to.
	for cut := 8; cut < len(buf)-16; cut += 17 {
		cases["truncated"+string(rune('a'+cut%26))] = buf[:cut]
	}
	for name, in := range cases {
		if _, err := controldoc.Decode(in); err == nil {
			t.Errorf("%s: Decode accepted %d bytes", name, len(in))
		}
	}
}

// FuzzDecode_SYN_060 checks that Decode never panics on arbitrary bytes.
//
// Verifies: SYN-060, SEC-052.
func FuzzDecode_SYN_060(f *testing.F) {
	buf, err := controldoc.Encode(sample())
	if err != nil {
		f.Fatalf("Encode: %v", err)
	}
	f.Add(buf)
	f.Fuzz(func(_ *testing.T, in []byte) { _, _ = controldoc.Decode(in) })
}
