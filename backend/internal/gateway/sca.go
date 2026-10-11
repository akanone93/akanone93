// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl/decimal"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

const (
	// SigningPrefix opens the message an SCA key signs. It names the format
	// and its version, so a signature made for another purpose or version
	// never verifies as a transaction confirmation.
	SigningPrefix = "plux-sca-v1"

	challengeBytes = 32
	// usedMargin keeps a spent challenge remembered past its expiry.
	usedMargin = time.Minute
	// jsonDepth bounds the nesting of a transaction summary and body.
	jsonDepth = 32

	challengeKey = "sca:challenge:"
	usedKey      = "sca:used:"
)

// SigningInput returns the message a device's SCA key signs to confirm a
// transaction, and the server verifies (SEC-027, SEC-028). It is
//
//	"plux-sca-v1" 0x00 field(challengeID) field(upstream) field(operation)
//	field(challenge) field(summary)
//
// where field(x) is the length of x as four big-endian bytes followed by
// x, challengeID, upstream and operation are UTF-8, challenge is the
// random bytes of the challenge, and summary is the RFC 8785 canonical
// JSON the challenge was created with. The key signs the message with
// ECDSA over P-256 and SHA-256 (the platform hashes it), and the
// signature is ASN.1 DER.
func SigningInput(challengeID, upstream, operation string, challenge, summary []byte) []byte {
	var b bytes.Buffer
	b.WriteString(SigningPrefix)
	b.WriteByte(0)
	for _, f := range [][]byte{[]byte(challengeID), []byte(upstream), []byte(operation), challenge, summary} {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(f))) //nolint:gosec // fields are bounded by the request size
		b.Write(n[:])
		b.Write(f)
	}
	return b.Bytes()
}

// SCAKeys finds the SCA key a device enrolled.
type SCAKeys interface {
	SCAKey(ctx context.Context, d device.Identity) (device.SCAKey, error)
}

// SCA issues the challenges of strong customer authentication and verifies
// the signatures that answer them (SEC-027, SEC-028, SEC-032).
type SCA struct {
	// Cache holds the challenges and remembers the spent ones. Losing it
	// costs a confirmation, never correctness: a lost challenge is
	// unknown, and a spent one is remembered for longer than it is valid.
	Cache cache.Cache
	// Keys finds the device's SCA key.
	Keys SCAKeys
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// Random supplies challenges; nil uses crypto/rand.
	Random io.Reader
}

// Challenge is a challenge for the device to answer.
type Challenge struct {
	ID        string
	Bytes     []byte
	ExpiresAt time.Time
}

// record is what the cache keeps of a challenge.
type record struct {
	Device    string `json:"d"`
	App       string `json:"a"`
	Env       string `json:"e"`
	Upstream  string `json:"u"`
	Operation string `json:"o"`
	Challenge []byte `json:"c"`
	Summary   []byte `json:"s"`
}

func (s *SCA) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Create issues a single-use challenge bound to the device, the upstream's
// operation and the summary, which is what the user is shown. The summary
// must be RFC 8785 canonical JSON with the string members the operation's
// mapping names: amount, currency and payee, and summary when mapped.
func (s *SCA) Create(ctx context.Context, d device.Identity, u Upstream, op Operation, summary []byte, lifetime time.Duration) (Challenge, error) {
	if op.Transaction == nil {
		return Challenge{}, plxerr.New(plxerr.InvalidFormat, "operation %q is not a transaction", op.Name)
	}
	if _, err := s.Keys.SCAKey(ctx, d); err != nil {
		if code, _ := plxerr.CodeOf(err); code == plxerr.ResourceNotFound {
			return Challenge{}, plxerr.New(plxerr.PreconditionFailed, "the device has no SCA key; enrol one first")
		}
		return Challenge{}, err //nolint:wrapcheck // a domain error
	}
	if _, err := parseSummary(summary, *op.Transaction); err != nil {
		return Challenge{}, err
	}
	id, err := randomID(s.Random)
	if err != nil {
		return Challenge{}, err
	}
	challenge := make([]byte, challengeBytes)
	src := s.Random
	if src == nil {
		src = rand.Reader
	}
	if _, err := io.ReadFull(src, challenge); err != nil {
		return Challenge{}, fmt.Errorf("gateway: draw a challenge: %w", err)
	}
	value, err := json.Marshal(record{
		Device: d.DeviceID, App: d.AppID, Env: d.EnvironmentID, Upstream: u.Name, Operation: op.Name,
		Challenge: challenge, Summary: summary,
	})
	if err != nil {
		return Challenge{}, fmt.Errorf("gateway: encode a challenge: %w", err)
	}
	if err := s.Cache.Set(ctx, challengeKey+id, value, lifetime); err != nil {
		return Challenge{}, plxerr.Wrap(plxerr.UpstreamUnavailable, err, "the challenge store is unavailable")
	}
	return Challenge{ID: id, Bytes: challenge, ExpiresAt: s.now().Add(lifetime).UTC().Truncate(time.Second)}, nil
}

// Verify accepts a transaction call: the challenge must be one this
// server issued to this device for this operation, unexpired and unused;
// the request body must carry the amount, currency and payee the user
// confirmed; and the signature must verify with the device's SCA key. It
// spends the challenge first, so a second call with it fails whatever the
// first did. It returns what the assertion carries.
func (s *SCA) Verify(ctx context.Context, d device.Identity, u Upstream, op Operation, challengeID string, signature, body []byte, lifetime time.Duration) (SCAProof, error) {
	if op.Transaction == nil {
		return SCAProof{}, plxerr.New(plxerr.InvalidFormat, "operation %q is not a transaction", op.Name)
	}
	if challengeID == "" || len(challengeID) > 64 {
		return SCAProof{}, plxerr.New(plxerr.SCAChallengeInvalid, "the call names no challenge")
	}
	raw, found, err := s.Cache.Get(ctx, challengeKey+challengeID)
	if err != nil {
		return SCAProof{}, plxerr.Wrap(plxerr.UpstreamUnavailable, err, "the challenge store is unavailable")
	}
	var rec record
	if !found || json.Unmarshal(raw, &rec) != nil {
		return SCAProof{}, plxerr.New(plxerr.SCAChallengeInvalid, "the challenge is unknown or has expired")
	}
	if rec.Device != d.DeviceID || rec.App != d.AppID || rec.Env != d.EnvironmentID || rec.Upstream != u.Name || rec.Operation != op.Name {
		return SCAProof{}, plxerr.New(plxerr.SCAChallengeInvalid, "the challenge was issued for another device or operation")
	}
	first, err := s.Cache.SetNX(ctx, usedKey+challengeID, []byte{1}, lifetime+usedMargin)
	if err != nil {
		return SCAProof{}, plxerr.Wrap(plxerr.UpstreamUnavailable, err, "the challenge store is unavailable")
	}
	if !first {
		return SCAProof{}, plxerr.New(plxerr.SCAChallengeInvalid, "the challenge was already used")
	}
	if err := checkLinking(body, *op.Transaction, rec.Summary); err != nil {
		return SCAProof{}, err
	}
	key, err := s.Keys.SCAKey(ctx, d)
	if err != nil {
		return SCAProof{}, plxerr.New(plxerr.SCASignatureInvalid, "the device has no SCA key")
	}
	digest := sha256.Sum256(SigningInput(challengeID, u.Name, op.Name, rec.Challenge, rec.Summary))
	if !ecdsa.VerifyASN1(key.Public, digest[:], signature) {
		return SCAProof{}, plxerr.New(plxerr.SCASignatureInvalid, "the signature does not verify with the device's SCA key")
	}
	sum := sha256.Sum256(rec.Summary)
	return SCAProof{ChallengeID: challengeID, SummaryHash: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// summary is a parsed transaction summary.
type summary struct {
	amount, currency, payee, text string
}

// parseSummary parses the summary of a challenge: RFC 8785 canonical JSON,
// an object of string members.
func parseSummary(raw []byte, t Transaction) (summary, error) {
	tree, err := jcs.Parse(raw, jsonDepth)
	if err != nil {
		return summary{}, plxerr.New(plxerr.InvalidFormat, "the transaction summary is not valid JSON")
	}
	if canonical, err := jcs.Marshal(tree); err != nil || !bytes.Equal(canonical, raw) {
		return summary{}, plxerr.New(plxerr.InvalidFormat, "the transaction summary is not in RFC 8785 canonical form")
	}
	obj, ok := tree.(map[string]any)
	if !ok {
		return summary{}, plxerr.New(plxerr.InvalidFormat, "the transaction summary is a JSON object")
	}
	want := 3
	if t.Summary != "" {
		want = 4
	}
	var out summary
	for name, dst := range map[string]*string{"amount": &out.amount, "currency": &out.currency, "payee": &out.payee} {
		v, ok := obj[name].(string)
		if !ok || v == "" {
			return summary{}, plxerr.New(plxerr.InvalidFormat, "the transaction summary needs the string %q", name)
		}
		*dst = v
	}
	if t.Summary != "" {
		v, ok := obj["summary"].(string)
		if !ok || v == "" {
			return summary{}, plxerr.New(plxerr.InvalidFormat, "the transaction summary needs the string \"summary\"")
		}
		out.text = v
	}
	if len(obj) != want {
		return summary{}, plxerr.New(plxerr.InvalidFormat, "the transaction summary has members the operation does not map")
	}
	if _, err := decimal.Parse(out.amount); err != nil {
		return summary{}, plxerr.New(plxerr.InvalidFormat, "the amount of the transaction summary is not a decimal number")
	}
	return out, nil
}

// checkLinking is dynamic linking (SEC-028): the fields the operation's
// mapping finds in the request body must equal what the user confirmed.
// The amount is compared as a decimal number, so 125.5 is 125.50; the
// others must be equal as strings.
func checkLinking(body []byte, t Transaction, signed []byte) error {
	mismatch := func(what string) error {
		return plxerr.New(plxerr.SCADynamicLinkingMismatch, "the %s of the request is not what the user confirmed", what)
	}
	confirmed, err := parseSummary(signed, t)
	if err != nil {
		return mismatch("transaction summary")
	}
	tree, err := jcs.Parse(body, jsonDepth)
	if err != nil {
		return plxerr.New(plxerr.SCADynamicLinkingMismatch, "the body of a transaction is strict JSON without duplicate members")
	}
	amount, err := pointed(tree, t.Amount)
	if err != nil {
		return mismatch("amount")
	}
	signedAmount, err := decimal.Parse(confirmed.amount)
	if err != nil {
		return mismatch("amount")
	}
	sent, err := decimal.Parse(amount)
	if err != nil || sent.Cmp(signedAmount) != 0 {
		return mismatch("amount")
	}
	for _, f := range []struct{ name, pointer, signed string }{
		{"currency", t.Currency, confirmed.currency},
		{"payee", t.Payee, confirmed.payee},
		{"summary", t.Summary, confirmed.text},
	} {
		if f.pointer == "" {
			continue
		}
		got, err := pointed(tree, f.pointer)
		if err != nil || subtle.ConstantTimeCompare([]byte(got), []byte(f.signed)) != 1 {
			return mismatch(f.name)
		}
	}
	return nil
}

// pointed resolves a JSON Pointer (RFC 6901) in a tree parsed by jcs.Parse
// and returns the value as the text a person would compare: a string as
// it is, a number as written.
func pointed(tree any, pointer string) (string, error) {
	cur := tree
	if pointer != "" {
		for _, token := range strings.Split(pointer[1:], "/") {
			token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
			switch c := cur.(type) {
			case map[string]any:
				v, ok := c[token]
				if !ok {
					return "", errors.New("gateway: the pointer finds nothing")
				}
				cur = v
			case []any:
				i, err := strconv.Atoi(token)
				if err != nil || i < 0 || i >= len(c) || (len(token) > 1 && token[0] == '0') {
					return "", errors.New("gateway: the pointer finds nothing")
				}
				cur = c[i]
			default:
				return "", errors.New("gateway: the pointer finds nothing")
			}
		}
	}
	switch v := cur.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	}
	return "", errors.New("gateway: the pointer finds neither a string nor a number")
}
