// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package webauthndriver

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// coseES256 is the COSE identifier of ECDSA with SHA-256 on P-256.
const coseES256 = -7

// Authenticator data flags (WebAuthn Level 2, section 6.1).
const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
	flagAttested     = 0x40
)

// credentialIDLength is the length of a generated credential identifier.
const credentialIDLength = 32

// coordinateLength is the length of a P-256 coordinate.
const coordinateLength = 32

// Credential is the secret half of a registered security key: its identifier,
// its private key and the signature counter. It is the state a [Store] keeps
// between the processes of one run, and it is never printed.
type Credential struct {
	ID      []byte
	Key     *ecdsa.PrivateKey
	Counter uint32
}

// Attestation is the answer to a registration ceremony, as the server takes it.
type Attestation struct {
	ClientDataJSON    []byte
	AttestationObject []byte
}

// Assertion is the answer to a sign-in or step-up ceremony, as the server
// takes it.
type Assertion struct {
	CredentialID      []byte
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
}

// CreationOptions are the parts of PublicKeyCredentialCreationOptions in
// WebAuthn's JSON serialisation that an authenticator acts on.
type CreationOptions struct {
	Challenge string `json:"challenge"`
	RP        struct {
		ID string `json:"id"`
	} `json:"rp"`
}

// RequestOptions are the parts of PublicKeyCredentialRequestOptions in
// WebAuthn's JSON serialisation that an authenticator acts on.
type RequestOptions struct {
	Challenge        string `json:"challenge"`
	RPID             string `json:"rpId"`
	AllowCredentials []struct {
		ID string `json:"id"`
	} `json:"allowCredentials"`
}

// ParseCreationOptions reads the options a registration returned.
func ParseCreationOptions(raw []byte) (CreationOptions, error) {
	var o CreationOptions
	if err := json.Unmarshal(raw, &o); err != nil {
		return o, fmt.Errorf("webauthndriver.ParseCreationOptions: %w", err)
	}
	if o.Challenge == "" || o.RP.ID == "" {
		return o, errors.New("webauthndriver.ParseCreationOptions: the options name no challenge or relying party")
	}
	return o, nil
}

// ParseRequestOptions reads the options a sign-in or step-up returned.
func ParseRequestOptions(raw []byte) (RequestOptions, error) {
	var o RequestOptions
	if err := json.Unmarshal(raw, &o); err != nil {
		return o, fmt.Errorf("webauthndriver.ParseRequestOptions: %w", err)
	}
	if o.Challenge == "" || o.RPID == "" {
		return o, errors.New("webauthndriver.ParseRequestOptions: the options name no challenge or relying party")
	}
	return o, nil
}

// Authenticator is a software security key with one credential. It asserts
// user presence and user verification on every ceremony, as a device with a
// PIN or a fingerprint does.
type Authenticator struct {
	cred   Credential
	random io.Reader
}

// NewAuthenticator makes a security key with a new credential, drawing its
// identifier and key from random.
func NewAuthenticator(random io.Reader) (*Authenticator, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), random)
	if err != nil {
		return nil, fmt.Errorf("webauthndriver.NewAuthenticator: %w", err)
	}
	id := make([]byte, credentialIDLength)
	if _, err := io.ReadFull(random, id); err != nil {
		return nil, fmt.Errorf("webauthndriver.NewAuthenticator: %w", err)
	}
	return &Authenticator{cred: Credential{ID: id, Key: key}, random: random}, nil
}

// AuthenticatorFor returns the security key that holds a stored credential and
// draws the randomness of its signatures from random.
func AuthenticatorFor(c Credential, random io.Reader) *Authenticator {
	return &Authenticator{cred: c, random: random}
}

// Credential returns the credential, with the counter as it stands.
func (a *Authenticator) Credential() Credential { return a.cred }

// Create answers a registration for the origin with "none" attestation.
func (a *Authenticator) Create(o CreationOptions, origin string) (Attestation, error) {
	clientData, err := clientDataJSON("webauthn.create", o.Challenge, origin)
	if err != nil {
		return Attestation{}, err
	}
	authData := authenticatorData(o.RP.ID, flagUserPresent|flagUserVerified|flagAttested, a.cred.Counter)
	authData = append(authData, make([]byte, 16)...) // the AAGUID of an anonymous authenticator
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(a.cred.ID)))
	authData = append(append(authData, a.cred.ID...), a.coseKey()...)
	object := cborMap(
		[2][]byte{cborText("fmt"), cborText("none")},
		[2][]byte{cborText("attStmt"), cborMap()},
		[2][]byte{cborText("authData"), cborBytes(authData)},
	)
	return Attestation{ClientDataJSON: clientData, AttestationObject: object}, nil
}

// Get answers a sign-in or step-up ceremony for the origin. It refuses options
// that do not allow this credential, because the server would refuse the
// answer without saying why. The counter advances; a caller must save the
// credential before it sends the assertion.
func (a *Authenticator) Get(o RequestOptions, origin string) (Assertion, error) {
	if !a.allowed(o) {
		return Assertion{}, errors.New("webauthndriver.Authenticator.Get: the server does not allow this security key")
	}
	clientData, err := clientDataJSON("webauthn.get", o.Challenge, origin)
	if err != nil {
		return Assertion{}, err
	}
	a.cred.Counter++
	authData := authenticatorData(o.RPID, flagUserPresent|flagUserVerified, a.cred.Counter)
	digest := sha256.Sum256(clientData)
	signed := sha256.Sum256(append(append([]byte{}, authData...), digest[:]...))
	sig, err := ecdsa.SignASN1(a.random, a.cred.Key, signed[:])
	if err != nil {
		return Assertion{}, fmt.Errorf("webauthndriver.Authenticator.Get: %w", err)
	}
	return Assertion{CredentialID: a.cred.ID, ClientDataJSON: clientData, AuthenticatorData: authData, Signature: sig}, nil
}

// allowed reports whether the options allow this credential; an empty list
// allows every credential, as in WebAuthn.
func (a *Authenticator) allowed(o RequestOptions) bool {
	if len(o.AllowCredentials) == 0 {
		return true
	}
	for _, c := range o.AllowCredentials {
		if id, err := base64.RawURLEncoding.DecodeString(c.ID); err == nil && bytes.Equal(id, a.cred.ID) {
			return true
		}
	}
	return false
}

// coseKey encodes the public key as a COSE_Key (RFC 9052) for ES256, with the
// map keys in the order of the CTAP2 canonical form.
func (a *Authenticator) coseKey() []byte {
	pub := a.cred.Key.PublicKey
	return cborMap(
		[2][]byte{cborInt(1), cborInt(2)},         // kty: EC2
		[2][]byte{cborInt(3), cborInt(coseES256)}, // alg
		[2][]byte{cborInt(-1), cborInt(1)},        // crv: P-256
		[2][]byte{cborInt(-2), cborBytes(pub.X.FillBytes(make([]byte, coordinateLength)))},
		[2][]byte{cborInt(-3), cborBytes(pub.Y.FillBytes(make([]byte, coordinateLength)))},
	)
}

// authenticatorData is the fixed part of authenticator data: the hash of the
// relying party identifier, the flags and the signature counter.
func authenticatorData(rpID string, flags byte, counter uint32) []byte {
	hash := sha256.Sum256([]byte(rpID))
	out := append(hash[:], flags)
	return binary.BigEndian.AppendUint32(out, counter)
}

// clientDataJSON is the record a browser makes of a ceremony.
func clientDataJSON(ceremony, challenge, origin string) ([]byte, error) {
	out, err := json.Marshal(map[string]any{
		"type": ceremony, "challenge": challenge, "origin": origin, "crossOrigin": false,
	})
	if err != nil {
		return nil, fmt.Errorf("webauthndriver.clientDataJSON: %w", err)
	}
	return out, nil
}
