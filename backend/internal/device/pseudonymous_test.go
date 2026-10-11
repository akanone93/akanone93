// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"reflect"
	"regexp"
	"testing"
)

// hardwareIdentifier matches field names that would carry an identifier of
// the hardware or the user across apps.
var hardwareIdentifier = regexp.MustCompile(`(?i)imei|meid|serial|androidid|idfv|idfa|advertis|mac(addr|$)|hardware|udid`)

// Verifies: SEC-160.
// A device is registered with a description of its software and with keys,
// never with an identifier of its hardware: the registration carries none,
// so the identifier the server mints (a random UUIDv7, see the server
// package's test) cannot be derived from one.
func TestRegistrationCarriesNoHardwareIdentifier(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[AttestedRegistration]()
	for f := range typ.Fields() {
		if hardwareIdentifier.MatchString(f.Name) {
			t.Errorf("AttestedRegistration.%s looks like a hardware identifier", f.Name)
		}
	}
	if typ.NumField() == 0 {
		t.Fatal("AttestedRegistration has no fields; the check proves nothing")
	}
}
