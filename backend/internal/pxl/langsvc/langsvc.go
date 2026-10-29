// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package langsvc is the JSON face of the PXL language service for the
// Studio WebAssembly module (PXL-007, ADR-0099): requests carry the scope of
// the use site, the expression and a code-point offset; answers are JSON.
// It keeps the checked scopes of recent requests, so typing in one field
// does not rebuild its environment on every keystroke.
package langsvc

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/pxl"
)

// Scope is what an expression at a use site can read: the environment and
// the paths a guard around the expression has checked are not null.
type Scope struct {
	pxl.EnvSpec
	NonNull []string `json:"nonNull,omitempty"`
	// Limits override the registry defaults for constant folding; zero
	// fields keep the default. The conformance vectors set them.
	Limits struct {
		Budget         int64 `json:"budget,omitempty"`
		StringLength   int64 `json:"stringLength,omitempty"`
		CollectionSize int64 `json:"collectionSize,omitempty"`
		DecimalDigits  int64 `json:"decimalDigits,omitempty"`
	} `json:"limits"`
}

// cacheSize bounds the scopes a Host remembers.
const cacheSize = 16

// Host serves language-service requests. It is not safe for concurrent use;
// the WebAssembly module is single threaded.
type Host struct {
	services map[string]*pxl.Service
	order    []string
}

// NewHost returns an empty host.
func NewHost() *Host { return &Host{services: map[string]*pxl.Service{}} }

// service returns the service of a scope given as JSON.
func (h *Host) service(scope string) (*pxl.Service, error) {
	if s, ok := h.services[scope]; ok {
		return s, nil
	}
	var sc Scope
	if err := json.Unmarshal([]byte(scope), &sc); err != nil {
		return nil, fmt.Errorf("scope: %w", err)
	}
	env, err := pxl.NewEnv(sc.EnvSpec)
	if err != nil {
		return nil, fmt.Errorf("scope: %w", err)
	}
	opts := pxl.DefaultOptions()
	opts.NonNull = sc.NonNull
	l := &opts.Limits
	l.Budget = cmp.Or(sc.Limits.Budget, l.Budget)
	l.StringLength = cmp.Or(sc.Limits.StringLength, l.StringLength)
	l.CollectionSize = cmp.Or(sc.Limits.CollectionSize, l.CollectionSize)
	l.DecimalDigits = cmp.Or(sc.Limits.DecimalDigits, l.DecimalDigits)
	s := pxl.NewService(env, opts)
	if len(h.order) >= cacheSize {
		delete(h.services, h.order[0])
		h.order = h.order[1:]
	}
	h.services[scope] = s
	h.order = append(h.order, scope)
	return s, nil
}

// Handle answers one request as JSON. op is check, complete, hover or
// signature; offset is in code points and ignored by check. Hover and
// signature answer null where there is nothing to show. A failure answers
// {"error": …}; Handle never panics.
func (h *Host) Handle(op, scope, src string, offset int) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = errorJSON(fmt.Sprintf("the language service failed unexpectedly: %v", r))
		}
	}()
	svc, err := h.service(scope)
	if err != nil {
		return errorJSON(err.Error())
	}
	var v any
	switch op {
	case "check":
		v = svc.Check(src)
	case "complete":
		v = svc.Complete(src, offset)
	case "hover":
		if r, ok := svc.Hover(src, offset); ok {
			v = r
		}
	case "signature":
		if r, ok := svc.SignatureHelp(src, offset); ok {
			v = r
		}
	default:
		return errorJSON(errors.New("unknown operation " + op).Error())
	}
	data, err := json.Marshal(v)
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(data)
}

func errorJSON(msg string) string {
	data, _ := json.Marshal(map[string]string{"error": msg})
	return string(data)
}
