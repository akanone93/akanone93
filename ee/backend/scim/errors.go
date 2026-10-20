// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim

import (
	"errors"
	"net/http"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// requestError is a request the server rejects itself, with the status and
// the scimType RFC 7644 §3.12 gives the kind of failure.
type requestError struct {
	status   int
	scimType string
	detail   string
}

// Error returns the detail.
func (e *requestError) Error() string { return e.detail }

// reject returns a rejected-request error.
func reject(status int, scimType, detail string) error {
	return &requestError{status: status, scimType: scimType, detail: detail}
}

// invalid is a 400 for a value that cannot be accepted.
func invalid(detail string) error { return reject(http.StatusBadRequest, "invalidValue", detail) }

// The routing failures.
var (
	errNoRoute = reject(http.StatusNotFound, "", "there is no such SCIM endpoint")
	errMethod  = reject(http.StatusMethodNotAllowed, "", "the endpoint does not serve this method")
)

// fail renders err as a SCIM error. A rejected request and the directory's
// sentinels are the client's to know about, and their detail starts with
// PLX-6220; anything else is the server's, which is logged and answered
// without its text.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var rejected *requestError
	status, scimType := http.StatusInternalServerError, ""
	switch {
	case errors.As(err, &rejected):
		status, scimType = rejected.status, rejected.scimType
	case errors.Is(err, identity.ErrUnauthorized):
		w.Header().Set("WWW-Authenticate", `Bearer realm="scim"`)
		status = http.StatusUnauthorized
		err = errors.New("the bearer token is missing or not valid")
	case errors.Is(err, identity.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, identity.ErrConflict):
		status, scimType = http.StatusConflict, "uniqueness"
	case errors.Is(err, identity.ErrVersionMismatch):
		status = http.StatusPreconditionFailed
	case errors.Is(err, identity.ErrInvalid):
		status, scimType = http.StatusBadRequest, "invalidValue"
	case errors.Is(err, identity.ErrRefused):
		status = http.StatusConflict
	default:
		s.log.ErrorContext(r.Context(), "scim: request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		identity.WriteSCIMError(w, status, "", errors.New("the request could not be processed"))
		return
	}
	identity.WriteSCIMError(w, status, scimType, identity.InvalidSCIMRequest("%s", err.Error()))
}
