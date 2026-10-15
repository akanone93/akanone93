// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"encoding/json"
	"errors"
	"time"
)

// Limits bound one invocation (FN-011, FN-053). The values come from the
// limits registry (LIM-001); the package has no defaults. A zero byte or
// count limit allows none of that resource.
type Limits struct {
	// Timeout is the wall time of the invocation; it must be positive.
	Timeout time.Duration
	// MemoryBytes caps the instance's linear memory.
	MemoryBytes uint64
	// InputBytes caps the input; a larger one is refused before an instance
	// exists.
	InputBytes int
	// OutputBytes caps the JSON payload of the result.
	OutputBytes int
	// LogBytes caps the log output of the invocation.
	LogBytes int
	// HTTPCalls caps the http_request calls that reach the Host.
	HTTPCalls int
	// CallDepth caps the nesting of invocations: a call at depth d may
	// invoke only while d+1 <= CallDepth.
	CallDepth int
}

// Grants are the capabilities declared for the function (FN-012). The zero
// value grants nothing.
type Grants struct {
	// HTTPDomains is the allowlist of http_request: exact host names or
	// `*.example.com` wildcards. Empty means HTTP is not granted.
	HTTPDomains []string
	// KV grants the key-value namespace.
	KV bool
	// Secrets are the names secret_get may read.
	Secrets []string
	// Functions are the names invoke_function may call.
	Functions []string
	// Claims grants the verified claims of the caller.
	Claims bool
}

// ContextInfo is what context() reports besides the placement.
type ContextInfo struct {
	// App is the app identifier.
	App string `json:"app"`
	// Environment is the environment name.
	Environment string `json:"environment"`
	// AssuranceLevel is the caller's assurance level, e.g. "AL2".
	AssuranceLevel string `json:"assuranceLevel"`
}

// Call is one invocation of a function in a module.
type Call struct {
	// Index selects the entry of the module's dispatch table.
	Index uint32
	// Input is the JSON input, passed to the guest unchanged.
	Input []byte
	// Limits bound the invocation.
	Limits Limits
	// Grants are the capabilities of the function.
	Grants Grants
	// Context is reported by context().
	Context ContextInfo
	// Env holds the function's configured variables for env_get.
	Env map[string]string
	// Claims is the verified claims JSON for claims(); empty means none.
	Claims json.RawMessage
	// Depth is the nesting level of this invocation: zero for a top-level
	// one.
	Depth int
	// Host serves the granted capabilities; nil serves none.
	Host Host
	// Log receives the function's log records, in order; nil discards them.
	// It is called on the invoking goroutine.
	Log func(LogRecord)
}

// Stats describe a finished invocation, successful or not.
type Stats struct {
	// Duration is measured with the engine's clock.
	Duration time.Duration
	// PeakMemoryBytes is the largest linear memory the instance reached.
	PeakMemoryBytes uint64
	// HTTPCalls counts the http_request calls that reached the Host.
	HTTPCalls int
	// LogBytes counts the log bytes accepted.
	LogBytes int
}

// validate rejects a call that is a caller bug rather than a guest failure.
func (c *Call) validate() error {
	if c.Limits.Timeout <= 0 {
		return errors.New("fnhost: Limits.Timeout must be positive")
	}
	if c.Depth < 0 {
		return errors.New("fnhost: Depth must not be negative")
	}
	return nil
}
