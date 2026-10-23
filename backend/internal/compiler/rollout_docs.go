// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// reportDoc reports a problem of a rollout document (segment, flag,
// experiment or policy); ValidatePage checks only the edited page, so the
// documents are checked in Compile alone.
func (u *unit) reportDoc(code plxerr.Code, file, ptr, format string, args ...any) {
	if !u.validating {
		u.report(code, file, ptr, format, args...)
	}
}

// docKey reports a document whose key differs from its file name.
func (u *unit) docKey(file, suffix, key string) {
	if want, _ := strings.CutSuffix(path.Base(file), suffix); want != key {
		u.reportDoc(plxerr.InvalidProjectLayout, file, "/key", "the key %q must match the file name %q", key, want)
	}
}

// indexRollout indexes and checks the project's segments, flags,
// experiments and policies (REL-051, ABT-001, ABT-006, GOV-020). It runs
// before any node is built, because a Variant node names an experiment.
func (u *unit) indexRollout() {
	u.experiments, u.segments = map[string]*experiment{}, map[string]bool{}
	p := u.project
	keys := u.newKeys("segment")
	for _, l := range p.Segments {
		if l.Doc == nil {
			continue
		}
		u.claimID(l.Doc.ID, l.Source.File, "/id")
		u.docKey(l.Source.File, ".segment.json", l.Doc.Key)
		keys.claim(l.Doc.Key, l.Source.File, "/key")
		u.segments[l.Doc.Key] = true
		u.checkSegment(l.Doc, l.Source.File)
	}
	u.indexExperiments()
	u.checkFlags()
	pol := u.newKeys("policy")
	for _, l := range p.Policies {
		if l.Doc == nil {
			continue
		}
		u.claimID(l.Doc.ID, l.Source.File, "/id")
		u.docKey(l.Source.File, ".policy.json", l.Doc.Key)
		pol.claim(l.Doc.Key, l.Source.File, "/key")
		u.checkPolicy(l.Doc, l.Source.File)
	}
}

// attrKind is the kind of value a targeting attribute holds.
type attrKind uint8

const (
	attrString attrKind = iota + 1
	attrInteger
	attrNumber
	attrBool
	attrVersion
)

// builtinAttributes are the targeting attributes of REL-051 that every
// device and build has.
var builtinAttributes = map[string]attrKind{
	"build": attrInteger, "runtimeVersion": attrVersion, "os": attrString, "osVersion": attrVersion,
	"deviceClass": attrString, "locale": attrString, "region": attrString, "channel": attrString,
}

// versionPattern is a version: one to three dot-separated numbers.
var versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)

// userAttrKind returns the kind of the user-context attribute name, and
// false when the app declares no such attribute or its type cannot be
// targeted.
func (u *unit) userAttrKind(name string) (attrKind, bool) {
	for _, f := range u.project.App.Doc.UserContext {
		if f.Name != name {
			continue
		}
		switch strings.TrimSuffix(f.Type, "?") {
		case "bool":
			return attrBool, true
		case "int":
			return attrInteger, true
		case "double":
			return attrNumber, true
		case "string":
			return attrString, true
		}
		return 0, false
	}
	return 0, false
}

// checkSegment checks the rule of a segment (PLX-1704).
func (u *unit) checkSegment(doc *schema.SegmentDocument, file string) {
	u.checkRule(&doc.Rule, file, "/rule")
}

// checkRule checks a rule: a group's rules, or a condition's attribute,
// operator and value.
func (u *unit) checkRule(r *schema.SegmentRule, file, ptr string) {
	if len(r.All) > 0 || len(r.Any) > 0 {
		for i := range r.All {
			u.checkRule(&r.All[i], file, ptr+"/all/"+strconv.Itoa(i))
		}
		for i := range r.Any {
			u.checkRule(&r.Any[i], file, ptr+"/any/"+strconv.Itoa(i))
		}
		return
	}
	kind, ok := builtinAttributes[r.Attribute]
	if user, isUser := strings.CutPrefix(r.Attribute, "user."); isUser {
		kind, ok = u.userAttrKind(user)
	}
	if !ok {
		u.reportDoc(plxerr.SegmentRuleInvalid, file, ptr+"/attribute", "%q is not a targeting attribute: use one of the built-in attributes or a user attribute the app declares and that can be compared", r.Attribute)
		return
	}
	ordered := r.Operator == schema.SegmentOperatorLt || r.Operator == schema.SegmentOperatorLte || r.Operator == schema.SegmentOperatorGt || r.Operator == schema.SegmentOperatorGte
	if ordered && (kind == attrString || kind == attrBool) {
		u.reportDoc(plxerr.SegmentRuleInvalid, file, ptr+"/operator", "%q does not order the values of %s", r.Operator, r.Attribute)
		return
	}
	u.checkRuleValue(r, kind, file, ptr+"/value")
}

// checkRuleValue checks that a condition's value, or each element of a
// list for in and notIn, has the kind of its attribute.
func (u *unit) checkRuleValue(r *schema.SegmentRule, kind attrKind, file, ptr string) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(r.Value))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		u.reportDoc(plxerr.SegmentRuleInvalid, file, ptr, "the value is not JSON")
		return
	}
	list, isList := v.([]any)
	set := r.Operator == schema.SegmentOperatorIn || r.Operator == schema.SegmentOperatorNotIn
	switch {
	case set && (!isList || len(list) == 0):
		u.reportDoc(plxerr.SegmentRuleInvalid, file, ptr, "%q takes a non-empty list of values", r.Operator)
	case !set && isList:
		u.reportDoc(plxerr.SegmentRuleInvalid, file, ptr, "%q takes one value, not a list", r.Operator)
	case set:
		for i, e := range list {
			if !attrValueOK(kind, e) {
				u.reportDoc(plxerr.SegmentRuleInvalid, file, ptr+"/"+strconv.Itoa(i), "the value does not suit %s", r.Attribute)
			}
		}
	case !attrValueOK(kind, v):
		u.reportDoc(plxerr.SegmentRuleInvalid, file, ptr, "the value does not suit %s", r.Attribute)
	}
}

// attrValueOK reports whether a decoded JSON value suits an attribute kind.
func attrValueOK(kind attrKind, v any) bool {
	switch x := v.(type) {
	case string:
		return kind == attrString || kind == attrVersion && versionPattern.MatchString(x)
	case bool:
		return kind == attrBool
	case json.Number:
		if kind == attrInteger {
			_, err := x.Int64()
			return err == nil
		}
		return kind == attrNumber
	}
	return false
}

// flagTypes are the types of the flag documents (ABT-006).
var flagTypes = []string{"bool", "number", "string", "json"}

// checkFlags checks the flag documents (PLX-1701).
func (u *unit) checkFlags() {
	names := u.newKeys("flag")
	for i, f := range u.project.App.Doc.Flags {
		names.claim(f.Name, "app.json", plxerr.Pointer("flags", strconv.Itoa(i), "name"))
	}
	for _, l := range u.project.Flags {
		doc, file := l.Doc, ""
		if doc == nil {
			continue
		}
		file = l.Source.File
		u.claimID(doc.ID, file, "/id")
		u.docKey(file, ".flag.json", doc.Key)
		names.claim(doc.Key, file, "/key")
		if !slices.Contains(flagTypes, doc.Type) {
			u.reportDoc(plxerr.UnknownFlagType, file, "/type", "%q is not a flag type: use %s", doc.Type, strings.Join(flagTypes, ", "))
			continue
		}
		u.checkFlagValue(doc, doc.Type, doc.Default, file, "/default")
		for i, sv := range doc.Segments {
			sp := plxerr.Pointer("segments", strconv.Itoa(i))
			if !u.segments[sv.Segment] {
				u.reportDoc(plxerr.UnresolvedReference, file, sp+"/segment", "no segment %q", sv.Segment)
			}
			u.checkFlagValue(doc, doc.Type, sv.Value, file, sp+"/value")
		}
		if doc.Type != "json" && len(doc.Schema) > 0 {
			u.reportDoc(plxerr.UnknownProperty, file, "/schema", "only a json flag has a schema")
		}
	}
}

// checkFlagValue checks a flag's default or segment value against its type.
func (u *unit) checkFlagValue(doc *schema.FlagDocument, typ string, raw json.RawMessage, file, ptr string) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		u.reportDoc(plxerr.ValueTypeMismatch, file, ptr, "not a JSON value")
		return
	}
	ok := true
	switch typ {
	case "bool":
		_, ok = v.(bool)
	case "number":
		_, ok = v.(json.Number)
	case "string":
		_, ok = v.(string)
	}
	if !ok {
		u.reportDoc(plxerr.ValueTypeMismatch, file, ptr, "the value of the %s flag %q must be a %s", typ, doc.Key, typ)
	}
}

// checkPolicy checks what the policy schema cannot say (PLX-1700): the
// environments exist, risk attributes apply to the matched resources and
// the approval can be given.
func (u *unit) checkPolicy(doc *schema.PolicyDocument, file string) {
	envs := map[string]bool{}
	for _, e := range u.project.App.Doc.Environments {
		envs[e.Key] = true
	}
	for i, e := range doc.Match.Environments {
		if !envs[e] {
			u.reportDoc(plxerr.PolicyDocumentInvalid, file, plxerr.Pointer("match", "environments", strconv.Itoa(i)), "the app has no environment %q", e)
		}
	}
	if len(doc.Match.Risk) > 0 && len(doc.Match.Resources) > 0 &&
		!slices.Contains(doc.Match.Resources, schema.PolicyResourceRelease) && !slices.Contains(doc.Match.Resources, schema.PolicyResourceExperiment) {
		u.reportDoc(plxerr.PolicyDocumentInvalid, file, "/match/risk", "risk attributes are computed for releases and experiments, which this policy does not match")
	}
	req := doc.Require
	if len(req.Users) > 0 && len(req.Roles) == 0 && req.Approvers > int64(len(req.Users)) {
		u.reportDoc(plxerr.PolicyDocumentInvalid, file, "/require/approvers", "%d approvals are required, but only %d approvers are named", req.Approvers, len(req.Users))
	}
	if req.DistinctTeams != nil && *req.DistinctTeams && req.Approvers < 2 {
		u.reportDoc(plxerr.PolicyDocumentInvalid, file, "/require/distinctTeams", "approvals from distinct teams need at least two approvers")
	}
	if req.Stages == schema.PolicyStagesSequential && req.Approvers > 1 && len(req.Roles)+len(req.Users) == 0 {
		u.reportDoc(plxerr.PolicyDocumentInvalid, file, "/require/stages", "sequential stages follow the order of the roles and approvers listed, and none is listed")
	}
}
