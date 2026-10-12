// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// The reference API uses JSON Web Signatures in one shape only: ES256, the
// compact serialisation, a key named by `kid`. The identity provider signs
// its users' tokens with it and the bank verifies the gateway's assertions
// with it; nothing else is accepted, `none` and the symmetric algorithms
// included.

// jwsHeader is the protected header of a token.
type jwsHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ,omitempty"`
	Kid string `json:"kid,omitempty"`
}

// jwk is an ES256 public key as a JSON Web Key.
type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Kid string `json:"kid"`
	Alg string `json:"alg,omitempty"`
	Use string `json:"use,omitempty"`
}

// jwkSet is a JSON Web Key Set.
type jwkSet struct {
	Keys []jwk `json:"keys"`
}

// publicJWK describes a public key.
func publicJWK(pub *ecdsa.PublicKey, kid string) (jwk, error) {
	raw, err := pub.Bytes()
	if err != nil {
		return jwk{}, fmt.Errorf("encoding a public key: %w", err)
	}
	return jwk{
		Kty: "EC", Crv: "P-256", Kid: kid, Alg: "ES256", Use: "sig",
		X: base64.RawURLEncoding.EncodeToString(raw[1:33]),
		Y: base64.RawURLEncoding.EncodeToString(raw[33:]),
	}, nil
}

// key reads the public key of a JWK; it refuses any curve but P-256.
func (k jwk) key() (*ecdsa.PublicKey, error) {
	if k.Kty != "EC" || k.Crv != "P-256" {
		return nil, errors.New("the key is not an EC P-256 key")
	}
	x, errX := base64.RawURLEncoding.DecodeString(k.X)
	y, errY := base64.RawURLEncoding.DecodeString(k.Y)
	if errX != nil || errY != nil || len(x) != 32 || len(y) != 32 {
		return nil, errors.New("the key's coordinates are malformed")
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append([]byte{4}, append(x, y...)...))
	if err != nil {
		return nil, fmt.Errorf("the key is not on the curve: %w", err)
	}
	return pub, nil
}

// signES256 signs header and claims as a compact JWS.
func signES256(key *ecdsa.PrivateKey, h jwsHeader, claims any) (string, error) {
	h.Alg = "ES256"
	head, err := json.Marshal(h)
	if err != nil {
		return "", fmt.Errorf("encoding a header: %w", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encoding claims: %w", err)
	}
	input := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(input))
	der, err := ecdsa.SignASN1(rand.Reader, key, sum[:])
	if err != nil {
		return "", fmt.Errorf("signing: %w", err)
	}
	var sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(der, &sig); err != nil {
		return "", fmt.Errorf("reading a signature: %w", err)
	}
	raw := make([]byte, 64)
	sig.R.FillBytes(raw[:32])
	sig.S.FillBytes(raw[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(raw), nil
}

// parsedJWS is a compact JWS split into its parts.
type parsedJWS struct {
	header  jwsHeader
	payload []byte
	input   string
	sig     []byte
}

// parseJWS splits a compact JWS and requires ES256.
func parseJWS(token string) (parsedJWS, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return parsedJWS{}, errors.New("not a compact JWS")
	}
	head, errH := base64.RawURLEncoding.DecodeString(parts[0])
	payload, errP := base64.RawURLEncoding.DecodeString(parts[1])
	sig, errS := base64.RawURLEncoding.DecodeString(parts[2])
	if errH != nil || errP != nil || errS != nil || len(sig) != 64 {
		return parsedJWS{}, errors.New("a part of the token is malformed")
	}
	var h jwsHeader
	if err := json.Unmarshal(head, &h); err != nil || h.Alg != "ES256" {
		return parsedJWS{}, errors.New("the token is not signed with ES256")
	}
	return parsedJWS{header: h, payload: payload, input: parts[0] + "." + parts[1], sig: sig}, nil
}

// verify checks the signature with pub.
func (p parsedJWS) verify(pub *ecdsa.PublicKey) bool {
	sum := sha256.Sum256([]byte(p.input))
	der, err := asn1.Marshal(struct{ R, S *big.Int }{new(big.Int).SetBytes(p.sig[:32]), new(big.Int).SetBytes(p.sig[32:])})
	return err == nil && ecdsa.VerifyASN1(pub, sum[:], der)
}
