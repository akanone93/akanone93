// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// SCIMErrorSchema is the schema URN of a SCIM error response (RFC 7644
// §3.12).
const SCIMErrorSchema = "urn:ietf:params:scim:api:messages:2.0:Error"

// InvalidSCIMRequest returns the error of a malformed or unacceptable SCIM
// request (PLX-6220).
func InvalidSCIMRequest(format string, args ...any) error {
	return plxerr.New(plxerr.SCIMRequestInvalid, format, args...)
}

// WriteSCIMError renders err as a SCIM error response with an HTTP status
// and, when RFC 7644 defines one for the failure, a scimType. The detail
// leads with the error's Plux code (PLX-6220 for an invalid request,
// PLX-8600 for an unlicensed feature) so an operator can look it up; it is
// the error's own message, which never carries a secret.
func WriteSCIMError(w http.ResponseWriter, status int, scimType string, err error) {
	detail := "the request failed"
	if err != nil {
		detail = err.Error()
	}
	body := struct {
		Schemas  []string `json:"schemas"`
		Status   string   `json:"status"`
		ScimType string   `json:"scimType,omitempty"`
		Detail   string   `json:"detail"`
	}{Schemas: []string{SCIMErrorSchema}, Status: strconv.Itoa(status), ScimType: scimType, Detail: detail}
	w.Header().Set("Content-Type", "application/scim+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
