// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// contentType is the media type of every SCIM response (RFC 7644 §3.1).
const contentType = "application/scim+json"

// respond writes a JSON document. A document that cannot be encoded is the
// server's failure, not a half-written response.
func (s *server) respond(c call, status int, doc any, version int64, location string) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(doc); err != nil {
		s.fail(c.w, c.r, err)
		return
	}
	h := c.w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", "no-store")
	if version != 0 {
		h.Set("ETag", etag(version))
	}
	if location != "" {
		h.Set("Location", location)
	}
	c.w.WriteHeader(status)
	_, _ = c.w.Write(body.Bytes())
}

// decode reads a request body into v. A body that is not JSON of the shape is
// a 400 invalidSyntax; one over the core's bound is a 413.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return reject(http.StatusRequestEntityTooLarge, "", "the request body is too large")
		}
		return reject(http.StatusBadRequest, "invalidSyntax", "the request body is not valid JSON of the expected shape")
	}
	if dec.More() {
		return reject(http.StatusBadRequest, "invalidSyntax", "the request body has content after the document")
	}
	return nil
}

// hasSchema reports whether a request names the schema it is made of.
func hasSchema(schemas []string, want string) bool {
	for _, s := range schemas {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

// etag renders a version as a weak entity tag.
func etag(version int64) string { return `W/"` + strconv.FormatInt(version, 10) + `"` }

// matchesETag reports whether a header value of entity tags (If-Match,
// If-None-Match) names the version, by the weak comparison RFC 7232 gives
// these headers. "*" names every version.
func matchesETag(header string, version int64) bool {
	for _, tag := range strings.Split(header, ",") {
		tag = strings.TrimSpace(tag)
		if tag == "*" || strings.TrimPrefix(tag, "W/") == `"`+strconv.FormatInt(version, 10)+`"` {
			return true
		}
	}
	return false
}

// unchanged answers a conditional GET whose If-None-Match names the current
// version with 304, and reports whether it did.
func (s *server) unchanged(c call, version int64) bool {
	if header := c.r.Header.Get("If-None-Match"); header != "" && matchesETag(header, version) {
		c.w.Header().Set("ETag", etag(version))
		c.w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

// precondition checks an If-Match header against the current version: a
// request that carries one acts only on the version it names.
func precondition(r *http.Request, version int64) error {
	if header := r.Header.Get("If-Match"); header != "" && !matchesETag(header, version) {
		return reject(http.StatusPreconditionFailed, "", "the resource has changed since the version named in If-Match")
	}
	return nil
}

// location is where a resource of a kind is found.
func location(kind, id string) string { return mount + "/" + kind + "/" + id }
