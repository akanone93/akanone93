// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"errors"

	"github.com/tetratelabs/wazero/api"
)

// ABI names and versions (ADR-0062).
const (
	abiVersion = 1
	hostModule = "plux_v1"
	wasiModule = "wasi_snapshot_preview1"
)

// exportInitialise is the name of the WASI reactor export that runs a module's
// constructors; the American spelling is the name in the ABI.
const exportInitialise = "_initialize" //nolint:misspell // the ABI's export name

// Result tags of plux_invoke and of invoke_function.
const (
	tagOutput        = 0
	tagBusinessError = 1
	tagInputRefused  = 2
	tagOutputFailed  = 3
)

// Statuses a host call returns to the guest, negative so that they cannot be
// mistaken for a packed buffer.
const (
	statusNotFound     = -1
	statusDenied       = -2
	statusLimit        = -3
	statusEgressDenied = -4
	statusUpstream     = -5
	statusInvalid      = -6
	statusUnavailable  = -7
)

// Placement says where a module runs. A device host does not provide the
// server-only imports (FN-052).
type Placement string

// The placements of FN-050.
const (
	// PlacementServer runs on the server with every capability the function
	// was granted.
	PlacementServer Placement = "server"
	// PlacementDevice runs on the device with log, clock and the plugin's
	// key-value namespace only.
	PlacementDevice Placement = "device"
)

// importSpec is the signature of one import of plux_v1.
type importSpec struct {
	params     []api.ValueType
	results    []api.ValueType
	serverOnly bool
}

const (
	i32 = api.ValueTypeI32
	i64 = api.ValueTypeI64
)

// hostImports lists every import of plux_v1 with its signature. Pointers and
// lengths are i32; a packed buffer or a status is i64.
func hostImports() map[string]importSpec {
	return map[string]importSpec{
		"log":             {params: []api.ValueType{i32, i32, i32}},
		"clock_now":       {results: []api.ValueType{i64}},
		"context":         {results: []api.ValueType{i64}},
		"env_get":         {params: []api.ValueType{i32, i32}, results: []api.ValueType{i64}},
		"kv_get":          {params: []api.ValueType{i32, i32}, results: []api.ValueType{i64}},
		"kv_put":          {params: []api.ValueType{i32, i32, i32, i32}, results: []api.ValueType{i32}},
		"kv_delete":       {params: []api.ValueType{i32, i32}, results: []api.ValueType{i32}},
		"http_request":    {params: []api.ValueType{i32, i32}, results: []api.ValueType{i64}, serverOnly: true},
		"secret_get":      {params: []api.ValueType{i32, i32}, results: []api.ValueType{i64}, serverOnly: true},
		"claims":          {results: []api.ValueType{i64}, serverOnly: true},
		"invoke_function": {params: []api.ValueType{i32, i32, i32, i32}, results: []api.ValueType{i64}, serverOnly: true},
	}
}

// exportSpec is the signature of one export a module must (or may) provide.
type exportSpec struct {
	name     string
	params   []api.ValueType
	results  []api.ValueType
	optional bool
}

// requiredExports lists the functions of the module ABI; `memory` is checked
// separately.
func requiredExports() []exportSpec {
	return []exportSpec{
		{name: "plux_abi_version", results: []api.ValueType{i32}},
		{name: "plux_alloc", params: []api.ValueType{i32}, results: []api.ValueType{i32}},
		{name: "plux_invoke", params: []api.ValueType{i32, i32, i32}, results: []api.ValueType{i64}},
		{name: "plux_schemas", params: []api.ValueType{i32}, results: []api.ValueType{i64}, optional: true},
		{name: exportInitialise, optional: true},
	}
}

// Sentinel errors a Host returns; the host maps them to the statuses of the
// ABI. Any other error is reported to the guest as unavailable.
var (
	// ErrNotFound reports a missing key, secret, environment variable or
	// function (-1).
	ErrNotFound = errors.New("fnhost: not found")
	// ErrLimit reports that a limit of the capability was exceeded (-3).
	ErrLimit = errors.New("fnhost: limit exceeded")
	// ErrEgressRefused reports that the egress refused the request (-4).
	ErrEgressRefused = errors.New("fnhost: egress refused")
	// ErrUpstream reports that the remote side failed (-5).
	ErrUpstream = errors.New("fnhost: upstream failed")
	// ErrUnavailable reports that the capability cannot be served now (-7).
	ErrUnavailable = errors.New("fnhost: unavailable")
)

// statusOf maps a Host error to the status the guest receives.
func statusOf(err error) int32 {
	switch {
	case errors.Is(err, ErrNotFound):
		return statusNotFound
	case errors.Is(err, ErrLimit):
		return statusLimit
	case errors.Is(err, ErrEgressRefused):
		return statusEgressDenied
	case errors.Is(err, ErrUpstream):
		return statusUpstream
	default:
		return statusUnavailable
	}
}
