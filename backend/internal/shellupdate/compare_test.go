// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package shellupdate_test

import (
	"reflect"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/generator"
	"github.com/nightCode42/plux3/backend/internal/shellupdate"
)

func spec() generator.ShellSpec {
	return generator.ShellSpec{
		Name: "Shop", Package: "shop", ApplicationID: "com.example.shop", BundleID: "com.example.shop", AppVersion: "1.0.0+1",
		SplashColor: "#FFFFFF", DeviceAPIs: []string{"camera", "location"}, Plux: generator.PluxSource{EntryRoute: "home"},
	}
}

// TestCompareNamesWhatNeedsAStoreBuild_GEN_004 proves a changed shell setting
// is reported with its reason, in a fixed order, and that nothing else is.
func TestCompareNamesWhatNeedsAStoreBuild_GEN_004(t *testing.T) {
	old := spec()
	if got := shellupdate.Compare(old, old); len(got) != 0 {
		t.Fatalf("identical settings differ: %v", got)
	}
	reordered := spec()
	reordered.DeviceAPIs = []string{"location", "camera", "camera"}
	if got := shellupdate.Compare(old, reordered); len(got) != 0 {
		t.Fatalf("a reordering is a change: %v", got)
	}
	cur := spec()
	cur.AppVersion = "1.1.0+2"
	cur.Name = "Shop Plus"
	cur.Push = true
	cur.DeviceAPIs = []string{"camera"}
	got := shellupdate.Compare(old, cur)
	var fields []string
	for _, c := range got {
		fields = append(fields, c.Field)
		if c.Reason == "" {
			t.Errorf("%s has no reason", c.Field)
		}
	}
	if want := []string{"name", "appVersion", "deviceApis", "push"}; !reflect.DeepEqual(fields, want) {
		t.Fatalf("fields %v, want %v", fields, want)
	}
	if got[2].Old != "camera,location" || got[2].New != "camera" {
		t.Errorf("device APIs: %+v", got[2])
	}
}

// TestCheckRefusesInvalidSettings_GEN_004 refuses what the generator would.
func TestCheckRefusesInvalidSettings_GEN_004(t *testing.T) {
	bad := spec()
	bad.ApplicationID = "not an id"
	if _, err := shellupdate.Check(bad); err == nil {
		t.Fatal("an invalid application ID was accepted")
	}
	if _, err := shellupdate.Check(spec()); err != nil {
		t.Fatalf("valid settings refused: %v", err)
	}
}
