// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
)

// mount is where the core serves SCIM; the locations of resources are
// relative to the host and start with it.
const mount = "/scim/v2"

// Options configure the service.
type Options struct {
	// Log receives the failures a client is not told about. Nil discards
	// them.
	Log *slog.Logger
}

// Service is the SCIM 2.0 server. It holds nothing but its logger, so one
// value serves every organisation.
type Service struct{ log *slog.Logger }

var _ identity.SCIMService = Service{}

// New returns the SCIM service.
func New(o Options) Service {
	if o.Log == nil {
		o.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return Service{log: o.Log}
}

// Handler returns the SCIM server over a directory. Its routes are relative
// to /scim/v2/.
func (s Service) Handler(d identity.Directory) http.Handler {
	srv := &server{dir: d, log: s.log}
	srv.routes = map[string]handler{
		"GET Users":                 srv.listUsers,
		"POST Users":                srv.createUser,
		"GET Users/{id}":            srv.getUser,
		"PUT Users/{id}":            srv.replaceUser,
		"PATCH Users/{id}":          srv.patchUser,
		"DELETE Users/{id}":         srv.deleteUser,
		"GET Groups":                srv.listGroups,
		"POST Groups":               srv.createGroup,
		"GET Groups/{id}":           srv.getGroup,
		"PUT Groups/{id}":           srv.replaceGroup,
		"PATCH Groups/{id}":         srv.patchGroup,
		"DELETE Groups/{id}":        srv.deleteGroup,
		"GET ServiceProviderConfig": srv.serviceProviderConfig,
		"GET Schemas":               srv.listSchemas,
		"GET Schemas/{id}":          srv.getSchema,
		"GET ResourceTypes":         srv.listResourceTypes,
		"GET ResourceTypes/{id}":    srv.getResourceType,
	}
	return srv
}

// server serves one directory.
type server struct {
	dir    identity.Directory
	log    *slog.Logger
	routes map[string]handler
}

// handler serves one route for an organisation; id is the resource named in
// the path, or "".
type handler func(c call, id string)

// call is a request in the organisation its token belongs to.
type call struct {
	w   http.ResponseWriter
	r   *http.Request
	org string
}

// ctx is the request's context.
func (c call) ctx() context.Context { return c.r.Context() }

// ServeHTTP authenticates the request, then routes it.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, err := s.authenticate(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c := call{w: w, r: r, org: org}
	resource, id, ok := splitPath(r.URL.Path)
	if !ok {
		s.fail(w, r, errNoRoute)
		return
	}
	route := resource
	if id != "" {
		route += "/{id}"
	}
	h, ok := s.routes[r.Method+" "+route]
	if !ok {
		s.refuseRoute(c, route)
		return
	}
	if r.Method != http.MethodGet {
		if err := edition.Require(r.Context(), edition.FeatureSCIM); err != nil {
			identity.WriteSCIMError(w, http.StatusForbidden, "", err)
			return
		}
	}
	h(c, id)
}

// refuseRoute answers a request no route serves: 405 with the methods that
// are served for the path, or 404 when none is.
func (s *server) refuseRoute(c call, route string) {
	var allowed []string
	for key := range s.routes {
		if method, path, _ := strings.Cut(key, " "); path == route {
			allowed = append(allowed, method)
		}
	}
	if len(allowed) == 0 {
		s.fail(c.w, c.r, errNoRoute)
		return
	}
	slices.Sort(allowed)
	c.w.Header().Set("Allow", strings.Join(allowed, ", "))
	s.fail(c.w, c.r, errMethod)
}

// splitPath splits /Users or /Users/{id} into the resource and the id.
func splitPath(path string) (resource, id string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch len(parts) {
	case 1:
		return parts[0], "", parts[0] != ""
	case 2:
		return parts[0], parts[1], parts[0] != "" && parts[1] != ""
	default:
		return "", "", false
	}
}

// authenticate resolves the request's bearer token to an organisation.
func (s *server) authenticate(r *http.Request) (string, error) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", identity.ErrUnauthorized
	}
	return s.dir.Authenticate(r.Context(), token) //nolint:wrapcheck // examined by fail
}
