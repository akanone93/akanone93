// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package reconstruct

import "encoding/json"

// ManifestType and ManifestSpecVersion identify the reconstruction manifest.
const (
	ManifestType        = "reconstruction"
	ManifestSpecVersion = 1
)

// Subject kinds a reconstruction is requested by.
const (
	KindDevice = "device"
	KindUser   = "user"
)

// Manifest is the reconstruction manifest: the document a reconstruction
// release is signed over (SEC-142). schema/json/reconstruction.schema.json
// describes its JSON form, and its RFC 8785 canonical form is what the
// signatures cover.
type Manifest struct {
	Type        string `json:"type"`
	SpecVersion int    `json:"specVersion"`
	// ID is the reconstruction release's identifier.
	ID string `json:"id"`
	// App is the app's identifier, Environment the key of the environment
	// the device belongs to, and Channel the audit channel.
	App         string `json:"app"`
	Environment string `json:"environment"`
	Channel     string `json:"channel"`
	// IssuedAt is when the release was made and Moment the reconstructed
	// instant, both RFC 3339 in UTC.
	IssuedAt string `json:"issuedAt"`
	Moment   string `json:"moment"`
	// Subject is who the moment was resolved for.
	Subject SubjectRef `json:"subject"`
	// Release is the release the device ran.
	Release ReleaseRef `json:"release"`
	// Bundles are the bundles the device held, sorted by key; the app
	// bundle has the empty key.
	Bundles []Bundle `json:"bundles"`
	// Control is the control document in force.
	Control Control `json:"control"`
	// Experiments are the assignments in force.
	Experiments []Experiment `json:"experiments"`
	// Presentation is what the device last reported.
	Presentation Presentation `json:"presentation"`
	// Route is the page the device last reported viewing; absent when none
	// was recorded.
	Route string `json:"route,omitempty"`
}

// SubjectRef names the device and, when the request named one, the user.
type SubjectRef struct {
	Kind     string `json:"kind"`
	DeviceID string `json:"deviceId"`
	UserRef  string `json:"userRef,omitempty"`
}

// ReleaseRef is the release and channel the device ran.
type ReleaseRef struct {
	Sequence int64 `json:"sequence"`
	// Channel is the channel the device followed.
	Channel string `json:"channel"`
}

// Bundle is one bundle exactly as shipped.
type Bundle struct {
	Key     string `json:"key"`
	Version int64  `json:"version"`
	// Hash is "sha256:<hex>" of the bundle.
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Control is the control document in force at the moment. Version is zero,
// with no switches, when the channel had none yet. The three parts are the
// document's own JSON.
type Control struct {
	Version         int64           `json:"version"`
	KillSwitches    json.RawMessage `json:"killSwitches"`
	MandatoryUpdate json.RawMessage `json:"mandatoryUpdate"`
	ForcedRollback  json.RawMessage `json:"forcedRollback"`
}

// Experiment is one assignment in force.
type Experiment struct {
	Key     string `json:"key"`
	Layer   string `json:"layer"`
	Variant string `json:"variant"`
}

// Presentation is the locale, device class, theme and text scale the device
// last reported; the last two are absent until the host reports them.
type Presentation struct {
	Locale           string `json:"locale"`
	DeviceClass      string `json:"deviceClass"`
	Theme            string `json:"theme,omitempty"`
	TextScalePercent int32  `json:"textScalePercent,omitempty"`
}
