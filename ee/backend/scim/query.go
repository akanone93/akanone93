// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// The schema URNs of the resources and messages.
const (
	schemaUser     = "urn:ietf:params:scim:schemas:core:2.0:User"
	schemaGroup    = "urn:ietf:params:scim:schemas:core:2.0:Group"
	schemaList     = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	schemaPatchOp  = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	schemaConfig   = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	schemaResource = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	schemaSchema   = "urn:ietf:params:scim:schemas:core:2.0:Schema"
)

// stripSchema drops the schema URN a path or filter attribute may start
// with, as in urn:ietf:params:scim:schemas:core:2.0:User:userName.
func stripSchema(attr string) string {
	for _, urn := range []string{schemaUser, schemaGroup} {
		if rest, ok := strings.CutPrefix(attr, urn+":"); ok {
			return rest
		}
	}
	return attr
}

// parseFilter reads the one filter the directory can search: attr eq "value".
// attrs are the attributes the resource can be searched by; the attribute is
// matched without regard to case and returned as spelled there.
func parseFilter(filter string, attrs ...string) (identity.ListQuery, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return identity.ListQuery{}, nil
	}
	name, value, err := parseEquality(filter)
	if err != nil {
		return identity.ListQuery{}, reject(http.StatusBadRequest, "invalidFilter", `the filter must be: attribute eq "value"`)
	}
	for _, a := range attrs {
		if strings.EqualFold(a, name) {
			return identity.ListQuery{Attribute: a, Value: value}, nil
		}
	}
	return identity.ListQuery{}, reject(http.StatusBadRequest, "invalidFilter", "the resource cannot be filtered by "+strconv.Quote(name))
}

// errNotEquality reports an expression that is not attribute eq "value".
var errNotEquality = errors.New(`not of the form: attribute eq "value"`)

// parseEquality reads attribute eq "value". The attribute is returned as
// written, without a schema prefix.
func parseEquality(expr string) (name, value string, err error) {
	name, rest, ok := strings.Cut(strings.TrimSpace(expr), " ")
	op, literal, ok2 := strings.Cut(strings.TrimSpace(rest), " ")
	if !ok || !ok2 || !strings.EqualFold(op, "eq") {
		return "", "", errNotEquality
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(literal)), &value); err != nil {
		return "", "", errNotEquality
	}
	return stripSchema(name), value, nil
}

// parseList reads the filter and the pagination parameters of a list request.
// A count of 0 asks for the total only, which is returned as zeroCount.
func (s *server) parseList(r *http.Request, attrs ...string) (q identity.ListQuery, zeroCount bool, err error) {
	params := r.URL.Query()
	if q, err = parseFilter(params.Get("filter"), attrs...); err != nil {
		return identity.ListQuery{}, false, err
	}
	q.StartIndex = 1
	if v := params.Get("startIndex"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return identity.ListQuery{}, false, invalid("startIndex must be an integer")
		}
		q.StartIndex = max(n, 1)
	}
	q.Count = s.dir.Limits().MaxResults
	if v := params.Get("count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return identity.ListQuery{}, false, invalid("count must be an integer")
		}
		q.Count = min(max(n, 0), q.Count)
	}
	if q.Count == 0 {
		q.Count, zeroCount = 1, true
	}
	return q, zeroCount, nil
}

// listResponse is a page of resources (RFC 7644 §3.4.2).
type listResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
	Resources    any      `json:"Resources"`
}

// page builds a list response of resources.
func page(total, start int, resources any, n int) listResponse {
	return listResponse{Schemas: []string{schemaList}, TotalResults: total, StartIndex: start, ItemsPerPage: n, Resources: resources}
}
