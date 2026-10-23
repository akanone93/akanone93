// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// maxRuleDepth bounds how deeply a rule nests all and any groups, so that
// evaluating one on the manifest path costs a bounded amount of work.
const maxRuleDepth = 8

// Attributes are the facts about a device's app and platform that a rule can
// test (REL-051). An attribute the device did not send is empty, and no
// condition on an empty attribute holds.
type Attributes struct {
	// Build is the host app's build number.
	Build string
	// RuntimeVersion is the version of the Plux runtime the app embeds.
	RuntimeVersion string
	// OS is the platform, such as "android"; OSVersion its version.
	OS, OSVersion string
	DeviceClass   string
	// Locale is the device's locale tag and Region its region setting; the
	// region is the device's own setting, never derived from an address.
	Locale, Region string
	// Channel is the channel the device follows.
	Channel string
}

// Device is what the server knows of a device that asks for its manifest.
type Device struct {
	// ID is the device identifier.
	ID string
	// UserHash identifies the signed-in user, hashed by the device; empty if
	// the host app set no user.
	UserHash string
	Attributes
	// UserAttributes maps an attribute of the user context to the value
	// HashUserAttribute gives; plain values never leave the device.
	UserAttributes map[string]string
}

// HashUserAttribute is the form in which a device sends, and a rule compares,
// the value of a user attribute: the hex SHA-256 of the name, a zero byte and
// the value. Including the name keeps equal values of different attributes
// apart.
func HashUserAttribute(name, value string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + value))
	return hex.EncodeToString(sum[:])
}

// Segment is a named audience, the rule of a segment document (REL-051).
type Segment struct {
	Key  string             `json:"key"`
	Rule schema.SegmentRule `json:"rule"`
}

// Targeting selects the devices a rollout may reach: the rule and every
// segment must hold. It is a snapshot taken when the rollout is created, so
// editing a segment later does not move devices in or out of a running one.
type Targeting struct {
	Rule     *schema.SegmentRule `json:"rule,omitempty"`
	Segments []Segment           `json:"segments,omitempty"`
}

// Matches reports whether a device satisfies the rule and every segment; a
// targeting without either matches every device.
func (t Targeting) Matches(d Device) bool {
	if t.Rule != nil && !Evaluate(*t.Rule, d) {
		return false
	}
	for _, s := range t.Segments {
		if !Evaluate(s.Rule, d) {
			return false
		}
	}
	return true
}

// JSON renders the rule, and each segment as a minimal segment document, in
// the form ParseTargeting reads back.
func (t Targeting) JSON() (rule []byte, segments [][]byte, err error) {
	if t.Rule != nil {
		if rule, err = json.Marshal(t.Rule); err != nil {
			return nil, nil, fmt.Errorf("rollout: %w", err)
		}
	}
	for _, s := range t.Segments {
		doc, err := json.Marshal(struct {
			Kind string             `json:"kind"`
			Key  string             `json:"key"`
			Rule schema.SegmentRule `json:"rule"`
		}{"segment", s.Key, s.Rule})
		if err != nil {
			return nil, nil, fmt.Errorf("rollout: %w", err)
		}
		segments = append(segments, doc)
	}
	return rule, segments, nil
}

// Validate checks the rule and the segments (PLX-3152).
func (t Targeting) Validate() error {
	if t.Rule != nil {
		if err := ValidateRule(*t.Rule, "/rule"); err != nil {
			return err
		}
	}
	for i, s := range t.Segments {
		if err := ValidateRule(s.Rule, "/segments/"+strconv.Itoa(i)+"/rule"); err != nil {
			return err
		}
	}
	return nil
}

// attrKind is how an attribute's values compare.
type attrKind uint8

const (
	kindString attrKind = iota + 1
	kindInteger
	kindVersion
	kindUserHash
)

// builtinKinds are the attributes of REL-051 that every device has.
var builtinKinds = map[string]attrKind{
	"build": kindInteger, "runtimeVersion": kindVersion, "os": kindString, "osVersion": kindVersion,
	"deviceClass": kindString, "locale": kindString, "region": kindString, "channel": kindString,
}

var (
	versionPattern  = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
	userNamePattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)
)

// kindOf returns the kind of an attribute and, for a user attribute, its name
// in the user context.
func kindOf(attribute string) (kind attrKind, userName string, ok bool) {
	if name, isUser := strings.CutPrefix(attribute, "user."); isUser {
		return kindUserHash, name, userNamePattern.MatchString(name)
	}
	kind, ok = builtinKinds[attribute]
	return kind, "", ok
}

// have returns a device's value of an attribute, and whether it has one.
func (d Device) have(attribute string) (string, bool) {
	var v string
	switch attribute {
	case "build":
		v = d.Build
	case "runtimeVersion":
		v = d.RuntimeVersion
	case "os":
		v = d.OS
	case "osVersion":
		v = d.OSVersion
	case "deviceClass":
		v = d.DeviceClass
	case "locale":
		v = d.Locale
	case "region":
		v = d.Region
	case "channel":
		v = d.Channel
	default:
		name, _ := strings.CutPrefix(attribute, "user.")
		v = d.UserAttributes[name]
	}
	return v, v != ""
}

// Evaluate reports whether a device satisfies a rule: a group holds when all,
// or any, of its rules hold; a condition holds when the device has the
// attribute and it compares as the operator says. A rule that does not
// validate never holds; rollouts validate their rules when they are created.
func Evaluate(r schema.SegmentRule, d Device) bool { return evaluate(r, d, 0) }

func evaluate(r schema.SegmentRule, d Device, depth int) bool {
	if depth > maxRuleDepth {
		return false
	}
	switch {
	case len(r.All) > 0:
		for _, sub := range r.All {
			if !evaluate(sub, d, depth+1) {
				return false
			}
		}
		return true
	case len(r.Any) > 0:
		for _, sub := range r.Any {
			if evaluate(sub, d, depth+1) {
				return true
			}
		}
		return false
	}
	return condition(r, d)
}

// condition evaluates one attribute, operator and value.
func condition(r schema.SegmentRule, d Device) bool {
	kind, userName, ok := kindOf(r.Attribute)
	if !ok {
		return false
	}
	have, present := d.have(r.Attribute)
	if !present {
		return false
	}
	values, err := literals(r)
	if err != nil {
		return false
	}
	if kind == kindUserHash {
		for i, v := range values {
			values[i] = HashUserAttribute(userName, v)
		}
	}
	switch r.Operator {
	case schema.SegmentOperatorEq:
		return len(values) == 1 && equal(kind, have, values[0])
	case schema.SegmentOperatorNeq:
		return len(values) == 1 && parses(kind, have, values[0]) && !equal(kind, have, values[0])
	case schema.SegmentOperatorIn:
		return anyEqual(kind, have, values)
	case schema.SegmentOperatorNotIn:
		return allParse(kind, have, values) && !anyEqual(kind, have, values)
	case schema.SegmentOperatorLt, schema.SegmentOperatorLte, schema.SegmentOperatorGt, schema.SegmentOperatorGte:
		return len(values) == 1 && ordered(kind, r.Operator, have, values[0])
	}
	return false
}

// literals decodes a condition's value as the canonical strings of its
// scalars: one for most operators, the elements for in and notIn.
func literals(r schema.SegmentRule) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(r.Value))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("rollout: a rule value: %w", err)
	}
	list, isList := v.([]any)
	if r.Operator == schema.SegmentOperatorIn || r.Operator == schema.SegmentOperatorNotIn {
		if !isList || len(list) == 0 {
			return nil, fmt.Errorf("rollout: %q takes a non-empty list", r.Operator)
		}
	} else {
		if isList {
			return nil, fmt.Errorf("rollout: %q takes one value", r.Operator)
		}
		list = []any{v}
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		s, ok := scalar(e)
		if !ok {
			return nil, errors.New("rollout: a rule value is not a scalar")
		}
		out = append(out, s)
	}
	return out, nil
}

// scalar renders a decoded JSON scalar as the string devices send it as.
func scalar(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case bool:
		return strconv.FormatBool(x), true
	}
	return "", false
}

// equal compares two values of a kind; values that do not parse are unequal.
func equal(kind attrKind, a, b string) bool {
	switch kind {
	case kindInteger, kindVersion:
		c, ok := compareNumbers(kind, a, b)
		return ok && c == 0
	case kindUserHash:
		return a == b
	}
	return strings.EqualFold(a, b)
}

// parses reports whether two values of a kind can be compared at all, so
// that "not equal" is not claimed of a value that is merely malformed.
func parses(kind attrKind, a, b string) bool {
	if kind == kindInteger || kind == kindVersion {
		_, ok := compareNumbers(kind, a, b)
		return ok
	}
	return true
}

func allParse(kind attrKind, have string, values []string) bool {
	for _, v := range values {
		if !parses(kind, have, v) {
			return false
		}
	}
	return true
}

func anyEqual(kind attrKind, have string, values []string) bool {
	for _, v := range values {
		if equal(kind, have, v) {
			return true
		}
	}
	return false
}

// ordered applies an ordering operator to numbers and versions.
func ordered(kind attrKind, op schema.SegmentOperator, a, b string) bool {
	c, ok := compareNumbers(kind, a, b)
	if !ok {
		return false
	}
	switch op {
	case schema.SegmentOperatorLt:
		return c < 0
	case schema.SegmentOperatorLte:
		return c <= 0
	case schema.SegmentOperatorGt:
		return c > 0
	case schema.SegmentOperatorGte:
		return c >= 0
	}
	return false
}

// compareNumbers orders two integers or two dotted versions; ok is false when
// either does not parse. A version with fewer components is padded with zeros,
// so 1.2 equals 1.2.0.
func compareNumbers(kind attrKind, a, b string) (int, bool) {
	switch kind {
	case kindInteger:
		x, errA := strconv.ParseInt(a, 10, 64)
		y, errB := strconv.ParseInt(b, 10, 64)
		if errA != nil || errB != nil {
			return 0, false
		}
		return cmpInt(x, y), true
	case kindVersion:
		return compareVersions(a, b)
	}
	return 0, false
}

func cmpInt(x, y int64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// compareVersions orders two versions of one to three numeric components.
func compareVersions(a, b string) (int, bool) {
	if !versionPattern.MatchString(a) || !versionPattern.MatchString(b) {
		return 0, false
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range 3 {
		x, y := component(pa, i), component(pb, i)
		if c := cmpInt(x, y); c != 0 {
			return c, true
		}
	}
	return 0, true
}

// component is the i-th component of a version, or zero past its end. The
// pattern admits only digits, so a component too long for int64 is the only
// way to fail, and it saturates.
func component(parts []string, i int) int64 {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.ParseInt(parts[i], 10, 64)
	if err != nil {
		return 1<<63 - 1
	}
	return n
}

// ValidateRule checks a rule the way the compiler checks a segment document,
// with the refusal of PLX-3152 and the JSON pointer of the offending part
// (REL-051). path is the pointer of the rule itself.
func ValidateRule(r schema.SegmentRule, path string) error { return validateRule(r, path, 0) }

func validateRule(r schema.SegmentRule, path string, depth int) error {
	if depth > maxRuleDepth {
		return refuse(path, "rules nest at most %d groups deep", maxRuleDepth)
	}
	group := len(r.All) > 0 || len(r.Any) > 0
	switch {
	case group && (len(r.All) > 0 && len(r.Any) > 0 || r.Attribute != "" || r.Operator != "" || len(r.Value) > 0):
		return refuse(path, "a rule is either a group (all or any) or a condition, not both")
	case group:
		return validateGroup(r, path, depth)
	}
	return validateCondition(r, path)
}

func validateGroup(r schema.SegmentRule, path string, depth int) error {
	if err := validateAll(r.All, path+"/all", depth); err != nil {
		return err
	}
	return validateAll(r.Any, path+"/any", depth)
}

// validateAll checks the rules of one group in order.
func validateAll(rules []schema.SegmentRule, path string, depth int) error {
	for i, sub := range rules {
		if err := validateRule(sub, path+"/"+strconv.Itoa(i), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateCondition(r schema.SegmentRule, path string) error {
	kind, _, ok := kindOf(r.Attribute)
	if !ok {
		return refuse(path+"/attribute", "%q is not a targeting attribute: use build, runtimeVersion, os, osVersion, deviceClass, locale, region, channel or user.<name>", r.Attribute)
	}
	if !r.Operator.Valid() {
		return refuse(path+"/operator", "%q is not an operator", r.Operator)
	}
	isOrder := r.Operator == schema.SegmentOperatorLt || r.Operator == schema.SegmentOperatorLte ||
		r.Operator == schema.SegmentOperatorGt || r.Operator == schema.SegmentOperatorGte
	if isOrder && (kind == kindString || kind == kindUserHash) {
		return refuse(path+"/operator", "%q does not order the values of %s; hashed and textual attributes compare for equality and membership only", r.Operator, r.Attribute)
	}
	values, err := literals(r)
	if err != nil {
		return refuse(path+"/value", "%v", err)
	}
	for _, v := range values {
		if !valueSuits(kind, v) {
			return refuse(path+"/value", "the value %q does not suit %s", v, r.Attribute)
		}
	}
	return nil
}

// valueSuits reports whether a rule's value parses as its attribute's kind.
func valueSuits(kind attrKind, v string) bool {
	switch kind {
	case kindInteger:
		_, err := strconv.ParseInt(v, 10, 64)
		return err == nil
	case kindVersion:
		return versionPattern.MatchString(v)
	}
	return true
}

// refuse builds the PLX-3152 refusal for a part of a rule.
func refuse(path, format string, args ...any) error {
	return plxerr.New(plxerr.TargetingRuleInvalid, "targeting rule %s: %s", path, fmt.Sprintf(format, args...))
}
