// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest/playintegrity"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

func mustSettings(t *testing.T, p settings.Profile) Settings {
	t.Helper()
	s, err := SettingsFor(p)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Verifies: SEC-007, SEC-001.
// Every level rule: Android AL1 is a recognised app, AL2 adds a
// hardware key and the profile's device verdict, AL3 adds strong
// integrity; iOS AL1 is App Attest with a software key and AL2 adds the
// Secure Enclave; a software key caps either at AL1; the development
// provider is AL0.
func TestAssess(t *testing.T) {
	t.Parallel()
	android := func(storage KeyStorage, recognised bool, verdict playintegrity.DeviceLabel) Proof {
		return Proof{Provider: ProviderPlayIntegrity, KeyStorage: storage, AppRecognised: recognised, DeviceVerdict: verdict}
	}
	ios := func(storage KeyStorage) Proof { return Proof{Provider: ProviderAppAttest, KeyStorage: storage} }
	standard, strict := mustSettings(t, settings.Standard), mustSettings(t, settings.Strict)
	maximum := mustSettings(t, settings.Maximum)
	for _, c := range []struct {
		name  string
		proof Proof
		conf  Settings
		want  Level
	}{
		{"development", Proof{Provider: ProviderDevelopment, KeyStorage: KeyStorageSoftware}, standard, AL0},
		{"unknown provider", Proof{}, standard, AL0},
		{"android, app not recognised", android(KeyStorageTEE, false, playintegrity.LabelStrong), standard, AL0},
		{"android, software key, no verdict", android(KeyStorageSoftware, true, playintegrity.LabelNone), standard, AL1},
		{"android, TEE key, no device verdict", android(KeyStorageTEE, true, playintegrity.LabelNone), standard, AL1},
		{"android, TEE key, basic, standard", android(KeyStorageTEE, true, playintegrity.LabelBasic), standard, AL2},
		{"android, StrongBox key, basic, standard", android(KeyStorageStrongBox, true, playintegrity.LabelBasic), standard, AL2},
		{"android, TEE key, basic, strict", android(KeyStorageTEE, true, playintegrity.LabelBasic), strict, AL1},
		{"android, TEE key, device, strict", android(KeyStorageTEE, true, playintegrity.LabelDevice), strict, AL2},
		{"android, TEE key, device, maximum", android(KeyStorageTEE, true, playintegrity.LabelDevice), maximum, AL2},
		{"android, TEE key, strong, standard", android(KeyStorageTEE, true, playintegrity.LabelStrong), standard, AL3},
		{"android, TEE key, strong, maximum", android(KeyStorageStrongBox, true, playintegrity.LabelStrong), maximum, AL3},
		{"android, software key caps strong integrity", android(KeyStorageSoftware, true, playintegrity.LabelStrong), standard, AL1},
		{"android, unspecified key caps strong integrity", android(KeyStorageUnspecified, true, playintegrity.LabelStrong), standard, AL1},
		{"ios, software key", ios(KeyStorageSoftware), standard, AL1},
		{"ios, secure enclave", ios(KeyStorageSecureEnclave), standard, AL2},
		{"ios, secure enclave, maximum", ios(KeyStorageSecureEnclave), maximum, AL2},
	} {
		if got := Assess(c.proof, c.conf); got != c.want {
			t.Errorf("%s: Assess = %s, want %s", c.name, got, c.want)
		}
	}
}

// Verifies: SEC-001, SEC-007.
// A profile's defaults decide whether a software key is refused and which
// device verdict AL2 needs.
func TestSettingsFor(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		profile  settings.Profile
		hardware bool
		verdict  playintegrity.DeviceLabel
		minSync  Level
	}{
		{settings.Standard, false, playintegrity.LabelBasic, AL0},
		{settings.Strict, true, playintegrity.LabelDevice, AL1},
		{settings.Maximum, true, playintegrity.LabelDevice, AL2},
	} {
		s := mustSettings(t, c.profile)
		if s.requireHardware() != c.hardware || s.AndroidDeviceVerdictAL2 != c.verdict || s.MinAssuranceForSync != c.minSync {
			t.Errorf("%s: %+v", c.profile, s)
		}
	}
	if _, err := SettingsFor("paranoid"); err == nil {
		t.Error("an unknown profile was accepted")
	}
	loose := mustSettings(t, settings.Standard)
	loose.AllowSoftwareKeys = false
	if !loose.requireHardware() {
		t.Error("standard without allowSoftwareKeys accepts a software key")
	}
}

// Verifies: SEC-002.
// A proven storage supports the claims at or below it, never above.
func TestCovers(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		proven, claim KeyStorage
		want          bool
	}{
		{KeyStorageSoftware, KeyStorageUnspecified, true},
		{KeyStorageSoftware, KeyStorageSoftware, true},
		{KeyStorageSoftware, KeyStorageTEE, false},
		{KeyStorageTEE, KeyStorageTEE, true},
		{KeyStorageTEE, KeyStorageStrongBox, false},
		{KeyStorageStrongBox, KeyStorageTEE, true},
		{KeyStorageStrongBox, KeyStorageStrongBox, true},
		{KeyStorageStrongBox, KeyStorageSecureEnclave, false},
		{KeyStorageSoftware, KeyStorageSecureEnclave, false},
		{KeyStorageSecureEnclave, KeyStorageSecureEnclave, true},
	} {
		if got := c.proven.covers(c.claim); got != c.want {
			t.Errorf("%s covers %s = %v", c.proven, c.claim, got)
		}
	}
}

// fixedValues is a configuration: a profile and the values that differ.
type fixedValues struct {
	profile settings.Profile
	set     map[settings.Key]settings.Value
}

func (f fixedValues) Profile() settings.Profile { return f.profile }

func (f fixedValues) Get(k settings.Key) settings.Value {
	if v, ok := f.set[k]; ok {
		return v
	}
	s, _ := settings.Lookup(k)
	v, _ := s.Defaults.For(f.profile)
	return v
}

// Verifies: SEC-182.
// An operator's override replaces the profile's preset for the settings
// the device service acts on.
func TestSettingsOf(t *testing.T) {
	t.Parallel()
	s, err := SettingsOf(fixedValues{profile: settings.Standard, set: map[settings.Key]settings.Value{
		settings.AllowSoftwareKeys:               settings.BoolValue(false),
		settings.AndroidDeviceVerdictAL2:         settings.TextValue("MEETS_DEVICE_INTEGRITY"),
		settings.AccessTokenLifetime:             settings.IntValue(120),
		settings.AndroidRefreshRequiresIntegrity: settings.BoolValue(true),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !s.requireHardware() || s.AndroidDeviceVerdictAL2 != playintegrity.LabelDevice ||
		s.AccessTokenLifetime != 120*time.Second || !s.AndroidRefreshRequiresIntegrity || s.Profile != settings.Standard {
		t.Errorf("SettingsOf = %+v", s)
	}
	if plain, err := SettingsOf(fixedValues{profile: settings.Strict}); err != nil || plain != mustSettings(t, settings.Strict) {
		t.Errorf("a configuration with no overrides: %+v %v", plain, err)
	}
}

// Verifies: SEC-007.
// The levels are ordered AL0 to AL3, and a value that is no level ranks
// below all of them.
func TestLevelRank(t *testing.T) {
	t.Parallel()
	for i, l := range []Level{AL0, AL1, AL2, AL3} {
		if l.Rank() != i {
			t.Errorf("%s ranks %d, want %d", l, l.Rank(), i)
		}
	}
	if Level("AL9").Rank() >= 0 || Level("").Rank() >= 0 {
		t.Error("an unknown level ranks as a level")
	}
}

// Verifies: SEC-007.
// An iOS device reaches AL3 only with a Secure Enclave key and a verified
// receipt whose risk metric is at most riskMetricMaxAL3: 5 under standard,
// 3 under strict, 1 under maximum. Android proofs ignore the receipt.
func TestAssessFraudReceipt_SEC_007(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		profile settings.Profile
		limit   int
	}{{settings.Standard, 5}, {settings.Strict, 3}, {settings.Maximum, 1}} {
		conf := mustSettings(t, c.profile)
		if conf.RiskMetricMaxAL3 != c.limit {
			t.Fatalf("%s: riskMetricMaxAL3 = %d, want %d", c.profile, conf.RiskMetricMaxAL3, c.limit)
		}
		ios := func(storage KeyStorage, receipt bool, risk int) Proof {
			return Proof{Provider: ProviderAppAttest, KeyStorage: storage, FraudReceipt: receipt, RiskMetric: risk}
		}
		for _, k := range []struct {
			name  string
			proof Proof
			want  Level
		}{
			{"risk 0", ios(KeyStorageSecureEnclave, true, 0), AL3},
			{"risk at the limit", ios(KeyStorageSecureEnclave, true, c.limit), AL3},
			{"risk above the limit", ios(KeyStorageSecureEnclave, true, c.limit+1), AL2},
			{"negative risk", ios(KeyStorageSecureEnclave, true, -1), AL2},
			{"no receipt", ios(KeyStorageSecureEnclave, false, 0), AL2},
			{"software key with a good receipt", ios(KeyStorageSoftware, true, 0), AL1},
			{"android ignores the receipt", Proof{Provider: ProviderPlayIntegrity, KeyStorage: KeyStorageTEE, AppRecognised: true, DeviceVerdict: playintegrity.LabelDevice, FraudReceipt: true}, AL2},
		} {
			if got := Assess(k.proof, conf); got != k.want {
				t.Errorf("%s, %s: Assess = %s, want %s", c.profile, k.name, got, k.want)
			}
		}
	}
	zero := mustSettings(t, settings.Maximum)
	zero.RiskMetricMaxAL3 = 0
	if got := Assess(Proof{Provider: ProviderAppAttest, KeyStorage: KeyStorageSecureEnclave, FraudReceipt: true, RiskMetric: 1}, zero); got != AL2 {
		t.Errorf("a limit of 0 with risk 1 = %s, want AL2", got)
	}
	if got := Assess(Proof{Provider: ProviderAppAttest, KeyStorage: KeyStorageSecureEnclave, FraudReceipt: true}, zero); got != AL3 {
		t.Errorf("a limit of 0 with risk 0 = %s, want AL3", got)
	}
}
