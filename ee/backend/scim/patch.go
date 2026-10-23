// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// The three operations of RFC 7644 §3.5.2, as clients spell them in any case.
const (
	opAdd     = "add"
	opReplace = "replace"
	opRemove  = "remove"
)

// patchOp is one operation of a PATCH request.
type patchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// readPatch reads the operations of a PATCH request.
func readPatch(r *http.Request) ([]patchOp, error) {
	var req struct {
		Schemas    []string  `json:"schemas"`
		Operations []patchOp `json:"Operations"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if !hasSchema(req.Schemas, schemaPatchOp) {
		return nil, reject(http.StatusBadRequest, "invalidSyntax", "schemas must name "+schemaPatchOp)
	}
	if len(req.Operations) == 0 {
		return nil, reject(http.StatusBadRequest, "invalidSyntax", "Operations must hold at least one operation")
	}
	for i := range req.Operations {
		op := strings.ToLower(req.Operations[i].Op)
		if op != opAdd && op != opReplace && op != opRemove {
			return nil, reject(http.StatusBadRequest, "invalidSyntax", fmt.Sprintf("operation %d: op must be add, replace or remove", i+1))
		}
		req.Operations[i].Op = op
	}
	return req.Operations, nil
}

// attrPath is a PATCH path: an attribute, optionally filtered by the value of
// a member, optionally followed by a sub-attribute.
type attrPath struct {
	// attr is the attribute, lower-cased, with a sub-attribute after a dot.
	attr string
	// member is the value members[value eq "member"] selects; hasMember tells
	// that the path has such a filter.
	member     string
	hasMember  bool
	filterAttr string
}

// base returns the attribute without its sub-attribute.
func (p attrPath) base() string {
	base, _, _ := strings.Cut(p.attr, ".")
	return base
}

// parsePath reads a PATCH path: attr, attr.sub, attr[value eq "x"] or
// attr[...].sub.
func parsePath(path string) (attrPath, error) {
	path = stripSchema(strings.TrimSpace(path))
	open := strings.IndexByte(path, '[')
	if open < 0 {
		return attrPath{attr: strings.ToLower(path)}, nil
	}
	closing := strings.LastIndexByte(path, ']')
	if closing < open {
		return attrPath{}, reject(http.StatusBadRequest, "invalidPath", "the path has an unclosed filter")
	}
	name, value, err := parseEquality(path[open+1 : closing])
	if err != nil {
		return attrPath{}, reject(http.StatusBadRequest, "invalidPath", "the path filter must be: attribute eq \"value\"")
	}
	return attrPath{attr: strings.ToLower(path[:open] + path[closing+1:]), hasMember: true, member: value, filterAttr: name}, nil
}

// objectOps expands an operation with no path, whose value is an object of
// attributes, into one operation per attribute in a fixed order.
func objectOps(op patchOp) ([]patchOp, error) {
	if op.Op == opRemove {
		return nil, reject(http.StatusBadRequest, "noTarget", "remove needs a path")
	}
	var attrs map[string]json.RawMessage
	if err := json.Unmarshal(op.Value, &attrs); err != nil || attrs == nil {
		return nil, reject(http.StatusBadRequest, "invalidValue", "an operation with no path takes an object of attributes")
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]patchOp, 0, len(keys))
	for _, k := range keys {
		out = append(out, patchOp{Op: op.Op, Path: k, Value: attrs[k]})
	}
	return out, nil
}

// flatten returns the operations with the object form expanded.
func flatten(ops []patchOp) ([]patchOp, error) {
	var out []patchOp
	for _, op := range ops {
		if op.Path != "" {
			out = append(out, op)
			continue
		}
		expanded, err := objectOps(op)
		if err != nil {
			return nil, err
		}
		out = append(out, expanded...)
	}
	return out, nil
}

// flexBool is a boolean that clients also send as the string "True" or
// "false".
type flexBool bool

// UnmarshalJSON reads a JSON boolean or its string spelling.
func (b *flexBool) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		switch strings.ToLower(s) {
		case "true":
			*b = true
			return nil
		case "false":
			*b = false
			return nil
		}
		return fmt.Errorf("%q is not a boolean", s)
	}
	var v bool
	if err := json.Unmarshal(data, &v); err != nil {
		return err //nolint:wrapcheck // reported as an invalid value by the caller
	}
	*b = flexBool(v)
	return nil
}

// stringValue reads a JSON string value.
func stringValue(raw json.RawMessage, what string) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", invalid(what + " must be a string")
	}
	return s, nil
}

// boolValue reads a boolean value, sent as a boolean or a string.
func boolValue(raw json.RawMessage, what string) (bool, error) {
	var b flexBool
	if err := json.Unmarshal(bytes.TrimSpace(raw), &b); err != nil {
		return false, invalid(what + " must be true or false")
	}
	return bool(b), nil
}

// ignoredUserAttributes are the attributes of the core User schema that Plux
// does not keep. A client may send them; they change nothing. The ones that
// the server owns (id, meta, groups) are read-only, which the specification
// says to ignore as well, and a password is never taken.
var ignoredUserAttributes = []string{
	"id", "meta", "groups", "schemas", "name", "nickname", "profileurl", "title", "usertype", "preferredlanguage",
	"locale", "timezone", "emails", "phonenumbers", "ims", "photos", "addresses", "entitlements", "roles",
	"x509certificates", "password",
}

// applyUserOps applies PATCH operations to the state a user has.
func applyUserOps(spec *identity.UserSpec, ops []patchOp) error {
	flat, err := flatten(ops)
	if err != nil {
		return err
	}
	for _, op := range flat {
		p, err := parsePath(op.Path)
		if err != nil {
			return err
		}
		if err := applyUserOp(spec, op, p); err != nil {
			return err
		}
	}
	return nil
}

// applyUserOp applies one operation to a user.
func applyUserOp(spec *identity.UserSpec, op patchOp, p attrPath) error {
	if slices.Contains(ignoredUserAttributes, p.base()) {
		return nil
	}
	if p.attr != p.base() || p.hasMember {
		return reject(http.StatusBadRequest, "invalidPath", "the path "+op.Path+" is not supported")
	}
	switch p.attr {
	case "username":
		return setRequired(&spec.UserName, op, "userName")
	case "displayname":
		return setOptional(&spec.DisplayName, op, "displayName")
	case "externalid":
		return setOptional(&spec.ExternalID, op, "externalId")
	case "active":
		if op.Op == opRemove {
			return reject(http.StatusBadRequest, "mutability", "active cannot be removed")
		}
		active, err := boolValue(op.Value, "active")
		spec.Active = active
		return err
	default:
		return reject(http.StatusBadRequest, "invalidPath", "the path "+op.Path+" is not supported")
	}
}

// setRequired sets an attribute that cannot be removed or emptied.
func setRequired(dst *string, op patchOp, what string) error {
	if op.Op == opRemove {
		return reject(http.StatusBadRequest, "mutability", what+" cannot be removed")
	}
	v, err := stringValue(op.Value, what)
	if err != nil {
		return err
	}
	if v == "" {
		return invalid(what + " must not be empty")
	}
	*dst = v
	return nil
}

// setOptional sets an attribute, or clears it on remove.
func setOptional(dst *string, op patchOp, what string) error {
	if op.Op == opRemove {
		*dst = ""
		return nil
	}
	v, err := stringValue(op.Value, what)
	if err != nil {
		return err
	}
	*dst = v
	return nil
}

// applyGroupOps applies PATCH operations to the state a group has.
func applyGroupOps(spec *identity.GroupSpec, ops []patchOp) error {
	flat, err := flatten(ops)
	if err != nil {
		return err
	}
	for _, op := range flat {
		p, err := parsePath(op.Path)
		if err != nil {
			return err
		}
		if err := applyGroupOp(spec, op, p); err != nil {
			return err
		}
	}
	return nil
}

// applyGroupOp applies one operation to a group.
func applyGroupOp(spec *identity.GroupSpec, op patchOp, p attrPath) error {
	switch {
	case slices.Contains([]string{"id", "meta", "schemas"}, p.base()):
		return nil
	case p.base() == "members":
		return applyMembers(spec, op, p)
	case p.attr == "displayname" && !p.hasMember:
		return setRequired(&spec.DisplayName, op, "displayName")
	case p.attr == "externalid" && !p.hasMember:
		return setOptional(&spec.ExternalID, op, "externalId")
	default:
		return reject(http.StatusBadRequest, "invalidPath", "the path "+op.Path+" is not supported")
	}
}

// applyMembers applies an operation on a group's members: add appends, replace
// sets the list, remove takes out those the path or the value names, or all.
func applyMembers(spec *identity.GroupSpec, op patchOp, p attrPath) error {
	if p.attr != "members" {
		return reject(http.StatusBadRequest, "invalidPath", "the path "+op.Path+" is not supported")
	}
	if p.hasMember && !strings.EqualFold(p.filterAttr, "value") {
		return reject(http.StatusBadRequest, "invalidPath", "a path filter can select a member only by value")
	}
	if op.Op == opRemove && p.hasMember {
		spec.MemberIDs = slices.DeleteFunc(spec.MemberIDs, func(id string) bool { return id == p.member })
		return nil
	}
	if p.hasMember {
		return reject(http.StatusBadRequest, "invalidPath", "only remove can select a member")
	}
	if op.Op == opRemove && len(bytes.TrimSpace(op.Value)) == 0 {
		spec.MemberIDs = nil
		return nil
	}
	ids, err := memberIDs(op.Value)
	if err != nil {
		return err
	}
	switch op.Op {
	case opAdd:
		spec.MemberIDs = appendMissing(spec.MemberIDs, ids)
	case opReplace:
		spec.MemberIDs = appendMissing(nil, ids)
	case opRemove:
		spec.MemberIDs = slices.DeleteFunc(spec.MemberIDs, func(id string) bool { return slices.Contains(ids, id) })
	}
	return nil
}

// appendMissing appends the ids that dst does not hold, once each.
func appendMissing(dst, ids []string) []string {
	for _, id := range ids {
		if !slices.Contains(dst, id) {
			dst = append(dst, id)
		}
	}
	return dst
}

// memberRef is a member as a request names it.
type memberRef struct {
	Value string `json:"value"`
}

// memberIDs reads the user identifiers of a list of members, or of one.
func memberIDs(raw json.RawMessage) ([]string, error) {
	var refs []memberRef
	if err := json.Unmarshal(raw, &refs); err != nil {
		var one memberRef
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, invalid("members must be a list of objects with a value")
		}
		refs = []memberRef{one}
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Value == "" {
			return nil, invalid("a member needs a value")
		}
		ids = append(ids, ref.Value)
	}
	return ids, nil
}
