// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

const (
	// functionScope is what the challenge of a function invocation records
	// as its upstream; no registered upstream can be named so, because
	// upstream names carry no colon.
	functionScope = "plux:function"

	// The members the server adds to the summary of a function
	// invocation. A summary the device sends must not name them.
	memberFunction = "function"
	memberVersion  = "version"
	memberInput    = "inputSha256"
)

// FunctionTransaction names the invocation of a server function that the
// user confirms (FN-020, SEC-027).
type FunctionTransaction struct {
	Name    string
	Version int32
}

// operation is how the challenge of a function invocation records it.
func (t FunctionTransaction) operation() string {
	return t.Name + "@" + strconv.FormatInt(int64(t.Version), 10)
}

// CreateInvocation issues the challenge of a function invocation, a
// transaction whose operation is the function and its version and whose
// bound body is the input (SEC-027, SEC-028). summary is what the device
// shows the user: a canonical JSON object of strings (RFC 8785) that must
// not name the members the server adds. The server adds the function, the
// version and the SHA-256 of the input, so that the signature over the
// returned summary confirms exactly that input.
func (s *SCA) CreateInvocation(ctx context.Context, d device.Identity, t FunctionTransaction, inputSHA256 []byte, summary []byte, lifetime time.Duration) (Challenge, error) {
	if len(inputSHA256) != sha256.Size {
		return Challenge{}, plxerr.New(plxerr.InvalidFormat, "the input hash is a SHA-256, %d bytes", sha256.Size)
	}
	if _, err := s.Keys.SCAKey(ctx, d); err != nil {
		if code, _ := plxerr.CodeOf(err); code == plxerr.ResourceNotFound {
			return Challenge{}, plxerr.New(plxerr.PreconditionFailed, "the device has no SCA key; enrol one first")
		}
		return Challenge{}, err //nolint:wrapcheck // a domain error
	}
	merged, err := mergeInvocation(summary, t, inputSHA256)
	if err != nil {
		return Challenge{}, err
	}
	return s.issue(ctx, d, functionScope, t.operation(), merged, nil, lifetime)
}

// VerifyInvocation accepts the confirmation of a function invocation: the
// challenge must be one this server issued to this device for this
// function and version, unexpired and unused; the SHA-256 of the input of
// the call must equal the one the user confirmed (dynamic linking,
// SEC-028); and the signature must verify with the device's SCA key. It
// spends the challenge first, so a second call with it fails whatever the
// first did.
func (s *SCA) VerifyInvocation(ctx context.Context, d device.Identity, t FunctionTransaction, input []byte, challengeID string, signature []byte, lifetime time.Duration) error {
	rec, err := s.spend(ctx, d, challengeID, functionScope, t.operation(), nil, lifetime)
	if err != nil {
		return err
	}
	confirmed, err := confirmedInput(rec.Summary)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(input)
	if subtle.ConstantTimeCompare([]byte(confirmed), []byte(base64.RawURLEncoding.EncodeToString(sum[:]))) != 1 {
		return plxerr.New(plxerr.SCADynamicLinkingMismatch, "the input of the call is not what the user confirmed")
	}
	return s.checkSignature(ctx, d, rec, signature)
}

// mergeInvocation puts the function, its version and the input hash into
// the summary a device sent. A device that names one of them is refused,
// so that it cannot show the user one invocation and bind another.
func mergeInvocation(raw []byte, t FunctionTransaction, inputSHA256 []byte) ([]byte, error) {
	obj, err := parseStringObject(raw)
	if err != nil {
		return nil, err
	}
	for _, member := range []string{memberFunction, memberVersion, memberInput} {
		if _, set := obj[member]; set {
			return nil, plxerr.New(plxerr.InvalidFormat, "the transaction summary of a function names %q, which the server sets", member)
		}
	}
	obj[memberFunction] = t.Name
	obj[memberVersion] = strconv.FormatInt(int64(t.Version), 10)
	obj[memberInput] = base64.RawURLEncoding.EncodeToString(inputSHA256)
	merged, err := jcs.Marshal(obj)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the transaction summary cannot be encoded")
	}
	return merged, nil
}

// parseStringObject parses canonical JSON that is an object of strings.
func parseStringObject(raw []byte) (map[string]any, error) {
	tree, err := jcs.Parse(raw, jsonDepth)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the transaction summary is not valid JSON")
	}
	if canonical, err := jcs.Marshal(tree); err != nil || string(canonical) != string(raw) {
		return nil, plxerr.New(plxerr.InvalidFormat, "the transaction summary is not in RFC 8785 canonical form")
	}
	obj, ok := tree.(map[string]any)
	if !ok {
		return nil, plxerr.New(plxerr.InvalidFormat, "the transaction summary is a JSON object")
	}
	for name, v := range obj {
		if _, ok := v.(string); !ok {
			return nil, plxerr.New(plxerr.InvalidFormat, "the member %q of the transaction summary is a string", name)
		}
	}
	return obj, nil
}

// confirmedInput is the input hash a stored summary carries.
func confirmedInput(summary []byte) (string, error) {
	tree, err := jcs.Parse(summary, jsonDepth)
	if err != nil {
		return "", plxerr.New(plxerr.SCAChallengeInvalid, "the challenge holds no summary of a function")
	}
	obj, _ := tree.(map[string]any)
	hash, ok := obj[memberInput].(string)
	if !ok {
		return "", plxerr.New(plxerr.SCAChallengeInvalid, "the challenge holds no summary of a function")
	}
	return hash, nil
}
