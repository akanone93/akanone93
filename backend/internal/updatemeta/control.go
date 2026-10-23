// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package updatemeta

import (
	"encoding/base64"
	"fmt"
	"math"
	"time"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
)

// Control is the signed part of a control document file (ADR-0077,
// schema/update/control.schema.json): a small JSON header a verifier can
// read before it trusts anything, and the FlatBuffers document of
// schema/control.fbs as base64. The header repeats the fields a device
// checks before it parses the document (version, expiry and the channel it
// is for), and the verifier refuses a file whose document disagrees with
// them.
type Control struct {
	Type        string `json:"_type"`
	Version     int64  `json:"version"`
	Expires     string `json:"expires"`
	SpecVersion string `json:"specVersion"`
	App         string `json:"app"`
	Environment string `json:"environment"`
	Channel     string `json:"channel"`
	IssuedAt    string `json:"issuedAt"`
	// Document is the encoded controldoc.Document in standard base64.
	Document string `json:"document"`
}

// NewControl builds the signed part of a control document from the
// encoded FlatBuffers document and the fields its header repeats.
func NewControl(d controldoc.Document, encoded []byte) Control {
	return Control{
		Type: RoleControl, Version: int64(min(d.Version, math.MaxInt64)), Expires: FormatTime(d.ExpiresAt), SpecVersion: SpecVersion,
		App: d.AppID, Environment: d.Environment, Channel: d.Channel, IssuedAt: FormatTime(d.IssuedAt),
		Document: base64.StdEncoding.EncodeToString(encoded),
	}
}

// ParseControl reads and checks the signed part of a control document
// file. It does not read the FlatBuffers document: that is only done after
// the signature has been verified (SEC-052).
func ParseControl(signed []byte) (Control, error) {
	var c Control
	if err := decodeStrict(signed, &c); err != nil {
		return Control{}, err
	}
	if err := header(RoleControl, c.Type, c.Version, c.Expires, c.SpecVersion); err != nil {
		return Control{}, err
	}
	if _, err := time.Parse(time.RFC3339, c.IssuedAt); err != nil {
		return Control{}, fmt.Errorf("%w: issuedAt: %w", ErrFormat, err)
	}
	if c.App == "" || c.Environment == "" || c.Channel == "" {
		return Control{}, fmt.Errorf("%w: the app, environment and channel are required", ErrFormat)
	}
	if c.Document == "" || !base64Pattern.MatchString(c.Document) {
		return Control{}, fmt.Errorf("%w: the document is not base64", ErrFormat)
	}
	return c, nil
}

// ControlScope names the document a verifier expects: the channel it is
// asking about.
type ControlScope struct {
	App, Environment, Channel string
}

// Control verifies a control document file and returns the document it
// carries. The checks run in the order a device makes them, and the
// FlatBuffers document is not read until the first three have passed
// (SEC-052):
//
//  1. the signatures meet the threshold the root gives the control role
//     (ErrThreshold, PLX-3100); a root that delegates no control role
//     never does;
//  2. the document has not expired (ErrExpired, PLX-3101);
//  3. its version is not below the newest already trusted (ErrRollback,
//     PLX-3102);
//  4. it is for the channel asked about, and the document agrees with the
//     header that was signed (ErrMismatch).
func (v Verifier) Control(raw []byte, scope ControlScope) (controldoc.Document, error) {
	d, err := ParseDocument(raw)
	if err != nil {
		return controldoc.Document{}, err
	}
	c, err := ParseControl(d.Signed)
	if err != nil {
		return controldoc.Document{}, err
	}
	if _, delegated := v.Root.Roles.Named(RoleControl); !delegated {
		return controldoc.Document{}, fmt.Errorf("control: the root delegates no control role: %w", ErrThreshold)
	}
	if err := v.checkRole(RoleControl, d, c.Expires, c.Version, v.Floor.Control); err != nil {
		return controldoc.Document{}, err
	}
	if c.App != scope.App || c.Environment != scope.Environment || c.Channel != scope.Channel {
		return controldoc.Document{}, fmt.Errorf("%w: the control document is for %s/%s/%s, not %s/%s/%s", ErrMismatch,
			c.App, c.Environment, c.Channel, scope.App, scope.Environment, scope.Channel)
	}
	return c.document()
}

// document reads the FlatBuffers document of a verified header and
// checks that it says what the header says.
func (c Control) document() (controldoc.Document, error) {
	encoded, err := base64.StdEncoding.DecodeString(c.Document)
	if err != nil {
		return controldoc.Document{}, fmt.Errorf("%w: the document: %w", ErrFormat, err)
	}
	doc, err := controldoc.Decode(encoded)
	if err != nil {
		return controldoc.Document{}, fmt.Errorf("%w: %w", ErrFormat, err)
	}
	if int64(min(doc.Version, math.MaxInt64)) != c.Version || doc.AppID != c.App || doc.Environment != c.Environment || doc.Channel != c.Channel ||
		FormatTime(doc.IssuedAt) != c.IssuedAt || FormatTime(doc.ExpiresAt) != c.Expires {
		return controldoc.Document{}, fmt.Errorf("%w: the document disagrees with the signed header", ErrMismatch)
	}
	return doc, nil
}
