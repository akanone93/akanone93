// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package governance

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// PayloadType is the DSSE payload type of a compliance report's statement.
const PayloadType = "application/vnd.plux.compliance-report+json"

// Errors Envelope.Open returns, wrapped with %w.
var (
	// ErrMalformed reports an envelope or a statement that cannot be read.
	ErrMalformed = errors.New("governance: the envelope is malformed")
	// ErrSignature reports an envelope no trusted key signed.
	ErrSignature = errors.New("governance: no trusted key signed the envelope")
)

// Envelope is a DSSE envelope (https://github.com/secure-systems-lab/dsse):
// a payload, its type and the signatures over both.
type Envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"`
	Signatures  []Signature `json:"signatures"`
}

// Signature is one signature of an envelope; the fields are base64 as DSSE
// defines them, except KeyID, which names the key.
type Signature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

// PAE returns the bytes DSSE signs: the pre-authentication encoding of the
// payload type and the payload, so that a signature binds both and cannot be
// moved to another type.
func PAE(payloadType string, payload []byte) []byte {
	out := []byte("DSSEv1 " + strconv.Itoa(len(payloadType)) + " " + payloadType + " " + strconv.Itoa(len(payload)) + " ")
	return append(out, payload...)
}

// NewEnvelope assembles the envelope of a statement signed with the key
// keyID; sig is the signature over PAE(PayloadType, statement).
func NewEnvelope(statement []byte, keyID string, sig []byte) Envelope {
	return Envelope{
		PayloadType: PayloadType,
		Payload:     base64.StdEncoding.EncodeToString(statement),
		Signatures:  []Signature{{KeyID: keyID, Sig: base64.StdEncoding.EncodeToString(sig)}},
	}
}

// ParseEnvelope reads an envelope from its JSON.
func ParseEnvelope(data []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return Envelope{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	return e, nil
}

// Statement returns the envelope's payload without checking it.
func (e Envelope) Statement() ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("%w: the payload is not base64", ErrMalformed)
	}
	return payload, nil
}

// Open verifies the envelope against the audit keys an auditor trusts, by
// key identifier, and returns the report it carries. The signature is
// checked before the payload is parsed, and a payload type, a key or a
// report version it does not know is refused (SEC-122).
func (e Envelope) Open(keys map[string]ed25519.PublicKey) (Report, error) {
	if e.PayloadType != PayloadType {
		return Report{}, fmt.Errorf("%w: the payload type is %q", ErrMalformed, e.PayloadType)
	}
	payload, err := e.Statement()
	if err != nil {
		return Report{}, err
	}
	if !e.signedBy(keys, PAE(e.PayloadType, payload)) {
		return Report{}, ErrSignature
	}
	var r Report
	if err := json.Unmarshal(payload, &r); err != nil {
		return Report{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if r.Version != ReportVersion {
		return Report{}, fmt.Errorf("%w: report version %d is not known", ErrMalformed, r.Version)
	}
	return r, nil
}

// signedBy reports whether some signature verifies with a trusted key.
func (e Envelope) signedBy(keys map[string]ed25519.PublicKey, message []byte) bool {
	for _, s := range e.Signatures {
		key, ok := keys[s.KeyID]
		if !ok || len(key) != ed25519.PublicKeySize {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(s.Sig)
		if err != nil {
			continue
		}
		if ed25519.Verify(key, message, sig) {
			return true
		}
	}
	return false
}
