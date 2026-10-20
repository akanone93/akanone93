// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// ParseTargeting reads the targeting of a new rollout: an optional rule as
// JSON, and segment documents (segments/<key>.segment.json) whose rules all
// must hold. The result is validated (PLX-3152).
func ParseTargeting(rule []byte, segments [][]byte) (Targeting, error) {
	var t Targeting
	if len(bytes.TrimSpace(rule)) > 0 {
		var r schema.SegmentRule
		if err := decodeOne(rule, &r); err != nil {
			return Targeting{}, plxerr.Wrap(plxerr.TargetingRuleInvalid, err, "targeting rule /rule: not a rule")
		}
		t.Rule = &r
	}
	for i, raw := range segments {
		var doc schema.SegmentDocument
		if err := decodeOne(raw, &doc); err != nil {
			return Targeting{}, plxerr.Wrap(plxerr.TargetingRuleInvalid, err, "targeting rule /segments/%d: not a segment document", i)
		}
		if doc.Kind != "segment" || doc.Key == "" {
			return Targeting{}, plxerr.New(plxerr.TargetingRuleInvalid, "targeting rule /segments/%d: a segment document has kind \"segment\" and a key", i)
		}
		if slices.ContainsFunc(t.Segments, func(s Segment) bool { return s.Key == doc.Key }) {
			return Targeting{}, plxerr.New(plxerr.TargetingRuleInvalid, "targeting rule /segments/%d: the segment %q is named twice", i, doc.Key)
		}
		t.Segments = append(t.Segments, Segment{Key: doc.Key, Rule: doc.Rule})
	}
	if err := t.Validate(); err != nil {
		return Targeting{}, err
	}
	return t, nil
}

// decodeOne decodes one JSON value into v and refuses
// trailing data.
func decodeOne(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("rollout: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("rollout: trailing data after the document")
	}
	return nil
}

// parseDevices reads device identifiers, refusing a malformed one and a device
// on both lists.
func parseDevices(allow, deny []string) (a, d []string, err error) {
	for _, list := range [][]string{allow, deny} {
		for _, id := range list {
			if _, err := storage.UUID(id); err != nil {
				return nil, nil, plxerr.New(plxerr.InvalidFormat, "%q is not a device identifier", id)
			}
		}
	}
	for _, id := range allow {
		if slices.Contains(deny, id) {
			return nil, nil, plxerr.New(plxerr.InvalidFormat, "device %s is on the allow list and the deny list", id)
		}
	}
	return slices.Clone(allow), slices.Clone(deny), nil
}
