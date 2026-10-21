// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

// The actions the engine's own callers name. A policy may match any action
// string; these are the ones the stand-ins it replaces use.
const (
	// ActionPromote promotes a release to a channel of an environment.
	ActionPromote = "promote"
	// ActionDeploy deploys, promotes or re-points a function version.
	ActionDeploy = "deploy"
	// ActionSet changes a security configuration.
	ActionSet = "set"
	// ActionEnableDeviceFunctions turns device-placed functions on.
	ActionEnableDeviceFunctions = "enableDeviceFunctions"
	// ActionWaive adds or changes accessibility waivers.
	ActionWaive = "waive"
	// ActionApproveGoldens promotes golden images to baselines.
	ActionApproveGoldens = "approveGoldens"
	// ActionRaiseLimit raises an organisation-level limit.
	ActionRaiseLimit = "raise"
	// ActionSetCapabilities changes an app's approved capability set.
	ActionSetCapabilities = "setCapabilities"
	// ActionChangePolicy writes or withdraws a policy.
	ActionChangePolicy = "change"
	// ActionLaunch starts an experiment (ABT-007).
	ActionLaunch = "launch"
)

// maxDepth bounds the nesting of the JSON the engine reads or hashes.
const maxDepth = 32

// Environment is the environment a change is made in.
type Environment struct {
	// ID is the environment's identifier; empty for a change that belongs to
	// the app or the organisation as a whole.
	ID string
	// Key is the environment's key, which a policy's environments list names.
	Key string
	// Production is whether the environment is a production one.
	Production bool
}

// Artifact is a change a policy may guard: what it is, the content it
// changes to, and who is making it.
type Artifact struct {
	// Resource is the kind of artifact.
	Resource schema.PolicyResource
	// OrganizationID is the organisation the change belongs to.
	OrganizationID string
	// AppID is the app the change belongs to; empty for a change that belongs
	// to the organisation.
	AppID string
	// Environment is where the change is made; the zero value for a change
	// that is not in one environment.
	Environment Environment
	// Action is what is done, for example ActionPromote.
	Action string
	// Subject names what changes, stably across edits: "release:12",
	// "function:orders.create@build:<id>", "limit:<key>". A request is
	// about one subject, one action and one content hash.
	Subject string
	// Hash is the SHA-256 of the content the subject changes to, in hex.
	// Build it with HashOf or HashJSON.
	Hash string
	// Summary is a line for the approver's inbox.
	Summary string
	// Risk lists the risk attributes the change carries (GOV-020).
	Risk []schema.PolicyRisk
	// Payload is the proposed content as JSON, when the approver has to see
	// it because nothing else holds it yet; nil otherwise.
	Payload json.RawMessage
	// Author is the user who wrote the content, when that is not the
	// requester; the author can never approve.
	Author string
	// Requester is who makes the change.
	Requester audit.Actor
	// Profile is the security profile that decides the default policies, when
	// the caller knows it; empty has the engine ask for the environment's.
	Profile string
}

// validate refuses an artifact the engine cannot bind an approval to.
func (a Artifact) validate() error {
	switch {
	case !a.Resource.Valid():
		return fmt.Errorf("approval: %q is not a resource a policy can guard", a.Resource)
	case a.OrganizationID == "":
		return fmt.Errorf("approval: an artifact names its organisation")
	case a.Subject == "" || a.Action == "":
		return fmt.Errorf("approval: an artifact names its subject and its action")
	case len(a.Hash) != sha256.Size*2:
		return fmt.Errorf("approval: the content hash of %s is not a SHA-256 in hex", a.Subject)
	case a.Requester.ID == "":
		return fmt.Errorf("approval: an artifact names who makes the change")
	}
	return nil
}

// HashOf returns the hash that binds an approval to content made of the given
// parts. Every part is length-prefixed, so no content can be moved between
// parts without changing the hash.
func HashOf(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(strconv.Itoa(len(p))))
		h.Write([]byte{':'})
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// HashJSON returns the hash of a value's canonical JSON, so that the same
// content hashes the same wherever it is built (RFC 8785).
func HashJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("approval: encode the content to hash: %w", err)
	}
	canonical, err := jcs.Canonicalize(raw, maxDepth)
	if err != nil {
		return "", fmt.Errorf("approval: canonicalise the content to hash: %w", err)
	}
	return HashOf(canonical), nil
}

// CanonicalPayload returns the canonical form of a JSON payload, or nil for
// none.
func CanonicalPayload(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out, err := jcs.Canonicalize(raw, maxDepth)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.InvalidFormat, err, "the proposed content is not valid JSON")
	}
	return out, nil
}
