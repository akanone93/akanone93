// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"context"
	"encoding/json"
)

// Host serves the capabilities a function was granted. The runner implements
// it over the key-value store, the egress, the secrets and the nested
// invocation; the engine calls it only for a granted capability, within the
// call's limits, and never with a guest-controlled size larger than the
// guest's own memory. Every method receives the call's context, which ends
// with the call's deadline. Implementations return the package's sentinel
// errors (ErrNotFound and the others) so the engine can map them to a
// status. An implementation must be safe for concurrent use when the
// engine's callers share it between calls.
type Host interface {
	// KVGet returns the value of key, or ErrNotFound.
	KVGet(ctx context.Context, key string) ([]byte, error)
	// KVPut stores value under key.
	KVPut(ctx context.Context, key string, value []byte) error
	// KVDelete removes key, or returns ErrNotFound.
	KVDelete(ctx context.Context, key string) error
	// HTTP performs a request whose host the engine already matched against
	// the function's allowlist.
	HTTP(ctx context.Context, req HTTPRequest) (HTTPResponse, error)
	// Secret returns the value of the named secret, or ErrNotFound.
	Secret(ctx context.Context, name string) ([]byte, error)
	// Invoke runs a nested function at the given depth (the caller's depth
	// plus one, already checked against the call-depth limit).
	Invoke(ctx context.Context, name string, input []byte, depth int) (Outcome, error)
}

// HTTPRequest is the request of http_request, in the JSON form of the ABI.
// Body is base64 on the wire.
type HTTPRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

// HTTPResponse is the response of http_request, in the JSON form of the ABI.
// Body is base64 on the wire.
type HTTPResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}

// Outcome is the result of an invocation that ended in the function's own
// words: exactly one of Output and Business is set.
type Outcome struct {
	// Output is the JSON the function returned (tag 0).
	Output json.RawMessage
	// Business is the function's business error (tag 1, FN-022).
	Business *BusinessError
}

// BusinessError is a function's declared failure, not a system error
// (FN-022).
type BusinessError struct {
	// Code is the function's error code.
	Code string `json:"code"`
	// MessageKey is the localisation key of the message.
	MessageKey string `json:"messageKey"`
	// Params are the values the message interpolates, as a JSON object.
	Params json.RawMessage `json:"params,omitempty"`
}

// LogLevel is the severity of a log record; the values are those of the
// `log` import.
type LogLevel int

// The log levels of the ABI.
const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

// LogRecord is one line a function logged, through `log` or through
// standard output (info) and standard error (error).
type LogRecord struct {
	// Level is the severity.
	Level LogLevel
	// Message is the text, without a trailing newline.
	Message string
}

// LogTruncated is the message of the one record that replaces the log output
// beyond Limits.LogBytes.
const LogTruncated = "log output truncated"
